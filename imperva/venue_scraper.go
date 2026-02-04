package imperva

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/21Bruce/resolved-server/config"
	"github.com/21Bruce/resolved-server/store"
	"github.com/chromedp/chromedp"
)

// ScrapeBookingWindow fetches a venue page and extracts booking window information
func ScrapeBookingWindow(venueID int64) (*store.BookingWindow, error) {
	return ScrapeBookingWindowWithRetry(venueID, 3)
}

// ScrapeBookingWindowWithRetry attempts to scrape with retry logic
func ScrapeBookingWindowWithRetry(venueID int64, maxRetries int) (*store.BookingWindow, error) {
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("Booking window scrape attempt %d/%d for venue %d", attempt+1, maxRetries, venueID)
			time.Sleep(time.Duration(attempt*2) * time.Second)
		}

		bw, err := scrapeBookingWindowOnce(venueID)
		if err == nil {
			return bw, nil
		}

		lastErr = err
		log.Printf("Booking window scrape attempt %d failed for venue %d: %v", attempt+1, venueID, err)
	}

	return nil, fmt.Errorf("failed to scrape booking window after %d attempts: %w", maxRetries, lastErr)
}

// scrapeBookingWindowOnce performs a single scrape attempt
func scrapeBookingWindowOnce(venueID int64) (*store.BookingWindow, error) {
	venueURL := FetchCookiesVenueURL(venueID)
	// #region agent log
	appendDebugLog("A", "imperva/venue_scraper.go:scrapeBookingWindowOnce", "constructed venue URL", map[string]interface{}{
		"venue_id":  venueID,
		"venue_url": venueURL,
	})
	// #endregion agent log

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	opts := buildChromeOptions()
	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	chromeCtx, chromeCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer chromeCancel()

	var nextDataJSON string
	var pageHTML string

	err := chromedp.Run(chromeCtx,
		chromedp.Navigate(venueURL),
		chromedp.Sleep(5*time.Second),
		chromedp.WaitVisible("body", chromedp.ByQuery),
		chromedp.Sleep(3*time.Second),
		// Try to get __NEXT_DATA__ script content
		chromedp.Evaluate(`
			(function() {
				var el = document.getElementById('__NEXT_DATA__');
				return el ? el.textContent : '';
			})()
		`, &nextDataJSON),
		// Also get page HTML for fallback parsing
		chromedp.OuterHTML("body", &pageHTML, chromedp.ByQuery),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scrape venue page: %w", err)
	}

	// #region agent log
	appendDebugLog("B", "imperva/venue_scraper.go:scrapeBookingWindowOnce", "scrape results lengths", map[string]interface{}{
		"next_data_length": len(nextDataJSON),
		"html_length":      len(pageHTML),
	})
	// #endregion agent log
	htmlLower := strings.ToLower(pageHTML)
	botIndicators := []string{"access denied", "captcha", "are you a robot", "imperva", "incapsula", "cloudflare", "blocked"}
	foundIndicators := make([]string, 0, len(botIndicators))
	for _, indicator := range botIndicators {
		if strings.Contains(htmlLower, indicator) {
			foundIndicators = append(foundIndicators, indicator)
		}
	}
	// #region agent log
	appendDebugLog("E", "imperva/venue_scraper.go:scrapeBookingWindowOnce", "html content markers", map[string]interface{}{
		"contains_next_data":   strings.Contains(pageHTML, "__NEXT_DATA__"),
		"contains_days_phrase": strings.Contains(htmlLower, "days in advance"),
		"bot_indicators":       foundIndicators,
	})
	// #endregion agent log

	// Try to parse from __NEXT_DATA__ first
	if nextDataJSON != "" {
		bw, err := parseNextData(venueID, nextDataJSON)
		if err == nil {
			return bw, nil
		}
		log.Printf("Failed to parse __NEXT_DATA__, trying HTML fallback: %v", err)
	}

	// Fallback: parse from page HTML
	bw, err := parseHTMLContent(venueID, pageHTML)
	if err != nil {
		return nil, fmt.Errorf("failed to extract booking window from page: %w", err)
	}

	return bw, nil
}

