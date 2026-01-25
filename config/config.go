package config

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"strconv"
	"sync"
	"time"
)

// Venue represents a restaurant venue
type Venue struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// venuesFile represents the structure of venues.json
type venuesFile struct {
	Venues []Venue `json:"venues"`
}

// Config holds all configuration values
type Config struct {
	RedisURL              string
	RedisPassword         string
	ResyAPIKey            string
	CookieSecretKey       []byte
	CookieBlockKey        []byte
	Port                  string
	AdminToken            string
	CookieRefreshEnabled  bool
	CookieRefreshInterval time.Duration
	Venues                []Venue
}

var (
	cfg  *Config
	once sync.Once
)

// Get returns the singleton configuration
func Get() *Config {
	once.Do(func() {
		cfg = &Config{
			RedisURL:              getEnv("REDIS_URL", "localhost:6379"),
			RedisPassword:         getEnv("REDIS_PASSWORD", ""),
			ResyAPIKey:            getEnv("RESY_API_KEY", "VbWk7s3L4KiK5fzlO7JD3Q5EYolJI7n5"),
			CookieSecretKey:       getSecretKey("COOKIE_SECRET_KEY"),
			CookieBlockKey:        getSecretKey("COOKIE_BLOCK_KEY"),
			Port:                  getEnv("PORT", "8090"),
			AdminToken:            getEnv("ADMIN_TOKEN", ""),
			CookieRefreshEnabled:  getEnvBool("COOKIE_REFRESH_ENABLED", true),
			CookieRefreshInterval: getEnvDuration("COOKIE_REFRESH_INTERVAL", 6*time.Hour),
			Venues:                loadVenues(),
		}
	})
	return cfg
}

// loadVenues reads venues from venues.json file
func loadVenues() []Venue {
	venuesPath := getEnv("VENUES_FILE", "venues.json")

	data, err := os.ReadFile(venuesPath)
	if err != nil {
		log.Printf("Warning: Could not read venues file %s: %v", venuesPath, err)
		return []Venue{}
	}

	var vf venuesFile
	if err := json.Unmarshal(data, &vf); err != nil {
		log.Printf("Warning: Could not parse venues file %s: %v", venuesPath, err)
		return []Venue{}
	}

	log.Printf("Loaded %d venues from %s", len(vf.Venues), venuesPath)
	return vf.Venues
}

// VenueIDs returns a slice of venue IDs for backward compatibility
func (c *Config) VenueIDs() []int64 {
	ids := make([]int64, len(c.Venues))
	for i, v := range c.Venues {
		ids[i] = v.ID
	}
	return ids
}

// getEnv returns the environment variable value or a default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvBool returns a boolean from environment variable or default
func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	// Accept "true", "1", "yes" as true; anything else as false
	return value == "true" || value == "1" || value == "yes"
}

// getEnvDuration returns a duration from environment variable or default
// Accepts formats like "6h", "30m", "1h30m"
func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}

	// First try parsing as a Go duration string (e.g., "6h", "30m")
	if d, err := time.ParseDuration(value); err == nil {
		return d
	}

	// Fall back to parsing as hours (e.g., "6" means 6 hours)
	if hours, err := strconv.Atoi(value); err == nil {
		return time.Duration(hours) * time.Hour
	}

	return defaultValue
}

// getSecretKey returns a 32-byte key from hex-encoded env var or nil if not set
func getSecretKey(key string) []byte {
	hexKey := os.Getenv(key)
	if hexKey == "" {
		return nil // Will trigger random key generation
	}
	decoded, err := hex.DecodeString(hexKey)
	if err != nil || len(decoded) != 32 {
		return nil
	}
	return decoded
}

// HasAdminToken returns true if an admin token is configured
func (c *Config) HasAdminToken() bool {
	return c.AdminToken != ""
}

// ValidateAdminToken checks if the provided token matches the configured admin token
func (c *Config) ValidateAdminToken(token string) bool {
	if !c.HasAdminToken() {
		return false // No admin token configured, deny all
	}
	return token == c.AdminToken
}
