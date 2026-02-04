package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const resyCredentialsEncryptionPrefix = "v1:"

var errEncryptionPrefixMissing = errors.New("encrypted value missing prefix")

func hasEncryptionPrefix(value string) bool {
	return strings.HasPrefix(value, resyCredentialsEncryptionPrefix)
}

func encryptString(plaintext string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, ciphertext...)
	return resyCredentialsEncryptionPrefix + base64.StdEncoding.EncodeToString(payload), nil
}

func decryptString(value string, key []byte) (string, error) {
	if !hasEncryptionPrefix(value) {
		return "", errEncryptionPrefixMissing
	}

	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, resyCredentialsEncryptionPrefix))
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
