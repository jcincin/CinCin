/*
Author: Bruce Jagid
Created On: Aug 12, 2023
*/
package resy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/21Bruce/resolved-server/api"
	"github.com/21Bruce/resolved-server/config"
	"github.com/21Bruce/resolved-server/store"
)

/*
Name: API
Type: API interface struct
Purpose: This struct acts as the resy implementation of the
api interface.
Note: The only known working APIKey value can be located and
defaulted using the GetDefaultAPI function, but we leave
it exposed so front-facing wrappers may expose it as a
setting
*/
type API struct {
	APIKey    string
	Cookies   []*http.Cookie // Imperva cookies for bypassing WAF
	UserAgent string         // User agent matching the cookies
}

/*
Name: isCodeFail
Type: Internal Func
Purpose: Function which takes in an HTTP code and returns
true if it is not a success code and false otherwise
*/
func isCodeFail(code int) bool {
	fst := code / 100
	return (fst != 2)
}

/*
Name: byteToJSONString
Type: Internal Func
Purpose: Function which takes in a byte sequence
representing a JSON struct and returns a string
or error. Useful for debugging
*/
func byteToJSONString(data []byte) (string, error) {
	var out bytes.Buffer
	err := json.Indent(&out, data, "", " ")

	if err != nil {
		return "", err
	}

	d := out.Bytes()
	return string(d), nil
}

/*
Name: min
Type: Internal Func
Purpose: Function that determins the min of two ints
*/
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncateForLog(body []byte, max int) string {
	if len(body) <= max {
		return string(body)
	}
	return string(body[:max]) + "..."
}

/*
Name: SetCookies
Type: API Func
Purpose: Set Imperva cookies and user agent for the API client
*/
func (a *API) SetCookies(cookies []*http.Cookie, userAgent string) {
	a.Cookies = cookies
	if userAgent != "" {
		a.UserAgent = userAgent
	} else {
		// Default user agent if none provided
		a.UserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	}
}

/*
Name: addCookiesToRequest
Type: Internal Func
Purpose: Add Imperva cookies and user agent to HTTP request
*/
func (a *API) addCookiesToRequest(req *http.Request) {
	// Add cookies to request
	if len(a.Cookies) > 0 {
		for _, cookie := range a.Cookies {
			req.AddCookie(cookie)
		}
	}

	// Set user agent if available
	if a.UserAgent != "" {
		req.Header.Set("User-Agent", a.UserAgent)
	}
}

/*
Name: extractCookiesFromResponse
Type: Internal Func
Purpose: Extract cookies from HTTP response headers and update API client cookies
*/
func (a *API) extractCookiesFromResponse(resp *http.Response) {
	// Check if this is an Imperva response
	if resp.Header.Get("X-Cdn") == "Imperva" || resp.Header.Get("Server") == "nginx" {
		log.Printf("Imperva challenge detected, extracting cookies")

		// Parse Set-Cookie headers
		for _, cookieStr := range resp.Header.Values("Set-Cookie") {
			// Parse the cookie string manually
			parts := strings.Split(cookieStr, ";")
			if len(parts) > 0 {
				nameValue := strings.SplitN(parts[0], "=", 2)
				if len(nameValue) == 2 {
					cookieName := strings.TrimSpace(nameValue[0])
					cookieValue := nameValue[1]

					// Check if it's an Imperva cookie
					if strings.HasPrefix(cookieName, "_incap_") ||
						strings.HasPrefix(cookieName, "incap_ses_") ||
						strings.HasPrefix(cookieName, "_visid_") ||
						strings.HasPrefix(cookieName, "visid_incap_") ||
						strings.HasPrefix(cookieName, "nlbi_") {

						cookie := &http.Cookie{
							Name:   cookieName,
							Value:  cookieValue,
							Domain: ".resy.com",
							Path:   "/",
						}

						// Parse additional attributes
						for i := 1; i < len(parts); i++ {
							part := strings.TrimSpace(parts[i])
							if strings.HasPrefix(strings.ToLower(part), "domain=") {
								cookie.Domain = strings.TrimPrefix(part, "domain=")
							} else if strings.HasPrefix(strings.ToLower(part), "path=") {
								cookie.Path = strings.TrimPrefix(part, "path=")
							} else if strings.ToLower(part) == "secure" {
								cookie.Secure = true
							} else if strings.ToLower(part) == "httponly" {
								cookie.HttpOnly = true
							} else if strings.HasPrefix(strings.ToLower(part), "expires=") {
								// Parse expiration if needed
								expiresStr := strings.TrimPrefix(part, "expires=")
								if t, err := time.Parse(time.RFC1123, expiresStr); err == nil {
									cookie.Expires = t
								}
							}
						}

						// Add or update cookie
						found := false
						for i, existingCookie := range a.Cookies {
							if existingCookie.Name == cookie.Name {
								a.Cookies[i] = cookie
								found = true
								break
							}
						}
						if !found {
							a.Cookies = append(a.Cookies, cookie)
						}
					}
				}
			}
		}

		if len(a.Cookies) > 0 {
			log.Printf("Updated cookies from Imperva response: %d cookies", len(a.Cookies))
		}
	}
}

