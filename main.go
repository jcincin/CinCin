// main.go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/21Bruce/resolved-server/api"
	"github.com/21Bruce/resolved-server/api/resy"
	"github.com/21Bruce/resolved-server/app"
	"github.com/21Bruce/resolved-server/config"
	"github.com/21Bruce/resolved-server/imperva"
	"github.com/21Bruce/resolved-server/store"
	"github.com/gorilla/securecookie"
)

// Maximum number of log lines to keep in memory
const maxLogLines = 500

// Structures for JSON responses
type SearchResponse struct {
	Results []api.SearchResult `json:"results"`
	Error   string             `json:"error,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginResponse struct {
	AuthToken string `json:"auth_token,omitempty"`
	VenueID   int64  `json:"venue_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

type ReserveRequest struct {
	VenueID          int64    `json:"venue_id"`
	ReservationTime  string   `json:"reservation_time"` // datetime-local format in NYC time: YYYY-MM-DDTHH:MM
	PartySize        int      `json:"party_size"`
	TablePreferences []string `json:"table_preferences"`
	IsImmediate      bool     `json:"is_immediate"`
	RequestTime      string   `json:"request_time"`  // datetime-local format in NYC time: YYYY-MM-DDTHH:MM
	AutoSchedule     bool     `json:"auto_schedule"` // If true, automatically calculate optimal run time from venue's booking window
}

type ReserveResponse struct {
	ReservationTime string `json:"reservation_time,omitempty"`
	ReservationID   string `json:"reservation_id,omitempty"`
	ScheduledFor    string `json:"scheduled_for,omitempty"` // When the sniper will run (for auto_schedule)
	Error           string `json:"error,omitempty"`
}

type BookingWindowResponse struct {
	VenueID       int64  `json:"venue_id"`
	DaysInAdvance int    `json:"days_in_advance"`
	ReleaseTime   string `json:"release_time"` // e.g., "09:00"
	Timezone      string `json:"timezone"`
	Error         string `json:"error,omitempty"`
}

type SelectVenueRequest struct {
	VenueID int64 `json:"venue_id"`
}

type SelectVenueResponse struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Admin request/response types
type CookieImportRequest struct {
	VenueID   int64        `json:"venue_id"`
	Cookies   []CookieData `json:"cookies"`
	UserAgent string       `json:"user_agent"`
	TTLHours  int          `json:"ttl_hours"` // Optional, defaults to 24
}

type CookieData struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain"`
	Path   string `json:"path"`
}

type CookieStatusResponse struct {
	VenueID   int64     `json:"venue_id"`
	Exists    bool      `json:"exists"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	TTL       string    `json:"ttl,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type HealthResponse struct {
	Status string `json:"status"`
	Redis  string `json:"redis"`
}

// Reservation list response types
type ReservationListResponse struct {
	Reservations []ReservationSummary `json:"reservations"`
	Error        string               `json:"error,omitempty"`
}

type ReservationSummary struct {
	ID               string   `json:"id"`
	VenueID          int64    `json:"venue_id"`
	VenueName        string   `json:"venue_name"`
	ReservationTime  string   `json:"reservation_time"`
	PartySize        int      `json:"party_size"`
	RunTime          string   `json:"run_time"`
	CreatedAt        string   `json:"created_at"`
	TablePreferences []string `json:"table_preferences"`
}

type CancelReservationResponse struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type AdminStatusResponse struct {
	Venues              []VenueStatus `json:"venues"`
	PendingReservations int64         `json:"pending_reservations"`
	Error               string        `json:"error,omitempty"`
}

type VenueStatus struct {
	VenueID      int64  `json:"venue_id"`
	CookieStatus string `json:"cookie_status"`
	TTL          string `json:"ttl,omitempty"`
}

