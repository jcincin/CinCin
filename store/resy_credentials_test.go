package store

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

const testResyCredentialsKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestResyCredentialsEncryptionRoundTrip(t *testing.T) {
	t.Setenv("RESY_CREDENTIALS_KEY", testResyCredentialsKey)
	setupTestRedis(t)
	ctx := context.Background()

	creds := &ResyCredentials{
		ClerkUserID:     "user_123",
		AuthToken:       "auth_token_value",
		PaymentMethodID: 123456,
	}

	if err := SaveResyCredentials(ctx, creds); err != nil {
		t.Fatalf("SaveResyCredentials failed: %v", err)
	}

	raw, err := GetClient().Get(ctx, ResyCredentialsKey(creds.ClerkUserID)).Bytes()
	if err != nil {
		t.Fatalf("Failed to read stored credentials: %v", err)
	}

	var stored map[string]interface{}
	if err := json.Unmarshal(raw, &stored); err != nil {
		t.Fatalf("Failed to unmarshal stored credentials: %v", err)
	}

	authToken, ok := stored["auth_token"].(string)
	if !ok || !strings.HasPrefix(authToken, resyCredentialsEncryptionPrefix) {
		t.Fatalf("Expected encrypted auth_token, got %v", stored["auth_token"])
	}

	paymentToken, ok := stored["payment_method_id"].(string)
	if !ok || !strings.HasPrefix(paymentToken, resyCredentialsEncryptionPrefix) {
		t.Fatalf("Expected encrypted payment_method_id, got %v", stored["payment_method_id"])
	}

	got, err := GetResyCredentials(ctx, creds.ClerkUserID)
	if err != nil {
		t.Fatalf("GetResyCredentials failed: %v", err)
	}

	if got.AuthToken != creds.AuthToken {
		t.Errorf("AuthToken mismatch: got %s, want %s", got.AuthToken, creds.AuthToken)
	}
	if got.PaymentMethodID != creds.PaymentMethodID {
		t.Errorf("PaymentMethodID mismatch: got %d, want %d", got.PaymentMethodID, creds.PaymentMethodID)
	}
}

func TestResyCredentialsPlaintextAutoMigration(t *testing.T) {
	t.Setenv("RESY_CREDENTIALS_KEY", testResyCredentialsKey)
	setupTestRedis(t)
	ctx := context.Background()

	plaintext := &ResyCredentials{
		ClerkUserID:     "user_456",
		AuthToken:       "plain_token",
		PaymentMethodID: 98765,
	}

	rawPlain, err := json.Marshal(plaintext)
	if err != nil {
		t.Fatalf("Failed to marshal plaintext credentials: %v", err)
	}

	if err := GetClient().Set(ctx, ResyCredentialsKey(plaintext.ClerkUserID), rawPlain, 0).Err(); err != nil {
		t.Fatalf("Failed to store plaintext credentials: %v", err)
	}

	got, err := GetResyCredentials(ctx, plaintext.ClerkUserID)
	if err != nil {
		t.Fatalf("GetResyCredentials failed: %v", err)
	}

	if got.AuthToken != plaintext.AuthToken {
		t.Errorf("AuthToken mismatch: got %s, want %s", got.AuthToken, plaintext.AuthToken)
	}
	if got.PaymentMethodID != plaintext.PaymentMethodID {
		t.Errorf("PaymentMethodID mismatch: got %d, want %d", got.PaymentMethodID, plaintext.PaymentMethodID)
	}

	rawMigrated, err := GetClient().Get(ctx, ResyCredentialsKey(plaintext.ClerkUserID)).Bytes()
	if err != nil {
		t.Fatalf("Failed to read migrated credentials: %v", err)
	}

	var migrated map[string]interface{}
	if err := json.Unmarshal(rawMigrated, &migrated); err != nil {
		t.Fatalf("Failed to unmarshal migrated credentials: %v", err)
	}

	authToken, ok := migrated["auth_token"].(string)
	if !ok || !strings.HasPrefix(authToken, resyCredentialsEncryptionPrefix) {
		t.Fatalf("Expected encrypted auth_token after migration, got %v", migrated["auth_token"])
	}

	paymentToken, ok := migrated["payment_method_id"].(string)
	if !ok || !strings.HasPrefix(paymentToken, resyCredentialsEncryptionPrefix) {
		t.Fatalf("Expected encrypted payment_method_id after migration, got %v", migrated["payment_method_id"])
	}
}
