package store

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// setupTestRedis creates a miniredis instance and configures the store to use it
func setupTestRedis(t *testing.T) *miniredis.Miniredis {
	t.Helper()

	// Reset any existing client first
	ResetClient()

	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("Failed to start miniredis: %v", err)
	}

	client := redis.NewClient(&redis.Options{
		Addr: mr.Addr(),
	})
	SetClient(client)

	t.Cleanup(func() {
		client.Close()
		mr.Close()
		ResetClient()
	})

	return mr
}

func TestSaveAndGetReservation(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()

	res := &ScheduledReservation{
		ID:               "test_res_123",
		VenueID:          89607,
		ReservationTime:  time.Now().Add(24 * time.Hour).UTC(),
		PartySize:        4,
		TablePreferences: []string{"dining_room", "outdoor"},
		AuthToken:        "test_auth_token",
		RunTime:          time.Now().Add(1 * time.Hour).UTC(),
		CreatedAt:        time.Now().UTC(),
	}

	// Save the reservation
	err := SaveReservation(ctx, res)
	if err != nil {
		t.Fatalf("SaveReservation failed: %v", err)
	}

	// Retrieve the reservation
	retrieved, err := GetReservation(ctx, res.ID)
	if err != nil {
		t.Fatalf("GetReservation failed: %v", err)
	}

	// Verify fields
	if retrieved.ID != res.ID {
		t.Errorf("ID mismatch: got %s, want %s", retrieved.ID, res.ID)
	}
	if retrieved.VenueID != res.VenueID {
		t.Errorf("VenueID mismatch: got %d, want %d", retrieved.VenueID, res.VenueID)
	}
	if retrieved.PartySize != res.PartySize {
		t.Errorf("PartySize mismatch: got %d, want %d", retrieved.PartySize, res.PartySize)
	}
	if retrieved.AuthToken != res.AuthToken {
		t.Errorf("AuthToken mismatch: got %s, want %s", retrieved.AuthToken, res.AuthToken)
	}
	if len(retrieved.TablePreferences) != len(res.TablePreferences) {
		t.Errorf("TablePreferences length mismatch: got %d, want %d", len(retrieved.TablePreferences), len(res.TablePreferences))
	}
}

func TestDeleteReservation(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()

	res := &ScheduledReservation{
		ID:              "test_res_delete",
		VenueID:         89607,
		ReservationTime: time.Now().Add(24 * time.Hour).UTC(),
		PartySize:       2,
		AuthToken:       "test_token",
		RunTime:         time.Now().Add(1 * time.Hour).UTC(),
		CreatedAt:       time.Now().UTC(),
	}

	// Save then delete
	if err := SaveReservation(ctx, res); err != nil {
		t.Fatalf("SaveReservation failed: %v", err)
	}

	if err := DeleteReservation(ctx, res.ID); err != nil {
		t.Fatalf("DeleteReservation failed: %v", err)
	}

	// Verify it's gone
	_, err := GetReservation(ctx, res.ID)
	if err == nil {
		t.Error("Expected error when getting deleted reservation, got nil")
	}

	// Verify it's removed from the pending set
	count, err := CountPendingReservations(ctx)
	if err != nil {
		t.Fatalf("CountPendingReservations failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 pending reservations after delete, got %d", count)
	}
}

func TestGetNextReservation(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Create reservations with different run times
	res1 := &ScheduledReservation{
		ID:              "res_later",
		VenueID:         89607,
		ReservationTime: now.Add(48 * time.Hour),
		PartySize:       2,
		AuthToken:       "token1",
		RunTime:         now.Add(2 * time.Hour), // Later
		CreatedAt:       now,
	}

	res2 := &ScheduledReservation{
		ID:              "res_earlier",
		VenueID:         89678,
		ReservationTime: now.Add(24 * time.Hour),
		PartySize:       4,
		AuthToken:       "token2",
		RunTime:         now.Add(1 * time.Hour), // Earlier
		CreatedAt:       now,
	}

	// Save in reverse order (later first)
	if err := SaveReservation(ctx, res1); err != nil {
		t.Fatalf("SaveReservation res1 failed: %v", err)
	}
	if err := SaveReservation(ctx, res2); err != nil {
		t.Fatalf("SaveReservation res2 failed: %v", err)
	}

	// GetNextReservation should return the earlier one
	next, err := GetNextReservation(ctx)
	if err != nil {
		t.Fatalf("GetNextReservation failed: %v", err)
	}
	if next == nil {
		t.Fatal("GetNextReservation returned nil")
	}
	if next.ID != "res_earlier" {
		t.Errorf("Expected earliest reservation (res_earlier), got %s", next.ID)
	}
}

func TestGetNextReservationEmpty(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()

	// No reservations saved
	next, err := GetNextReservation(ctx)
	if err != nil {
		t.Fatalf("GetNextReservation failed: %v", err)
	}
	if next != nil {
		t.Errorf("Expected nil for empty queue, got %+v", next)
	}
}