// Resy link request/response types
type ResyLinkRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type ResyLinkResponse struct {
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type ResyStatusResponse struct {
	Linked bool   `json:"linked"`
	Error  string `json:"error,omitempty"`
}

var s *securecookie.SecureCookie

// In-memory log lines
var logLines []string
var logMu sync.Mutex

// NYC timezone for parsing user input times
var nycLocation *time.Location

// Venue name lookup map (loaded from venues.json)
var venueNames map[int64]string

func loadVenueNames() {
	venueNames = make(map[int64]string)
	data, err := os.ReadFile("venues.json")
	if err != nil {
		log.Printf("Warning: Could not load venues.json: %v", err)
		return
	}
	var venues struct {
		Venues []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"venues"`
	}
	if err := json.Unmarshal(data, &venues); err != nil {
		log.Printf("Warning: Could not parse venues.json: %v", err)
		return
	}
	for _, v := range venues.Venues {
		venueNames[v.ID] = v.Name
	}
}

func getVenueName(venueID int64) string {
	if name, ok := venueNames[venueID]; ok {
		return name
	}
	return fmt.Sprintf("Venue %d", venueID)
}

func init() {
	// Load NYC timezone
	var err error
	nycLocation, err = time.LoadLocation("America/New_York")
	if err != nil {
		log.Fatalf("Failed to load NYC timezone: %v", err)
	}

	// Load venue names for lookup
	loadVenueNames()

	cfg := config.Get()
	if cfg.CookieSecretKey != nil && cfg.CookieBlockKey != nil {
		s = securecookie.New(cfg.CookieSecretKey, cfg.CookieBlockKey)
	} else {
		// Generate random keys if not configured (sessions won't survive restarts)
		s = securecookie.New(securecookie.GenerateRandomKey(32), securecookie.GenerateRandomKey(32))
	}
}

func main() {
	cfg := config.Get()

	resyAPI := resy.GetDefaultAPI()
	appCtx := app.AppCtx{API: &resyAPI}

	// Health endpoint
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		redisStatus := "connected"
		if err := store.Ping(ctx); err != nil {
			redisStatus = "disconnected"
		}
		sendJSONResponse(w, HealthResponse{
			Status: "ok",
			Redis:  redisStatus,
		}, http.StatusOK)
	})

	// Admin endpoints - protected by ADMIN_TOKEN
	http.HandleFunc("/admin/cookies/import", requireInternalToken(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if !validateAdminToken(r, cfg) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req CookieImportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sendJSONResponse(w, map[string]string{"error": "Invalid request format"}, http.StatusBadRequest)
			return
		}

		if req.VenueID == 0 {
			sendJSONResponse(w, map[string]string{"error": "venue_id is required"}, http.StatusBadRequest)
			return
		}

		// Convert to http.Cookie
		httpCookies := make([]*http.Cookie, len(req.Cookies))
		for i, c := range req.Cookies {
			httpCookies[i] = &http.Cookie{
				Name:   c.Name,
				Value:  c.Value,
				Domain: c.Domain,
				Path:   c.Path,
			}
		}

		ttl := 24 * time.Hour
		if req.TTLHours > 0 {
			ttl = time.Duration(req.TTLHours) * time.Hour
		}

		ctx := context.Background()
		if err := store.SaveCookies(ctx, req.VenueID, httpCookies, req.UserAgent, ttl); err != nil {
			appendLog("Failed to save cookies for venue " + strconv.FormatInt(req.VenueID, 10) + ": " + err.Error())
			sendJSONResponse(w, map[string]string{"error": "Failed to save cookies: " + err.Error()}, http.StatusInternalServerError)
			return
		}

		appendLog("Imported " + strconv.Itoa(len(httpCookies)) + " cookies for venue " + strconv.FormatInt(req.VenueID, 10))
		sendJSONResponse(w, map[string]string{"message": "Cookies imported successfully"}, http.StatusOK)
	}, cfg))

	http.HandleFunc("/admin/cookies/", requireInternalToken(func(w http.ResponseWriter, r *http.Request) {
		if !validateAdminToken(r, cfg) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Extract venue ID from path: /admin/cookies/{venue_id}
		pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/admin/cookies/"), "/")
		if len(pathParts) == 0 || pathParts[0] == "" {
			http.Error(w, "Venue ID required", http.StatusBadRequest)
			return
		}

		venueID, err := strconv.ParseInt(pathParts[0], 10, 64)
		if err != nil {
			http.Error(w, "Invalid venue ID", http.StatusBadRequest)
			return
		}

		ctx := context.Background()

		switch r.Method {
		case http.MethodGet:
			exists, err := store.CookieExists(ctx, venueID)
			if err != nil {
				sendJSONResponse(w, CookieStatusResponse{VenueID: venueID, Error: err.Error()}, http.StatusInternalServerError)
				return
			}

			resp := CookieStatusResponse{VenueID: venueID, Exists: exists}
			if exists {
				ttl, _ := store.GetCookieTTL(ctx, venueID)
				resp.TTL = ttl.String()
				cookieData, _ := store.GetCookies(ctx, venueID)
				if cookieData != nil {
					resp.ExpiresAt = cookieData.ExpiresAt
				}
			}
			sendJSONResponse(w, resp, http.StatusOK)

		case http.MethodDelete:
			if err := store.DeleteCookies(ctx, venueID); err != nil {
				sendJSONResponse(w, map[string]string{"error": err.Error()}, http.StatusInternalServerError)
				return
			}
			appendLog("Deleted cookies for venue " + strconv.FormatInt(venueID, 10))
			sendJSONResponse(w, map[string]string{"message": "Cookies deleted"}, http.StatusOK)

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}, cfg))

	http.HandleFunc("/admin/status", requireInternalToken(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if !validateAdminToken(r, cfg) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := context.Background()

		// Get pending reservation count
		pendingCount, err := store.CountPendingReservations(ctx)
		if err != nil {
			sendJSONResponse(w, AdminStatusResponse{Error: err.Error()}, http.StatusInternalServerError)
			return
		}

		// Get venue IDs from config
		venueIDs := cfg.VenueIDs()
		venues := make([]VenueStatus, 0, len(venueIDs))

		for _, venueID := range venueIDs {
			status := VenueStatus{VenueID: venueID}
			exists, _ := store.CookieExists(ctx, venueID)
			if exists {
				ttl, _ := store.GetCookieTTL(ctx, venueID)
				status.CookieStatus = "valid"
				status.TTL = ttl.String()
			} else {
				status.CookieStatus = "missing"
			}
			venues = append(venues, status)
		}

		sendJSONResponse(w, AdminStatusResponse{
			Venues:              venues,
			PendingReservations: pendingCount,
		}, http.StatusOK)
	}, cfg))

	// Search API endpoint
	http.HandleFunc("/api/search", requireInternalToken(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var searchRequest struct {
			Name  string `json:"name"`
			Limit int    `json:"limit"`
		}

		if err := json.NewDecoder(r.Body).Decode(&searchRequest); err != nil {
			sendJSONResponse(w, SearchResponse{Error: "Invalid request format"}, http.StatusBadRequest)
			return
		}

		searchParam := api.SearchParam{
			Name:  searchRequest.Name,
			Limit: searchRequest.Limit,
		}

		results, err := appCtx.API.Search(searchParam)
		if err != nil {
			sendJSONResponse(w, SearchResponse{Error: err.Error()}, http.StatusInternalServerError)
			return
		}

		sendJSONResponse(w, SearchResponse{Results: results.Results}, http.StatusOK)
	}, cfg))

	// Select Venue API endpoint
	http.HandleFunc("/api/select-venue", requireInternalToken(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var selectReq SelectVenueRequest
		if err := json.NewDecoder(r.Body).Decode(&selectReq); err != nil {
			sendJSONResponse(w, SelectVenueResponse{Error: "Invalid request format"}, http.StatusBadRequest)
			return
		}

		session, err := getSession(r)
		if err != nil {
			session = make(map[string]string)
		}

		session["venue_id"] = strconv.FormatInt(selectReq.VenueID, 10)

		encoded, err := s.Encode("session", session)
		if err != nil {
			sendJSONResponse(w, SelectVenueResponse{Error: "Failed to encode session"}, http.StatusInternalServerError)
			return
		}

		cookie := &http.Cookie{
			Name:     "session",
			Value:    encoded,
			Path:     "/",
			HttpOnly: true,
			Secure:   isSecureRequest(r),
		}
		http.SetCookie(w, cookie)

		sendJSONResponse(w, SelectVenueResponse{Message: "Venue selected successfully"}, http.StatusOK)
	}, cfg))

	// Login API endpoint
	http.HandleFunc("/api/login", requireInternalToken(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var loginReq LoginRequest
		if err := json.NewDecoder(r.Body).Decode(&loginReq); err != nil {
			sendJSONResponse(w, LoginResponse{Error: "Invalid request format"}, http.StatusBadRequest)
			return
		}

		loginParam := api.LoginParam{
			Email:    loginReq.Email,
			Password: loginReq.Password,
		}

		loginResp, err := appCtx.API.Login(loginParam)
		if err != nil {
			switch err {
			case api.ErrLoginWrong:
				sendJSONResponse(w, LoginResponse{Error: "Incorrect email or password"}, http.StatusUnauthorized)
			case api.ErrNetwork:
				sendJSONResponse(w, LoginResponse{Error: "Network error. Please try again later."}, http.StatusInternalServerError)
			case api.ErrNoPayInfo:
				sendJSONResponse(w, LoginResponse{Error: "No payment information found. Please update your account."}, http.StatusBadRequest)
			case api.ErrImperva:
				sendJSONResponse(w, LoginResponse{Error: "Imperva challenge: please refresh cookies via /admin/cookies/import"}, http.StatusServiceUnavailable)
			default:
				sendJSONResponse(w, LoginResponse{Error: "An unexpected error occurred."}, http.StatusInternalServerError)
			}
			return
		}

		value := map[string]string{
			"auth_token":        loginResp.AuthToken,
			"payment_method_id": strconv.FormatInt(loginResp.PaymentMethodID, 10),
		}
		encoded, err := s.Encode("session", value)
		if err != nil {
			sendJSONResponse(w, LoginResponse{Error: "Failed to set session"}, http.StatusInternalServerError)
			return
		}

		cookie := &http.Cookie{
			Name:     "session",
			Value:    encoded,
			Path:     "/",
			HttpOnly: true,
			Secure:   isSecureRequest(r),
		}
		http.SetCookie(w, cookie)

		sendJSONResponse(w, LoginResponse{
			AuthToken: loginResp.AuthToken,
		}, http.StatusOK)
	}, cfg))

	// Reserve API endpoint
	http.HandleFunc("/api/reserve", requireInternalToken(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var reserveReq ReserveRequest
		if err := json.NewDecoder(r.Body).Decode(&reserveReq); err != nil {
			sendJSONResponse(w, ReserveResponse{Error: "Invalid request format"}, http.StatusBadRequest)
			return
		}

		var authToken string
		var paymentMethodID int64
		var clerkUserID string
		var session map[string]string

		// Check for Clerk user ID header (new auth flow)
		clerkUserID = r.Header.Get("X-Clerk-User-Id")
		if clerkUserID != "" {
			// Fetch Resy credentials from Redis
			ctx := context.Background()
			creds, err := store.GetResyCredentials(ctx, clerkUserID)
			if err != nil {
				sendJSONResponse(w, ReserveResponse{Error: "Resy account not linked. Please link your Resy account first."}, http.StatusUnauthorized)
				return
			}
			authToken = creds.AuthToken
			paymentMethodID = creds.PaymentMethodID
		} else {
			// Fallback to session-based auth (legacy flow)
			var err error
			session, err = getSession(r)
			if err != nil {
				sendJSONResponse(w, ReserveResponse{Error: "Unauthorized. Please log in."}, http.StatusUnauthorized)
				return
			}

			var ok bool
			authToken, ok = session["auth_token"]
			if !ok || authToken == "" {
				sendJSONResponse(w, ReserveResponse{Error: "Authentication token missing. Please log in."}, http.StatusUnauthorized)
				return
			}

			// Get payment method ID from session
			if pmIDStr, ok := session["payment_method_id"]; ok && pmIDStr != "" {
				paymentMethodID, _ = strconv.ParseInt(pmIDStr, 10, 64)
			}
		}

		venueID := reserveReq.VenueID
		if venueID == 0 {
			// Only try session lookup for legacy flow (non-Clerk users)
			if session == nil {
				sendJSONResponse(w, ReserveResponse{Error: "Venue ID missing. Please select a restaurant."}, http.StatusBadRequest)
				return
			}
			venueIDStr, ok := session["venue_id"]
			if !ok || venueIDStr == "" {
				sendJSONResponse(w, ReserveResponse{Error: "Venue ID missing. Please select a restaurant first."}, http.StatusBadRequest)
				return
			}
			parsedVenueID, err := strconv.ParseInt(venueIDStr, 10, 64)
			if err != nil {
				sendJSONResponse(w, ReserveResponse{Error: "Invalid Venue ID"}, http.StatusBadRequest)
				return
			}
			venueID = parsedVenueID
		}

		// Parse the reservation time (NYC timezone, converted to UTC)
		reservationTime, err := parseTimeNYC(reserveReq.ReservationTime)
		if err != nil {
			sendJSONResponse(w, ReserveResponse{Error: "Invalid reservation time format. Use YYYY-MM-DDTHH:MM"}, http.StatusBadRequest)
			return
		}

		var requestTime time.Time
		if !reserveReq.IsImmediate {
			if reserveReq.AutoSchedule {
				// Auto-calculate run time from venue's booking window
				ctx := context.Background()
				bw, err := imperva.GetOrScrapeBookingWindow(ctx, venueID)
				if err != nil {
					appendLog("Failed to get booking window for venue " + strconv.FormatInt(venueID, 10) + ": " + err.Error())
					sendJSONResponse(w, ReserveResponse{Error: "Failed to determine booking window: " + err.Error()}, http.StatusInternalServerError)
					return
				}

				requestTime, err = bw.CalculateRunTime(reservationTime)
				if err != nil {
					sendJSONResponse(w, ReserveResponse{Error: "Failed to calculate run time: " + err.Error()}, http.StatusInternalServerError)
					return
				}

				appendLog("Auto-scheduled: venue " + strconv.FormatInt(venueID, 10) + " opens " + strconv.Itoa(bw.DaysInAdvance) + " days ahead at " + strconv.Itoa(bw.ReleaseHour) + ":" + fmt.Sprintf("%02d", bw.ReleaseMinute))
			} else {
				requestTime, err = parseTimeNYC(reserveReq.RequestTime)
				if err != nil {
					sendJSONResponse(w, ReserveResponse{Error: "Invalid request time format. Use YYYY-MM-DDTHH:MM"}, http.StatusBadRequest)
					return
				}
			}
		}

		// Convert table preferences
		var tableTypes []api.TableType
		for _, pref := range reserveReq.TablePreferences {
			tableTypes = append(tableTypes, api.TableType(pref))
		}

		if reserveReq.IsImmediate {
			// Attempt reservation now
			reserveParam := api.ReserveParam{
				VenueID:          venueID,
				ReservationTimes: []time.Time{reservationTime},
				PartySize:        reserveReq.PartySize,
				LoginResp:        api.LoginResponse{AuthToken: authToken, PaymentMethodID: paymentMethodID},
				TableTypes:       tableTypes,
			}

			appendLog("Attempting immediate reservation for venue " + strconv.FormatInt(venueID, 10))
			appendLog("Reservation details: party_size=" + strconv.Itoa(reserveReq.PartySize) + ", time=" + reservationTime.Format("2006-01-02 15:04"))
			if paymentMethodID == 0 {
				appendLog("Warning: No payment method ID found in session - booking step may fail")
			}
			reserveResp, err := appCtx.API.Reserve(reserveParam)
			if err != nil {
				appendLog("Immediate reservation failed: " + err.Error())

				// Check for specific error types using errors.Is/As
				var netErr *api.NetworkError
				if errors.As(err, &netErr) {
					appendLog("Network error details - Step: " + netErr.Step + ", Status: " + strconv.Itoa(netErr.Status) + ", Message: " + netErr.Message)
					sendJSONResponse(w, ReserveResponse{Error: "Network error at " + netErr.Step + " step: " + netErr.Message}, http.StatusInternalServerError)
				} else if errors.Is(err, api.ErrNetwork) {
					sendJSONResponse(w, ReserveResponse{Error: "Network error. Please try again later."}, http.StatusInternalServerError)
				} else if errors.Is(err, api.ErrNoTable) {
					sendJSONResponse(w, ReserveResponse{Error: "No available tables found for the selected time."}, http.StatusBadRequest)
				} else if errors.Is(err, api.ErrImperva) {
					sendJSONResponse(w, ReserveResponse{Error: "Imperva challenge: please refresh cookies via /admin/cookies/import"}, http.StatusServiceUnavailable)
				} else if errors.Is(err, api.ErrNoOffer) {
					sendJSONResponse(w, ReserveResponse{Error: "No reservations available for this date."}, http.StatusBadRequest)
				} else {
					sendJSONResponse(w, ReserveResponse{Error: "An unexpected error occurred: " + err.Error()}, http.StatusInternalServerError)
				}
				return
			}

			appendLog("Immediate reservation successful")
			sendJSONResponse(w, ReserveResponse{
				ReservationTime: reserveResp.ReservationTime.In(nycLocation).Format("2006-01-02 3:04 PM EST"),
			}, http.StatusOK)
		} else {
			// Schedule for later - save to Redis
			ctx := context.Background()
			resID := store.GenerateReservationID()

			usageType := "immediate"
			if reserveReq.AutoSchedule {
				usageType = "concierge"
			}

			scheduledRes := &store.ScheduledReservation{
				ID:               resID,
				VenueID:          venueID,
				ReservationTime:  reservationTime,
				PartySize:        reserveReq.PartySize,
				TablePreferences: reserveReq.TablePreferences,
				AuthToken:        authToken,
				PaymentMethodID:  paymentMethodID,
				ClerkUserID:      clerkUserID,
				UsageType:        usageType,
				RunTime:          requestTime,
				CreatedAt:        time.Now().UTC(),
			}

			if err := store.SaveReservation(ctx, scheduledRes); err != nil {
				appendLog("Failed to schedule reservation: " + err.Error())
				sendJSONResponse(w, ReserveResponse{Error: "Failed to schedule reservation: " + err.Error()}, http.StatusInternalServerError)
				return
			}

			appendLog("Scheduled reservation " + resID + " for: " + requestTime.In(nycLocation).Format("2006-01-02 3:04 PM EST"))
			sendJSONResponse(w, ReserveResponse{
				ReservationID: resID,
				ScheduledFor:  requestTime.In(nycLocation).Format("2006-01-02 3:04 PM EST"),
			}, http.StatusOK)
		}
	}, cfg))

	// Booking window endpoint - get or scrape booking window for a venue
	http.HandleFunc("/api/booking-window/", requireInternalToken(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract venue ID from path: /api/booking-window/{venue_id}
		pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/booking-window/"), "/")
		if len(pathParts) == 0 || pathParts[0] == "" {
			sendJSONResponse(w, BookingWindowResponse{Error: "Venue ID required"}, http.StatusBadRequest)
			return
		}

		venueID, err := strconv.ParseInt(pathParts[0], 10, 64)
		if err != nil {
			sendJSONResponse(w, BookingWindowResponse{Error: "Invalid venue ID"}, http.StatusBadRequest)
			return
		}

		ctx := context.Background()
		bw, err := imperva.GetOrScrapeBookingWindow(ctx, venueID)
		if err != nil {
			appendLog("Failed to get booking window for venue " + strconv.FormatInt(venueID, 10) + ": " + err.Error())
			sendJSONResponse(w, BookingWindowResponse{Error: "Failed to get booking window: " + err.Error()}, http.StatusInternalServerError)
			return
		}

		sendJSONResponse(w, BookingWindowResponse{
			VenueID:       bw.VenueID,
			DaysInAdvance: bw.DaysInAdvance,
			ReleaseTime:   fmt.Sprintf("%02d:%02d", bw.ReleaseHour, bw.ReleaseMinute),
			Timezone:      bw.Timezone,
		}, http.StatusOK)
	}, cfg))

	// List all scheduled reservations
	http.HandleFunc("/api/reservations", requireInternalToken(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx := context.Background()
		clerkUserID := r.Header.Get("X-Clerk-User-Id")

		var reservations []*store.ScheduledReservation
		var err error

		if clerkUserID != "" {
			// Filter by Clerk user ID
			reservations, err = store.GetReservationsByClerkUser(ctx, clerkUserID)
		} else {
			// Return all (legacy behavior for admin/testing)
			reservations, err = store.GetAllPendingReservations(ctx)
		}

		if err != nil {
			sendJSONResponse(w, ReservationListResponse{Error: "Failed to fetch reservations"}, http.StatusInternalServerError)
			return
		}

		summaries := make([]ReservationSummary, 0, len(reservations))
		for _, res := range reservations {
			summaries = append(summaries, ReservationSummary{
				ID:               res.ID,
				VenueID:          res.VenueID,
				VenueName:        getVenueName(res.VenueID),
				ReservationTime:  res.ReservationTime.In(nycLocation).Format("2006-01-02 3:04 PM"),
				PartySize:        res.PartySize,
				RunTime:          res.RunTime.In(nycLocation).Format("2006-01-02 3:04 PM EST"),
				CreatedAt:        res.CreatedAt.In(nycLocation).Format("2006-01-02 3:04 PM"),
				TablePreferences: res.TablePreferences,
			})
		}

		sendJSONResponse(w, ReservationListResponse{Reservations: summaries}, http.StatusOK)
	}, cfg))

	// Cancel a scheduled reservation
	http.HandleFunc("/api/reservations/", requireInternalToken(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract reservation ID from path: /api/reservations/{id}
		path := strings.TrimPrefix(r.URL.Path, "/api/reservations/")
		if path == "" {
			sendJSONResponse(w, CancelReservationResponse{Error: "Reservation ID required"}, http.StatusBadRequest)
			return
		}
		resID := path

		ctx := context.Background()
		clerkUserID := r.Header.Get("X-Clerk-User-Id")

		// Check if reservation exists
		res, err := store.GetReservation(ctx, resID)
		if err != nil {
			sendJSONResponse(w, CancelReservationResponse{Error: "Reservation not found"}, http.StatusNotFound)
			return
		}

		// Verify ownership if Clerk user ID is provided
		if clerkUserID != "" && res.ClerkUserID != clerkUserID {
			sendJSONResponse(w, CancelReservationResponse{Error: "Reservation not found"}, http.StatusNotFound)
			return
		}

		// Delete the reservation
		if err := store.DeleteReservation(ctx, resID); err != nil {
			sendJSONResponse(w, CancelReservationResponse{Error: "Failed to cancel reservation"}, http.StatusInternalServerError)
			return
		}

		appendLog("Cancelled reservation: " + resID)
		sendJSONResponse(w, CancelReservationResponse{Message: "Reservation cancelled"}, http.StatusOK)
	}, cfg))

	// Logs endpoint
	http.HandleFunc("/api/logs", requireInternalToken(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		logMu.Lock()
		lines := make([]string, len(logLines))
		copy(lines, logLines)
		logMu.Unlock()
		json.NewEncoder(w).Encode(lines)
	}, cfg))

	// Resy Link endpoint - link a Resy account to a Clerk user
	http.HandleFunc("/api/resy/link", requireInternalToken(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		clerkUserID := r.Header.Get("X-Clerk-User-Id")
		if clerkUserID == "" {
			sendJSONResponse(w, ResyLinkResponse{Error: "Unauthorized"}, http.StatusUnauthorized)
			return
		}

		var linkReq ResyLinkRequest
		if err := json.NewDecoder(r.Body).Decode(&linkReq); err != nil {
			sendJSONResponse(w, ResyLinkResponse{Error: "Invalid request format"}, http.StatusBadRequest)
			return
		}

		// Authenticate with Resy
		loginParam := api.LoginParam{
			Email:    linkReq.Email,
			Password: linkReq.Password,
		}

		loginResp, err := appCtx.API.Login(loginParam)
		if err != nil {
			switch err {
			case api.ErrLoginWrong:
				sendJSONResponse(w, ResyLinkResponse{Error: "Incorrect Resy email or password"}, http.StatusUnauthorized)
			case api.ErrNetwork:
				sendJSONResponse(w, ResyLinkResponse{Error: "Network error. Please try again later."}, http.StatusInternalServerError)
			case api.ErrNoPayInfo:
				sendJSONResponse(w, ResyLinkResponse{Error: "No payment information found on your Resy account."}, http.StatusBadRequest)
			default:
				sendJSONResponse(w, ResyLinkResponse{Error: "Failed to authenticate with Resy"}, http.StatusInternalServerError)
			}
			return
		}

		// Save credentials to Redis
		ctx := context.Background()
		creds := &store.ResyCredentials{
			ClerkUserID:     clerkUserID,
			AuthToken:       loginResp.AuthToken,
			PaymentMethodID: loginResp.PaymentMethodID,
		}

		if err := store.SaveResyCredentials(ctx, creds); err != nil {
			appendLog("Failed to save Resy credentials for user " + clerkUserID + ": " + err.Error())
			sendJSONResponse(w, ResyLinkResponse{Error: "Failed to save credentials"}, http.StatusInternalServerError)
			return
		}

		appendLog("Linked Resy account for Clerk user " + clerkUserID)
		sendJSONResponse(w, ResyLinkResponse{Message: "Resy account linked successfully"}, http.StatusOK)
	}, cfg))

	// Resy Status endpoint - check if a Clerk user has linked their Resy account
	http.HandleFunc("/api/resy/status", requireInternalToken(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		clerkUserID := r.Header.Get("X-Clerk-User-Id")
		if clerkUserID == "" {
			sendJSONResponse(w, ResyStatusResponse{Error: "Unauthorized"}, http.StatusUnauthorized)
			return
		}

		ctx := context.Background()
		exists, err := store.ResyCredentialsExist(ctx, clerkUserID)
		if err != nil {
			sendJSONResponse(w, ResyStatusResponse{Error: "Failed to check status"}, http.StatusInternalServerError)
			return
		}

		sendJSONResponse(w, ResyStatusResponse{Linked: exists}, http.StatusOK)
	}, cfg))

	// Resy Unlink endpoint - remove linked Resy account
	http.HandleFunc("/api/resy/unlink", requireInternalToken(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		clerkUserID := r.Header.Get("X-Clerk-User-Id")
		if clerkUserID == "" {
			sendJSONResponse(w, ResyLinkResponse{Error: "Unauthorized"}, http.StatusUnauthorized)
			return
		}

		ctx := context.Background()
		if err := store.DeleteResyCredentials(ctx, clerkUserID); err != nil {
			appendLog("Failed to unlink Resy account for user " + clerkUserID + ": " + err.Error())
			sendJSONResponse(w, ResyLinkResponse{Error: "Failed to unlink account"}, http.StatusInternalServerError)
			return
		}

		appendLog("Unlinked Resy account for Clerk user " + clerkUserID)
		sendJSONResponse(w, ResyLinkResponse{Message: "Resy account unlinked successfully"}, http.StatusOK)
	}, cfg))

	// Create cancellable context for scheduler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start the scheduling goroutine (Redis-backed)
	go handleScheduledReservations(ctx, appCtx, cfg)

	// Start the cookie refresh goroutine (if enabled)
	if cfg.CookieRefreshEnabled {
		go handleCookieRefresh(ctx, cfg)
	}

	// Create server for graceful shutdown
	port := cfg.Port
	server := &http.Server{Addr: ":" + port}

	// Handle shutdown signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-stop
		appendLog("Shutting down gracefully...")
		cancel() // Stop scheduler

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			appendLog("Error during shutdown: " + err.Error())
		}
	}()

	// Start server
	appendLog("Starting server on port " + port + "...")
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
	appendLog("Server stopped")
}

func handleScheduledReservations(ctx context.Context, appCtx app.AppCtx, cfg *config.Config) {
	for {
		select {
		case <-ctx.Done():
			appendLog("Scheduler shutting down")
			return
		default:
			// Get the next scheduled reservation
			nextRes, err := store.GetNextReservation(ctx)
			if err != nil || nextRes == nil {
				// No pending reservations, check again in 30 seconds (shorter for faster shutdown response)
				select {
				case <-ctx.Done():
					appendLog("Scheduler shutting down")
					return
				case <-time.After(30 * time.Second):
				}
				continue
			}

			now := time.Now().UTC()

			if nextRes.RunTime.After(now) {
				// Sleep until the scheduled time (max 30 seconds to allow for faster shutdown response)
				sleepDuration := nextRes.RunTime.Sub(now)
				if sleepDuration > 30*time.Second {
					sleepDuration = 30 * time.Second
				}
				select {
				case <-ctx.Done():
					appendLog("Scheduler shutting down")
					return
				case <-time.After(sleepDuration):
				}
				continue
			}

			// Time to attempt booking
			appendLog("Attempting scheduled reservation " + nextRes.ID + " for venue " + strconv.FormatInt(nextRes.VenueID, 10))

			// Get auth credentials - refresh from Redis if Clerk user
			authToken := nextRes.AuthToken
			paymentMethodID := nextRes.PaymentMethodID
			if nextRes.ClerkUserID != "" {
				// Fetch fresh credentials from Redis for Clerk users
				creds, err := store.GetResyCredentials(ctx, nextRes.ClerkUserID)
				if err != nil {
					appendLog("Failed to get Resy credentials for user " + nextRes.ClerkUserID + ": " + err.Error())
					// Delete the reservation since we can't execute it
					store.DeleteReservation(ctx, nextRes.ID)
					continue
				}
				authToken = creds.AuthToken
				paymentMethodID = creds.PaymentMethodID
			}

			// Convert table preferences
			var tableTypes []api.TableType
			for _, pref := range nextRes.TablePreferences {
				tableTypes = append(tableTypes, api.TableType(pref))
			}

			reserveParam := api.ReserveParam{
				VenueID:          nextRes.VenueID,
				ReservationTimes: []time.Time{nextRes.ReservationTime},
				PartySize:        nextRes.PartySize,
				LoginResp:        api.LoginResponse{AuthToken: authToken, PaymentMethodID: paymentMethodID},
				TableTypes:       tableTypes,
			}

			_, err = appCtx.API.Reserve(reserveParam)
			if err != nil {
				appendLog("Failed to book scheduled reservation " + nextRes.ID + ": " + err.Error())
			} else {
				appendLog("Successfully booked scheduled reservation " + nextRes.ID)
				notifyUsageIncrement(ctx, cfg, nextRes)
			}

			// Remove the reservation from Redis (regardless of success/failure)
			if err := store.DeleteReservation(ctx, nextRes.ID); err != nil {
				appendLog("Failed to delete reservation " + nextRes.ID + " from store: " + err.Error())
			}
		}
	}
}

// handleCookieRefresh periodically refreshes Imperva cookies for known venues
func handleCookieRefresh(ctx context.Context, cfg *config.Config) {
	appendLog("Cookie refresh goroutine started (interval: " + cfg.CookieRefreshInterval.String() + ")")

	// Run immediately on startup
	refreshAllCookies(ctx, cfg)

	// Then run periodically
	ticker := time.NewTicker(cfg.CookieRefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			appendLog("Cookie refresh goroutine shutting down")
			return
		case <-ticker.C:
			refreshAllCookies(ctx, cfg)
		}
	}
}

// refreshAllCookies checks and refreshes cookies for all known venues
func refreshAllCookies(ctx context.Context, cfg *config.Config) {
	venueIDs := cfg.VenueIDs()
	appendLog("Starting cookie refresh check for " + strconv.Itoa(len(venueIDs)) + " venues")

	for _, venueID := range venueIDs {
		select {
		case <-ctx.Done():
			return
		default:
			refreshCookiesIfNeeded(ctx, venueID)
		}
	}

	appendLog("Cookie refresh check completed")
}

// refreshCookiesIfNeeded checks if cookies need refreshing and fetches new ones if so
func refreshCookiesIfNeeded(ctx context.Context, venueID int64) {
	venueIDStr := strconv.FormatInt(venueID, 10)

	// Check if cookies exist and their TTL
	exists, err := store.CookieExists(ctx, venueID)
	if err != nil {
		appendLog("Error checking cookie existence for venue " + venueIDStr + ": " + err.Error())
		return
	}

	// If cookies exist, check if they're expiring soon (within 2 hours)
	if exists {
		ttl, err := store.GetCookieTTL(ctx, venueID)
		if err != nil {
			appendLog("Error getting cookie TTL for venue " + venueIDStr + ": " + err.Error())
			return
		}

		// Only refresh if TTL is less than 2 hours
		if ttl > 2*time.Hour {
			appendLog("Cookies for venue " + venueIDStr + " still valid (TTL: " + ttl.String() + "), skipping refresh")
			return
		}

		appendLog("Cookies for venue " + venueIDStr + " expiring soon (TTL: " + ttl.String() + "), refreshing...")
	} else {
		appendLog("No cookies found for venue " + venueIDStr + ", fetching...")
	}

	// Fetch new cookies using headless browser
	cookieData, err := imperva.FetchCookies(venueID)
	if err != nil {
		appendLog("Failed to fetch cookies for venue " + venueIDStr + ": " + err.Error())
		return
	}

	// Save cookies to Redis with 24 hour TTL
	if err := store.SaveCookies(ctx, venueID, cookieData.Cookies, cookieData.UserAgent, 24*time.Hour); err != nil {
		appendLog("Failed to save cookies for venue " + venueIDStr + ": " + err.Error())
		return
	}

	appendLog("Successfully refreshed " + strconv.Itoa(len(cookieData.Cookies)) + " cookies for venue " + venueIDStr)
}

// requireInternalToken validates the internal token for API/admin endpoints
func requireInternalToken(next http.HandlerFunc, cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if cfg.InternalAPIToken == "" {
			http.Error(w, "Server configuration error", http.StatusInternalServerError)
			return
		}
		token := r.Header.Get("X-Internal-Token")
		if token == "" || token != cfg.InternalAPIToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func notifyUsageIncrement(ctx context.Context, cfg *config.Config, res *store.ScheduledReservation) {
	if res.ClerkUserID == "" {
		return
	}
	if cfg.InternalAPIToken == "" {
		appendLog("Usage increment skipped: INTERNAL_API_TOKEN not configured")
		return
	}
	if cfg.WebAppURL == "" {
		appendLog("Usage increment skipped: web app URL not configured")
		return
	}

	usageType := res.UsageType
	if usageType == "" {
		usageType = "immediate"
	}

	payload := map[string]string{
		"clerkUserId": res.ClerkUserID,
		"type":        usageType,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		appendLog("Usage increment failed to marshal payload: " + err.Error())
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	url := strings.TrimRight(cfg.WebAppURL, "/") + "/api/internal/usage"
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		appendLog("Usage increment request failed: " + err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Token", cfg.InternalAPIToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		appendLog("Usage increment request error: " + err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		appendLog("Usage increment request failed with status " + strconv.Itoa(resp.StatusCode))
	}
}

// validateAdminToken checks the Authorization header for a valid admin token
func validateAdminToken(r *http.Request, cfg *config.Config) bool {
	queryToken := r.URL.Query().Get("token")
	authHeader := r.Header.Get("Authorization")

	if cfg.HasAdminToken() {
		if authHeader == "" {
			return cfg.ValidateAdminToken(queryToken)
		}

		// Expect "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return false
		}

		return cfg.ValidateAdminToken(parts[1])
	}

	if cfg.HasDevAdminToken() {
		if authHeader == "" {
			return cfg.ValidateDevAdminToken(queryToken)
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			return false
		}

		return cfg.ValidateDevAdminToken(parts[1])
	}

	return false
}

// Helper function to send JSON responses
func sendJSONResponse(w http.ResponseWriter, response interface{}, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(response)
}

func isSecureRequest(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	forwardedProto := r.Header.Get("X-Forwarded-Proto")
	return strings.EqualFold(forwardedProto, "https")
}

func getCookieValue(r *http.Request, name string) (string, error) {
	cookie, err := r.Cookie("session")
	if err != nil {
		return "", err
	}
	value := make(map[string]string)
	if err = s.Decode("session", cookie.Value, &value); err != nil {
		return "", err
	}
	return value[name], nil
}

func getSession(r *http.Request) (map[string]string, error) {
	cookie, err := r.Cookie("session")
	if err != nil {
		return nil, err
	}
	value := make(map[string]string)
	if err = s.Decode("session", cookie.Value, &value); err != nil {
		return nil, err
	}
	return value, nil
}

// parseTimeNYC parses a datetime-local format string as NYC time and returns UTC
func parseTimeNYC(timeStr string) (time.Time, error) {
	// datetime-local format: "2025-12-25T19:00"
	t, err := time.ParseInLocation("2006-01-02T15:04", timeStr, nycLocation)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil // Convert to UTC for storage/processing
}

// appendLog adds a log message to both the standard log and in-memory slice
func appendLog(message string) {
	logMu.Lock()
	defer logMu.Unlock()
	// Prevent unbounded memory growth by trimming old entries
	if len(logLines) >= maxLogLines {
		logLines = logLines[1:] // Remove oldest entry
	}
	logLines = append(logLines, time.Now().Format("2006-01-02 15:04:05")+" "+message)
	log.Println(message)
}