/*
Name: isImpervaChallenge
Type: Internal Func
Purpose: Check if an HTTP response is an Imperva challenge
*/
func isImpervaChallenge(resp *http.Response) bool {
	// Imperva can return 500, 403, or 503
	if resp.StatusCode != 500 && resp.StatusCode != 403 && resp.StatusCode != 503 {
		return false
	}
	// Check for Imperva headers
	if resp.Header.Get("X-Cdn") == "Imperva" {
		return true
	}
	// Sometimes nginx is used as a proxy
	if resp.Header.Get("Server") == "nginx" && resp.StatusCode == 500 {
		return true
	}
	return false
}

/*
Name: doRequestWithRetry
Type: Internal Func
Purpose: Execute HTTP request with automatic retry on Imperva challenge
Note: For POST requests, the bodyBytes should be provided to recreate the request on retry
Returns api.ErrImperva if all retries fail due to Imperva challenge
*/
func (a *API) doRequestWithRetry(client *http.Client, req *http.Request, bodyBytes []byte, maxRetries int, venueID int64) (*http.Response, error) {
	// Store original headers for retry
	originalHeaders := make(map[string][]string)
	for key, values := range req.Header {
		originalHeaders[key] = values
	}
	originalURL := req.URL.String()
	originalMethod := req.Method

	var lastImpervaResponse bool

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// On retry, recreate the request with the body
		if attempt > 0 {
			log.Printf("Retry %d/%d with updated cookies", attempt+1, maxRetries+1)

			// Recreate request with body for POST requests
			if bodyBytes != nil {
				var err error
				req, err = http.NewRequest(originalMethod, originalURL, bytes.NewBuffer(bodyBytes))
				if err != nil {
					return nil, fmt.Errorf("failed to recreate request: %w", err)
				}

				// Copy headers from original request
				for key, values := range originalHeaders {
					for _, value := range values {
						req.Header.Add(key, value)
					}
				}
			}

			// Re-add cookies in case they were updated
			a.addCookiesToRequest(req)

			// Small delay before retry
			time.Sleep(1 * time.Second)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}

		// Check if this is an Imperva challenge
		if isImpervaChallenge(resp) {
			log.Printf("Imperva challenge (status %d), retrying", resp.StatusCode)
			lastImpervaResponse = true

			// Extract cookies from response
			a.extractCookiesFromResponse(resp)

			// Retry if we haven't exceeded max retries
			if attempt < maxRetries {
				resp.Body.Close()
				continue
			} else {
				// Retries exhausted - return ErrImperva
				resp.Body.Close()
				log.Printf("Imperva challenge unresolved after %d retries", maxRetries+1)
				return nil, api.ErrImperva
			}
		}

		lastImpervaResponse = false
		return resp, nil
	}

	if lastImpervaResponse {
		return nil, api.ErrImperva
	}
	return nil, fmt.Errorf("max retries exceeded")
}

/*
Name: LoadCookiesFromStore
Type: API Func
Purpose: Load cookies from Redis store for a venue
*/
func (a *API) LoadCookiesFromStore(venueID int64) error {
	ctx := context.Background()
	cookieData, err := store.GetCookies(ctx, venueID)
	if err != nil {
		return err
	}
	a.SetCookies(cookieData.Cookies, cookieData.UserAgent)
	log.Printf("Loaded %d cookies for venue %d", len(cookieData.Cookies), venueID)
	return nil
}