// parseNextData extracts booking window from Resy's __NEXT_DATA__ JSON
func parseNextData(venueID int64, jsonStr string) (*store.BookingWindow, error) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, fmt.Errorf("failed to parse __NEXT_DATA__ JSON: %w", err)
	}

	// Navigate the JSON structure to find booking window info
	// Structure is typically: props.pageProps.venue or similar
	props, ok := data["props"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("props not found in __NEXT_DATA__")
	}

	pageProps, ok := props["pageProps"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("pageProps not found")
	}

	// Look for venue config/availability info
	// The exact structure may vary, so we search recursively
	bw := &store.BookingWindow{
		VenueID:   venueID,
		Timezone:  "America/New_York", // Default for NYC venues
		ScrapedAt: time.Now().UTC(),
	}

	// Try to find booking window info in various locations
	if err := extractBookingWindowFromMap(pageProps, bw); err != nil {
		return nil, err
	}

	// Validate we found the required info
	if bw.DaysInAdvance == 0 {
		// #region agent log
		appendDebugLog("C", "imperva/venue_scraper.go:parseNextData", "days_in_advance missing in next data", map[string]interface{}{
			"venue_id":       venueID,
			"top_level_keys": len(data),
			"release_hour":   bw.ReleaseHour,
			"release_minute": bw.ReleaseMinute,
		})
		// #endregion agent log
		return nil, fmt.Errorf("could not find days_in_advance in JSON")
	}

	return bw, nil
}

// extractBookingWindowFromMap recursively searches for booking window fields
func extractBookingWindowFromMap(data map[string]interface{}, bw *store.BookingWindow) error {
	// Common field names to look for
	daysFields := []string{"days_in_advance", "daysInAdvance", "advance_days", "booking_window", "bookingWindow"}
	timeFields := []string{"release_time", "releaseTime", "open_time", "openTime", "notify_time"}

	for key, value := range data {
		keyLower := strings.ToLower(key)

		// Check for days in advance
		for _, field := range daysFields {
			if strings.Contains(keyLower, strings.ToLower(field)) {
				if num, ok := value.(float64); ok {
					bw.DaysInAdvance = int(num)
				}
			}
		}

		// Check for release time
		for _, field := range timeFields {
			if strings.Contains(keyLower, strings.ToLower(field)) {
				if timeStr, ok := value.(string); ok {
					parseReleaseTime(timeStr, bw)
				}
			}
		}

		// Check nested venue object
		if keyLower == "venue" || keyLower == "venueinfo" || keyLower == "config" {
			if nested, ok := value.(map[string]interface{}); ok {
				extractBookingWindowFromMap(nested, bw)
			}
		}

		// Check availability object
		if keyLower == "availability" || keyLower == "schedule" || keyLower == "booking" {
			if nested, ok := value.(map[string]interface{}); ok {
				extractBookingWindowFromMap(nested, bw)
			}
		}

		// Recurse into nested maps
		if nested, ok := value.(map[string]interface{}); ok {
			extractBookingWindowFromMap(nested, bw)
		}
	}

	return nil
}

// parseReleaseTime parses time strings like "9:00 AM", "09:00", etc.
func parseReleaseTime(timeStr string, bw *store.BookingWindow) {
	timeStr = strings.TrimSpace(timeStr)

	// Try various formats
	formats := []string{
		"3:04 PM",
		"3:04PM",
		"15:04",
		"3PM",
		"3 PM",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, timeStr); err == nil {
			bw.ReleaseHour = t.Hour()
			bw.ReleaseMinute = t.Minute()
			return
		}
	}

	// Try regex for patterns like "9am", "9:00am"
	re := regexp.MustCompile(`(\d{1,2}):?(\d{2})?\s*(am|pm|AM|PM)?`)
	matches := re.FindStringSubmatch(timeStr)
	if len(matches) >= 2 {
		hour, _ := strconv.Atoi(matches[1])
		minute := 0
		if len(matches) >= 3 && matches[2] != "" {
			minute, _ = strconv.Atoi(matches[2])
		}
		isPM := len(matches) >= 4 && strings.ToLower(matches[3]) == "pm"
		if isPM && hour != 12 {
			hour += 12
		} else if !isPM && hour == 12 {
			hour = 0
		}
		bw.ReleaseHour = hour
		bw.ReleaseMinute = minute
	}
}

