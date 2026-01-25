package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// BookingWindow represents when reservations open for a venue
type BookingWindow struct {
	VenueID       int64  `json:"venue_id"`
	DaysInAdvance int    `json:"days_in_advance"` // How many days ahead you can book
	ReleaseHour   int    `json:"release_hour"`    // Hour reservations open (0-23)
	ReleaseMinute int    `json:"release_minute"`  // Minute reservations open (0-59)
	Timezone      string `json:"timezone"`        // Timezone for release time (e.g., "America/New_York")
	ScrapedAt     time.Time `json:"scraped_at"`   // When this info was fetched
}

const (
	BookingWindowKeyPrefix = "booking_window:"
	BookingWindowTTL       = 24 * time.Hour // Cache for 24 hours
)

// BookingWindowKey returns the Redis key for a venue's booking window
func BookingWindowKey(venueID int64) string {
	return fmt.Sprintf("%s%d", BookingWindowKeyPrefix, venueID)
}

// SaveBookingWindow stores booking window info in Redis
func SaveBookingWindow(ctx context.Context, bw *BookingWindow) error {
	jsonData, err := json.Marshal(bw)
	if err != nil {
		return fmt.Errorf("failed to marshal booking window: %w", err)
	}

	return GetClient().Set(ctx, BookingWindowKey(bw.VenueID), jsonData, BookingWindowTTL).Err()
}

// GetBookingWindow retrieves booking window info from Redis
func GetBookingWindow(ctx context.Context, venueID int64) (*BookingWindow, error) {
	jsonData, err := GetClient().Get(ctx, BookingWindowKey(venueID)).Bytes()
	if err != nil {
		return nil, err
	}

	var bw BookingWindow
	if err := json.Unmarshal(jsonData, &bw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal booking window: %w", err)
	}

	return &bw, nil
}

// BookingWindowExists checks if booking window info exists in Redis
func BookingWindowExists(ctx context.Context, venueID int64) (bool, error) {
	result, err := GetClient().Exists(ctx, BookingWindowKey(venueID)).Result()
	if err != nil {
		return false, err
	}
	return result > 0, nil
}

// CalculateRunTime calculates when to attempt booking based on booking window
// Given a desired reservation time, returns the optimal time to run the sniper
func (bw *BookingWindow) CalculateRunTime(reservationTime time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(bw.Timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timezone %s: %w", bw.Timezone, err)
	}

	// Convert reservation time to the venue's timezone
	resTimeLocal := reservationTime.In(loc)

	// Calculate the release date (reservation date minus days in advance)
	releaseDate := resTimeLocal.AddDate(0, 0, -bw.DaysInAdvance)

	// Set the release time on that date
	runTime := time.Date(
		releaseDate.Year(),
		releaseDate.Month(),
		releaseDate.Day(),
		bw.ReleaseHour,
		bw.ReleaseMinute,
		0, 0,
		loc,
	)

	return runTime.UTC(), nil
}