/*
Name: GetDefaultAPI
Type: External Func
Purpose: Function that provides an out of the box
working API struct
*/
func GetDefaultAPI() API {
	return API{
		APIKey: config.Get().ResyAPIKey,
	}
}

/*
Name: Login
Type: API Func
Purpose: Resy implementation of the Login api func
Note: The only required login fields for this func
are Email and Password.
*/
func (a *API) Login(params api.LoginParam) (*api.LoginResponse, error) {
	authUrl := "https://api.resy.com/3/auth/password"
	email := url.QueryEscape(params.Email)
	password := url.QueryEscape(params.Password)
	bodyStr := `email=` + email + `&password=` + password
	bodyBytes := []byte(bodyStr)

	request, err := http.NewRequest("POST", authUrl, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Authorization", `ResyAPI api_key="`+a.APIKey+`"`)

	// Add Imperva cookies and user agent
	a.addCookiesToRequest(request)

	client := &http.Client{}
	response, err := client.Do(request)

	if err != nil {
		return nil, err
	}

	defer response.Body.Close()

	// Resy servers return a 419 is the auth parameters were invalid
	if response.StatusCode == 419 {
		return nil, api.ErrLoginWrong
	}

	if isCodeFail(response.StatusCode) {
		return nil, api.ErrNetwork
	}

	responseBody, err := io.ReadAll(response.Body)

	if err != nil {
		return nil, err
	}

	var jsonMap map[string]interface{}
	err = json.Unmarshal(responseBody, &jsonMap)
	if err != nil {
		return nil, err
	}

	if jsonMap["payment_method_id"] == nil {
		return nil, api.ErrNoPayInfo
	}

	loginResponse := api.LoginResponse{
		ID:              int64(jsonMap["id"].(float64)),
		FirstName:       jsonMap["first_name"].(string),
		LastName:        jsonMap["last_name"].(string),
		Mobile:          jsonMap["mobile_number"].(string),
		Email:           jsonMap["em_address"].(string),
		PaymentMethodID: int64(jsonMap["payment_method_id"].(float64)),
		AuthToken:       jsonMap["token"].(string),
	}

	return &loginResponse, nil

}

/*
Name: Search
Type: API Func
Purpose: Resy implementation of the Search api func
*/
func (a *API) Search(params api.SearchParam) (*api.SearchResponse, error) {
	searchUrl := "https://api.resy.com/3/venuesearch/search"

	bodyStr := `{"query":"` + params.Name + `"}`
	bodyBytes := []byte(bodyStr)

	request, err := http.NewRequest("POST", searchUrl, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", `ResyAPI api_key="`+a.APIKey+`"`)
	request.Header.Set("Origin", `https://resy.com`)
	request.Header.Set("Referer", `https://resy.com/`)

	// Add Imperva cookies and user agent
	a.addCookiesToRequest(request)

	client := &http.Client{}
	response, err := client.Do(request)

	if err != nil {
		return nil, err
	}

	defer response.Body.Close()

	if isCodeFail(response.StatusCode) {
		responseBody, _ := io.ReadAll(response.Body)
		log.Printf("Search failed: status %d, body: %s", response.StatusCode, truncateForLog(responseBody, 200))
		return nil, api.ErrNetwork
	}

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	var jsonTopLevelMap map[string]interface{}
	err = json.Unmarshal(responseBody, &jsonTopLevelMap)
	if err != nil {
		log.Printf("Search unmarshal error: %v", err)
		return nil, err
	}

	// Check if "search" key exists
	searchValue, ok := jsonTopLevelMap["search"]
	if !ok {
		log.Printf("Search response missing 'search' key")
		return nil, api.ErrNetwork
	}

	jsonSearchMap, ok := searchValue.(map[string]interface{})
	if !ok {
		log.Printf("Search response 'search' is not a map")
		return nil, api.ErrNetwork
	}

	// Check if "hits" key exists
	hitsValue, ok := jsonSearchMap["hits"]
	if !ok {
		log.Printf("Search response missing 'hits' key")
		return nil, api.ErrNetwork
	}

	jsonHitsMap, ok := hitsValue.([]interface{})
	if !ok {
		log.Printf("Search response 'hits' is not an array")
		return nil, api.ErrNetwork
	}

	numHits := len(jsonHitsMap)

	// if input param limit is nonnegative, limit the search loop
	var limit int
	if params.Limit > 0 {
		limit = min(params.Limit, numHits)
	} else {
		limit = numHits
	}

	searchResults := make([]api.SearchResult, 0, limit)
	for i := 0; i < limit; i++ {
		jsonHitMap, ok := jsonHitsMap[i].(map[string]interface{})
		if !ok {
			continue
		}

		// Safely extract fields with nil checks
		objectID, ok := jsonHitMap["objectID"].(string)
		if !ok {
			continue
		}

		venueID, err := strconv.ParseInt(objectID, 10, 64)
		if err != nil {
			continue
		}

		name, _ := jsonHitMap["name"].(string)
		region, _ := jsonHitMap["region"].(string)
		locality, _ := jsonHitMap["locality"].(string)
		neighborhood, _ := jsonHitMap["neighborhood"].(string)

		searchResults = append(searchResults, api.SearchResult{
			VenueID:      venueID,
			Name:         name,
			Region:       region,
			Locality:     locality,
			Neighborhood: neighborhood,
		})
	}

	searchResponse := api.SearchResponse{
		Results: searchResults,
	}

	return &searchResponse, nil
}

/*
Name: Reserve
Type: API Func
Purpose: Resy implementation of the Reserve api func
*/
func (a *API) Reserve(params api.ReserveParam) (*api.ReserveResponse, error) {
	if len(params.ReservationTimes) == 0 {
		return nil, api.ErrTimeNull
	}

	// Try to load cookies from Redis store for this venue
	if err := a.LoadCookiesFromStore(params.VenueID); err != nil {
		log.Printf("Warning: cookies not found for venue %d: %v", params.VenueID, err)
		// Continue anyway - cookies might have been set manually or we'll get Imperva error
	}

	// Converting fields to URL query format
	// IMPORTANT: Convert to NYC timezone before extracting date components
	// The reservation time is stored in UTC, but Resy expects the date in NYC timezone
	nycLocation, err := time.LoadLocation("America/New_York")
	if err != nil {
		nycLocation = time.UTC
	}
	reservationTimeNYC := params.ReservationTimes[0].In(nycLocation)

	year := strconv.Itoa(reservationTimeNYC.Year())
	monthInt := int(reservationTimeNYC.Month())
	dayInt := reservationTimeNYC.Day()

	// Zero-pad month and day
	month := fmt.Sprintf("%02d", monthInt)
	day := fmt.Sprintf("%02d", dayInt)

	date := year + "-" + month + "-" + day

	// Use JSON body for find request (Resy API expects application/json)
	requestBody := map[string]interface{}{
		"day":        date,
		"venue_id":   params.VenueID,
		"party_size": params.PartySize,
		"lat":        0,
		"long":       0,
	}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	findUrl := "https://api.resy.com/4/find"

	request, err := http.NewRequest("POST", findUrl, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	// Setting headers - Important: User-Agent needed to bypass Imperva WAF
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", `ResyAPI api_key="`+a.APIKey+`"`)
	request.Header.Set("X-Resy-Auth-Token", params.LoginResp.AuthToken)
	request.Header.Set("X-Resy-Universal-Auth-Token", params.LoginResp.AuthToken)
	request.Header.Set("Referer", "https://resy.com/")
	request.Header.Set("Origin", "https://resy.com")

	// Add Imperva cookies and user agent (will override default User-Agent if set)
	a.addCookiesToRequest(request)

	// Fallback to default User-Agent if not set via cookies
	if a.UserAgent == "" {
		request.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	}

	client := &http.Client{Timeout: 12 * time.Second}

	// Use retry logic for Imperva challenges (pass bodyBytes to recreate request on retry, and venueID for fallback)
	response, err := a.doRequestWithRetry(client, request, bodyBytes, 2, params.VenueID)
	if err != nil {
		return nil, err
	}

	defer response.Body.Close()

	// Always read the response body, even on error, to see what the API says
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	if isCodeFail(response.StatusCode) {
		errorMsg := truncateForLog(responseBody, 200)
		var errorMap map[string]interface{}
		if json.Unmarshal(responseBody, &errorMap) == nil {
			if message, ok := errorMap["message"].(string); ok {
				errorMsg = message
			}
		}
		return nil, api.NewNetworkError("find", response.StatusCode, errorMsg)
	}

	var jsonTopLevelMap map[string]interface{}
	err = json.Unmarshal(responseBody, &jsonTopLevelMap)
	if err != nil {
		return nil, err
	}

	jsonResultsMap, ok := jsonTopLevelMap["results"].(map[string]interface{})
	if !ok {
		return nil, api.NewNetworkError("find", 0, "invalid response: 'results' key not found")
	}

	jsonVenuesList, ok := jsonResultsMap["venues"].([]interface{})
	if !ok {
		return nil, api.NewNetworkError("find", 0, "invalid response: 'venues' key not found")
	}

	if len(jsonVenuesList) == 0 {
		return nil, api.ErrNoOffer
	}

	// Find the venue that matches the requested venue ID
	var jsonVenueMap map[string]interface{}
	for _, v := range jsonVenuesList {
		venue, ok := v.(map[string]interface{})
		if !ok {
			continue
		}

		// Try to extract venue ID from the response structure
		// Resy API returns venue info nested under "venue" key
		if venueInfo, ok := venue["venue"].(map[string]interface{}); ok {
			if idInfo, ok := venueInfo["id"].(map[string]interface{}); ok {
				if resyID, ok := idInfo["resy"].(float64); ok {
					if int64(resyID) == params.VenueID {
						jsonVenueMap = venue
						break
					}
				}
			}
		}
	}

	// If no matching venue found, fall back to first venue
	if jsonVenueMap == nil {
		var ok bool
		jsonVenueMap, ok = jsonVenuesList[0].(map[string]interface{})
		if !ok {
			return nil, api.NewNetworkError("find", 0, "invalid response: venue structure is invalid")
		}
	}

	jsonSlotsList, ok := jsonVenueMap["slots"].([]interface{})
	if !ok {
		return nil, api.NewNetworkError("find", 0, "invalid response: 'slots' key not found in venue")
	}

	// Iterate over table types and reservation times
	// If no table types specified, match any slot based on time only
	hasTableTypePreference := len(params.TableTypes) > 0

	for k := 0; k < len(params.TableTypes) || (!hasTableTypePreference && k == 0); k++ {
		var currentTableType api.TableType
		if hasTableTypePreference {
			currentTableType = params.TableTypes[k]
		}

		for i := 0; i < len(params.ReservationTimes); i++ {
			currentTime := params.ReservationTimes[i]

			// First pass: Try to find exact match, then closest match within window
			var bestSlot map[string]interface{}
			var bestSlotIndex int = -1
			var bestSlotTime time.Time
			var bestSlotConfigToken string
			var bestTimeDiff time.Duration = 31 * time.Minute // Track smallest time difference found (start larger than max)
			const maxTimeDiff = 30 * time.Minute              // Maximum allowed time difference
			foundExactMatch := false

			for j := 0; j < len(jsonSlotsList); j++ {
				jsonSlotMap, ok := jsonSlotsList[j].(map[string]interface{})
				if !ok {
					continue
				}

				jsonDateMap, ok := jsonSlotMap["date"].(map[string]interface{})
				if !ok {
					continue
				}

				startRaw, ok := jsonDateMap["start"].(string)
				if !ok {
					continue
				}

				startFields := strings.Split(startRaw, " ")
				if len(startFields) != 2 {
					continue
				}

				dateStr := startFields[0]
				timeFields := strings.Split(startFields[1], ":")
				if len(timeFields) != 3 {
					continue
				}

				// Parse the slot's full date/time
				// NOTE: Resy API returns times in the venue's local timezone (NYC), not UTC
				// We need to parse it as NYC time and compare with the requested time in NYC
				dateTimeStr := dateStr + " " + timeFields[0] + ":" + timeFields[1] + ":00"
				slotTime, err := time.ParseInLocation("2006-01-02 15:04:05", dateTimeStr, nycLocation)
				if err != nil {
					continue
				}

				// Convert currentTime to NYC for comparison
				currentTimeNYC := currentTime.In(nycLocation)

				// Check if the slot is on the same date as the requested time (in NYC timezone)
				if slotTime.Year() != currentTimeNYC.Year() ||
					slotTime.Month() != currentTimeNYC.Month() ||
					slotTime.Day() != currentTimeNYC.Day() {
					continue
				}

				// Check if the slot matches the desired time (exact match) using NYC times
				timeMatches := slotTime.Hour() == currentTimeNYC.Hour() && slotTime.Minute() == currentTimeNYC.Minute()

				// Get config map to check table type
				jsonConfigMap, ok := jsonSlotMap["config"].(map[string]interface{})
				if !ok {
					continue
				}

				// Check table type if preference is specified
				if hasTableTypePreference {
					tableType, ok := jsonConfigMap["type"].(string)
					if !ok {
						continue
					}

					if !strings.Contains(strings.ToLower(tableType), string(currentTableType)) {
						continue
					}
				}

				// If exact time match, use it immediately
				if timeMatches {
					bestSlot = jsonSlotMap
					bestSlotIndex = j
					bestSlotTime = slotTime
					configToken, ok := jsonConfigMap["token"].(string)
					if ok {
						bestSlotConfigToken = configToken
					}
					foundExactMatch = true
					break
				}

				// If no exact match yet, track the closest slot within the time window
				// Compare using NYC times since slots are in NYC timezone
				if !foundExactMatch {
					timeDiff := slotTime.Sub(currentTimeNYC)
					absTimeDiff := timeDiff
					if absTimeDiff < 0 {
						absTimeDiff = -absTimeDiff // Use absolute value
					}

					// Only consider slots within the max time window and that are better than current best
					if absTimeDiff <= maxTimeDiff && absTimeDiff < bestTimeDiff {
						bestTimeDiff = absTimeDiff
						bestSlot = jsonSlotMap
						bestSlotIndex = j
						bestSlotTime = slotTime
						configToken, ok := jsonConfigMap["token"].(string)
						if ok {
							bestSlotConfigToken = configToken
						}
					}
				}
			}

			// If we found a slot (exact or closest), proceed with booking
			if bestSlotIndex >= 0 {

				configToken := bestSlotConfigToken
				if configToken == "" {
					jsonConfigMap, ok := bestSlot["config"].(map[string]interface{})
					if !ok {
						continue
					}
					configToken, ok = jsonConfigMap["token"].(string)
					if !ok {
						continue
					}
				}

				detailUrl := "https://api.resy.com/3/details"

				// Prepare the request body
				requestBody := map[string]string{
					"commit":     strconv.Itoa(1),                // Convert integer 1 to string
					"config_id":  configToken,                    // Assuming configToken is already a string
					"day":        date,                           // Assuming date is already a string
					"party_size": strconv.Itoa(params.PartySize), // Convert PartySize (an int) to string
				}
				jsonBody, err := json.Marshal(requestBody)
				if err != nil {
					continue
				}

				requestDetail, err := http.NewRequest("POST", detailUrl, bytes.NewBuffer(jsonBody))
				if err != nil {
					continue
				}

				// Setting headers for detail request
				// Set the appropriate headers
				requestDetail.Header.Set("Content-Type", "application/json")
				requestDetail.Header.Set("Authorization", `ResyAPI api_key="`+a.APIKey+`"`)

				// Add Imperva cookies and user agent
				a.addCookiesToRequest(requestDetail)

				// Fallback to default User-Agent if not set via cookies
				if a.UserAgent == "" {
					requestDetail.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
				}

				responseDetail, err := a.doRequestWithRetry(client, requestDetail, jsonBody, 2, params.VenueID)
				if err != nil {
					return nil, err
				}

				responseDetailBody, err := io.ReadAll(responseDetail.Body)
				responseDetail.Body.Close()
				if err != nil {
					return nil, err
				}

				if isCodeFail(responseDetail.StatusCode) {
					return nil, api.NewNetworkError("detail", responseDetail.StatusCode, truncateForLog(responseDetailBody, 200))
				}

				var detailTopLevelMap map[string]interface{}
				err = json.Unmarshal(responseDetailBody, &detailTopLevelMap)
				if err != nil {
					return nil, err
				}

				jsonBookTokenMap, ok := detailTopLevelMap["book_token"].(map[string]interface{})
				if !ok {
					continue
				}

				bookToken, ok := jsonBookTokenMap["value"].(string)
				if !ok {
					continue
				}

				// Proceed to booking step
				bookUrl := "https://api.resy.com/3/book"

				bookField := "book_token=" + url.QueryEscape(bookToken)
				paymentMethodStr := `{"id":` + strconv.FormatInt(params.LoginResp.PaymentMethodID, 10) + `}`
				paymentMethodField := "struct_payment_method=" + url.QueryEscape(paymentMethodStr)
				requestBookBodyStr := bookField + "&" + paymentMethodField + "&" + "source_id=resy.com-venue-details"

				requestBook, err := http.NewRequest("POST", bookUrl, bytes.NewBuffer([]byte(requestBookBodyStr)))
				if err != nil {
					continue
				}
				requestBook.Header.Set("Authorization", `ResyAPI api_key="`+a.APIKey+`"`)
				requestBook.Header.Set("Content-Type", `application/x-www-form-urlencoded`)
				requestBook.Header.Set("Host", `api.resy.com`)
				requestBook.Header.Set("X-Resy-Auth-Token", params.LoginResp.AuthToken)
				requestBook.Header.Set("X-Resy-Universal-Auth", params.LoginResp.AuthToken)
				requestBook.Header.Set("Referer", "https://resy.com/")

				// Add Imperva cookies and user agent
				a.addCookiesToRequest(requestBook)

				// Fallback to default User-Agent if not set via cookies
				if a.UserAgent == "" {
					requestBook.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
				}

				requestBookBytes := []byte(requestBookBodyStr)
				responseBook, err := a.doRequestWithRetry(client, requestBook, requestBookBytes, 2, params.VenueID)
				if err != nil {
					return nil, err
				}

				responseBookBody, err := io.ReadAll(responseBook.Body)
				responseBook.Body.Close()
				if err != nil {
					return nil, err
				}

				if isCodeFail(responseBook.StatusCode) {
					continue
				}

				var bookTopLevelMap map[string]interface{}
				err = json.Unmarshal(responseBookBody, &bookTopLevelMap)
				if err != nil {
					continue
				}

				// Check if booking was successful
				if _, ok := bookTopLevelMap["reservation_id"]; ok {
					resp := api.ReserveResponse{
						ReservationTime: bestSlotTime,
					}
					return &resp, nil
				} else {
					// Booking response missing confirmation, try next slot
					continue
				}
			}
		}
	}

	// If no table was found after all iterations
	return nil, api.ErrNoTable
}

/*
Name: AuthMinExpire
Type: API Func
Purpose: Resy implementation of the AuthMinExpire api func.
The largest minimum validity time is 6 days.
*/
func (a *API) AuthMinExpire() time.Duration {
	/* 6 days */
	var d time.Duration = time.Hour * 24 * 6
	return d
}

//func (a *API) Cancel(params api.CancelParam) (*api.CancelResponse, error) {
//    cancelUrl := `https://api.resy.com/3/cancel`
//    resyToken := url.QueryEscape(params.ResyToken)
//    requestBodyStr := "resy_token=" + resyToken
//    request, err := http.NewRequest("POST", cancelUrl, bytes.NewBuffer([]byte(requestBodyStr)))
//    if err != nil {
//        return nil, err
//    }
//
//    request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
//    request.Header.Set("Authorization", `ResyAPI api_key="` + a.APIKey + `"`)
//    request.Header.Set("X-Resy-Auth-Token", params.AuthToken)
//    request.Header.Set("X-Resy-Universal-Auth-Token", params.AuthToken)
//    request.Header.Set("Referer", "https://resy.com/")
//    request.Header.Set("Origin", "https://resy.com")
//
//
//    client := &http.Client{}
//    response, err := client.Do(request)
//    if err != nil {
//        return nil, err
//    }
//
//    if isCodeFail(response.StatusCode) {
//        return nil, api.ErrNetwork
//    }
//
//    responseBody, err := io.ReadAll(response.Body)
//    if err != nil {
//        return nil, err
//    }
//
//    defer response.Body.Close()
//    var jsonTopLevelMap map[string]interface{}
//    err = json.Unmarshal(responseBody, &jsonTopLevelMap)
//    if err != nil {
//        return nil, err
//    }
//
//    jsonPaymentMap := jsonTopLevelMap["payment"].(map[string]interface{})
//    jsonTransactionMap := jsonPaymentMap["transaction"].(map[string]interface{})
//    refund := jsonTransactionMap["refund"].(int) == 1
//    return &api.CancelResponse{Refund: refund}, nil
//}
//