func TestGetPendingReservations(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Create one past-due and one future reservation
	resPastDue := &ScheduledReservation{
		ID:              "res_past",
		VenueID:         89607,
		ReservationTime: now.Add(24 * time.Hour),
		PartySize:       2,
		AuthToken:       "token1",
		RunTime:         now.Add(-1 * time.Hour), // Past due
		CreatedAt:       now,
	}

	resFuture := &ScheduledReservation{
		ID:              "res_future",
		VenueID:         89678,
		ReservationTime: now.Add(48 * time.Hour),
		PartySize:       4,
		AuthToken:       "token2",
		RunTime:         now.Add(2 * time.Hour), // Future
		CreatedAt:       now,
	}

	if err := SaveReservation(ctx, resPastDue); err != nil {
		t.Fatalf("SaveReservation resPastDue failed: %v", err)
	}
	if err := SaveReservation(ctx, resFuture); err != nil {
		t.Fatalf("SaveReservation resFuture failed: %v", err)
	}

	// GetPendingReservations should only return the past-due one
	pending, err := GetPendingReservations(ctx)
	if err != nil {
		t.Fatalf("GetPendingReservations failed: %v", err)
	}

	if len(pending) != 1 {
		t.Fatalf("Expected 1 pending reservation, got %d", len(pending))
	}
	if pending[0].ID != "res_past" {
		t.Errorf("Expected past-due reservation, got %s", pending[0].ID)
	}
}

func TestCountPendingReservations(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Start with 0
	count, err := CountPendingReservations(ctx)
	if err != nil {
		t.Fatalf("CountPendingReservations failed: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 pending, got %d", count)
	}

	// Add 3 reservations
	for i := 0; i < 3; i++ {
		res := &ScheduledReservation{
			ID:              GenerateReservationID(),
			VenueID:         89607,
			ReservationTime: now.Add(24 * time.Hour),
			PartySize:       2,
			AuthToken:       "token",
			RunTime:         now.Add(time.Duration(i) * time.Hour),
			CreatedAt:       now,
		}
		if err := SaveReservation(ctx, res); err != nil {
			t.Fatalf("SaveReservation %d failed: %v", i, err)
		}
	}

	count, err = CountPendingReservations(ctx)
	if err != nil {
		t.Fatalf("CountPendingReservations failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 pending, got %d", count)
	}
}

func TestGetAllPendingReservations(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Create 3 reservations with different run times
	ids := []string{"res_1", "res_2", "res_3"}
	for i, id := range ids {
		res := &ScheduledReservation{
			ID:              id,
			VenueID:         89607,
			ReservationTime: now.Add(24 * time.Hour),
			PartySize:       2,
			AuthToken:       "token",
			RunTime:         now.Add(time.Duration(i+1) * time.Hour),
			CreatedAt:       now,
		}
		if err := SaveReservation(ctx, res); err != nil {
			t.Fatalf("SaveReservation %s failed: %v", id, err)
		}
	}

	all, err := GetAllPendingReservations(ctx)
	if err != nil {
		t.Fatalf("GetAllPendingReservations failed: %v", err)
	}

	if len(all) != 3 {
		t.Fatalf("Expected 3 reservations, got %d", len(all))
	}

	// Should be ordered by run time (earliest first due to sorted set)
	for i, res := range all {
		expectedID := ids[i]
		if res.ID != expectedID {
			t.Errorf("Reservation %d: expected ID %s, got %s", i, expectedID, res.ID)
		}
	}
}

func TestGenerateReservationID(t *testing.T) {
	id1 := GenerateReservationID()
	time.Sleep(time.Microsecond) // Ensure different nanosecond timestamps
	id2 := GenerateReservationID()

	// IDs should start with "res_"
	if len(id1) < 5 || id1[:4] != "res_" {
		t.Errorf("ID should start with 'res_', got %s", id1)
	}

	// IDs should be unique (with nanosecond precision and sleep, they should differ)
	if id1 == id2 {
		t.Errorf("Generated IDs should be unique, got %s twice", id1)
	}
}

func TestReservationTimeOrdering(t *testing.T) {
	setupTestRedis(t)
	ctx := context.Background()

	now := time.Now().UTC()

	// Create reservations in random order
	times := []time.Duration{5 * time.Hour, 1 * time.Hour, 3 * time.Hour, 2 * time.Hour, 4 * time.Hour}
	for i, d := range times {
		res := &ScheduledReservation{
			ID:              GenerateReservationID(),
			VenueID:         89607,
			ReservationTime: now.Add(24 * time.Hour),
			PartySize:       2,
			AuthToken:       "token",
			RunTime:         now.Add(d),
			CreatedAt:       now,
		}
		if err := SaveReservation(ctx, res); err != nil {
			t.Fatalf("SaveReservation %d failed: %v", i, err)
		}
		// Small delay to ensure unique timestamps for GenerateReservationID
		time.Sleep(time.Microsecond)
	}

	// Verify ordering by repeatedly getting next
	expectedOrder := []time.Duration{1 * time.Hour, 2 * time.Hour, 3 * time.Hour, 4 * time.Hour, 5 * time.Hour}
	for i, expectedDuration := range expectedOrder {
		next, err := GetNextReservation(ctx)
		if err != nil {
			t.Fatalf("GetNextReservation %d failed: %v", i, err)
		}
		if next == nil {
			t.Fatalf("GetNextReservation %d returned nil", i)
		}

		expectedRunTime := now.Add(expectedDuration)
		// Allow 1 second tolerance for time comparison
		if next.RunTime.Sub(expectedRunTime).Abs() > time.Second {
			t.Errorf("Reservation %d: expected run time around %v, got %v", i, expectedRunTime, next.RunTime)
		}

		// Delete to get the next one
		if err := DeleteReservation(ctx, next.ID); err != nil {
			t.Fatalf("DeleteReservation %d failed: %v", i, err)
		}
	}

	// Queue should be empty now
	next, err := GetNextReservation(ctx)
	if err != nil {
		t.Fatalf("Final GetNextReservation failed: %v", err)
	}
	if next != nil {
		t.Error("Expected empty queue after processing all reservations")
	}
}