// parseHTMLContent extracts booking window from page HTML as fallback
func parseHTMLContent(venueID int64, html string) (*store.BookingWindow, error) {
	bw := &store.BookingWindow{
		VenueID:       venueID,
		Timezone:      "America/New_York",
		ReleaseHour:   9, // Default to 9 AM if not found
		ReleaseMinute: 0,
		ScrapedAt:     time.Now().UTC(),
	}

	// Look for patterns like "Book up to X days in advance"
	daysPatterns := []string{
		`(\d+)\s*days?\s*in\s*advance`,
		`book\s*(?:up\s*to\s*)?(\d+)\s*days?`,
		`(\d+)\s*day\s*booking\s*window`,
		`reservations?\s*(?:open|available)\s*(\d+)\s*days?`,
	}

	htmlLower := strings.ToLower(html)
	for _, pattern := range daysPatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(htmlLower)
		if len(matches) >= 2 {
			days, err := strconv.Atoi(matches[1])
			if err == nil && days > 0 && days <= 365 {
				bw.DaysInAdvance = days
				break
			}
		}
	}

	// Look for release time patterns
	timePatterns := []string{
		`(?:open|released?|available)\s*(?:at|@)\s*(\d{1,2}):?(\d{2})?\s*(am|pm)?`,
		`(\d{1,2}):?(\d{2})?\s*(am|pm)\s*(?:daily|every\s*day)`,
	}

	for _, pattern := range timePatterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(htmlLower)
		if len(matches) >= 2 {
			hour, _ := strconv.Atoi(matches[1])
			minute := 0
			if len(matches) >= 3 && matches[2] != "" {
				minute, _ = strconv.Atoi(matches[2])
			}
			isPM := len(matches) >= 4 && matches[3] == "pm"
			if isPM && hour != 12 {
				hour += 12
			} else if !isPM && strings.Contains(matches[0], "am") && hour == 12 {
				hour = 0
			}
			bw.ReleaseHour = hour
			bw.ReleaseMinute = minute
			break
		}
	}

	if bw.DaysInAdvance == 0 {
		// #region agent log
		appendDebugLog("D", "imperva/venue_scraper.go:parseHTMLContent", "days_in_advance not found in HTML", map[string]interface{}{
			"venue_id":    venueID,
			"html_length": len(html),
		})
		// #endregion agent log
		return nil, fmt.Errorf("could not find booking window days in page content")
	}

	log.Printf("Extracted booking window from HTML: %d days in advance, release at %02d:%02d",
		bw.DaysInAdvance, bw.ReleaseHour, bw.ReleaseMinute)

	return bw, nil
}

func resolveVenueSlug(venueID int64) string {
	cfg := config.Get()
	for _, venue := range cfg.Venues {
		if venue.ID == venueID && venue.Slug != "" {
			return venue.Slug
		}
	}
	return ""
}

func appendDebugLog(hypothesisId, location, message string, data map[string]interface{}) {
	logPath := os.Getenv("DEBUG_LOG_PATH")
	if logPath == "" {
		return
	}
	payload := map[string]interface{}{
		"sessionId":    "debug-session",
		"runId":        "pre-fix",
		"hypothesisId": hypothesisId,
		"location":     location,
		"message":      message,
		"data":         data,
		"timestamp":    time.Now().UnixMilli(),
	}
	line, err := json.Marshal(payload)
	if err != nil {
		return
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

// GetOrScrapeBookingWindow gets cached booking window or scrapes if not available
func GetOrScrapeBookingWindow(ctx context.Context, venueID int64) (*store.BookingWindow, error) {
	// Check cache first
	bw, err := store.GetBookingWindow(ctx, venueID)
	if err == nil {
		log.Printf("Using cached booking window for venue %d", venueID)
		return bw, nil
	}

	// Not cached, scrape it
	log.Printf("Scraping booking window for venue %d", venueID)
	bw, err = ScrapeBookingWindow(venueID)
	if err != nil {
		return nil, err
	}

	// Cache it
	if err := store.SaveBookingWindow(ctx, bw); err != nil {
		log.Printf("Warning: failed to cache booking window for venue %d: %v", venueID, err)
	}

	return bw, nil
}
