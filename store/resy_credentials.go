package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/21Bruce/resolved-server/config"
)

// ResyCredentials stores a user's linked Resy account credentials
type ResyCredentials struct {
	ClerkUserID     string `json:"clerk_user_id"`
	AuthToken       string `json:"auth_token"`
	PaymentMethodID int64  `json:"payment_method_id"`
}

type resyCredentialsRecord struct {
	ClerkUserID     string `json:"clerk_user_id"`
	AuthToken       string `json:"auth_token"`
	PaymentMethodID string `json:"payment_method_id"`
}

const ResyCredentialsKeyPrefix = "resy_credentials:"

var errResyCredentialsKeyMissing = errors.New("resy credentials key not configured")

// ResyCredentialsKey returns the Redis key for a user's Resy credentials
func ResyCredentialsKey(clerkUserID string) string {
	return fmt.Sprintf("%s%s", ResyCredentialsKeyPrefix, clerkUserID)
}

// SaveResyCredentials stores Resy credentials for a Clerk user
func SaveResyCredentials(ctx context.Context, creds *ResyCredentials) error {
	key := config.Get().ResyCredentialsKey
	if len(key) == 0 {
		return errResyCredentialsKeyMissing
	}

	encryptedAuthToken, err := encryptString(creds.AuthToken, key)
	if err != nil {
		return err
	}

	encryptedPaymentID, err := encryptString(strconv.FormatInt(creds.PaymentMethodID, 10), key)
	if err != nil {
		return err
	}

	record := resyCredentialsRecord{
		ClerkUserID:     creds.ClerkUserID,
		AuthToken:       encryptedAuthToken,
		PaymentMethodID: encryptedPaymentID,
	}

	jsonData, err := json.Marshal(record)
	if err != nil {
		return err
	}

	redisKey := ResyCredentialsKey(creds.ClerkUserID)
	return GetClient().Set(ctx, redisKey, jsonData, 0).Err()
}

// GetResyCredentials retrieves Resy credentials for a Clerk user
func GetResyCredentials(ctx context.Context, clerkUserID string) (*ResyCredentials, error) {
	key := config.Get().ResyCredentialsKey
	if len(key) == 0 {
		return nil, errResyCredentialsKeyMissing
	}

	jsonData, err := GetClient().Get(ctx, ResyCredentialsKey(clerkUserID)).Bytes()
	if err != nil {
		return nil, err
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(jsonData, &raw); err != nil {
		return nil, err
	}

	authTokenRaw, ok := raw["auth_token"].(string)
	if !ok || authTokenRaw == "" {
		return nil, errors.New("auth_token missing or invalid")
	}

	paymentRawValue, ok := raw["payment_method_id"]
	if !ok {
		return nil, errors.New("payment_method_id missing")
	}

	paymentRaw, err := coercePaymentMethodID(paymentRawValue)
	if err != nil {
		return nil, err
	}

	needsReencrypt := false

	authToken := authTokenRaw
	if hasEncryptionPrefix(authTokenRaw) {
		authToken, err = decryptString(authTokenRaw, key)
		if err != nil {
			return nil, err
		}
	} else {
		needsReencrypt = true
	}

	paymentToken := paymentRaw
	if hasEncryptionPrefix(paymentRaw) {
		paymentToken, err = decryptString(paymentRaw, key)
		if err != nil {
			return nil, err
		}
	} else {
		needsReencrypt = true
	}

	paymentMethodID, err := strconv.ParseInt(paymentToken, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid payment_method_id: %w", err)
	}

	resolvedClerkID := clerkUserID
	if rawClerkID, ok := raw["clerk_user_id"].(string); ok && rawClerkID != "" {
		resolvedClerkID = rawClerkID
	}

	creds := &ResyCredentials{
		ClerkUserID:     resolvedClerkID,
		AuthToken:       authToken,
		PaymentMethodID: paymentMethodID,
	}

	if needsReencrypt {
		if err := SaveResyCredentials(ctx, creds); err != nil {
			log.Printf("Warning: failed to re-encrypt Resy credentials for %s: %v", clerkUserID, err)
		}
	}

	return creds, nil
}

func coercePaymentMethodID(value interface{}) (string, error) {
	switch v := value.(type) {
	case string:
		if v == "" {
			return "", errors.New("payment_method_id missing")
		}
		return v, nil
	case float64:
		return strconv.FormatInt(int64(v), 10), nil
	default:
		return "", fmt.Errorf("invalid payment_method_id type %T", value)
	}
}

// DeleteResyCredentials removes Resy credentials for a Clerk user
func DeleteResyCredentials(ctx context.Context, clerkUserID string) error {
	return GetClient().Del(ctx, ResyCredentialsKey(clerkUserID)).Err()
}

// ResyCredentialsExist checks if a user has linked their Resy account
func ResyCredentialsExist(ctx context.Context, clerkUserID string) (bool, error) {
	count, err := GetClient().Exists(ctx, ResyCredentialsKey(clerkUserID)).Result()
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
