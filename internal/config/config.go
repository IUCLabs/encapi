// Package config loads encapi server configuration from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config is the fully-resolved server configuration.
type Config struct {
	ListenAddr string

	DBHost string
	DBPort int
	DBUser string
	DBPass string
	DBName string
	DBSSL  string

	// WriteToken guards all mutating endpoints. Reads are always open.
	// When empty, writes are refused entirely (fail-closed).
	WriteToken string

	// DistroAPIURL, when set, points encapi at an external kickstart/distro
	// API that resolves per-host provisioning params (epel, os release, ...).
	// Left empty, no distro params are injected into ENC output.
	DistroAPIURL string
}

// DatabaseDSN renders a libpq/pgx connection string.
func (c *Config) DatabaseDSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.DBUser, c.DBPass, c.DBHost, c.DBPort, c.DBName, c.DBSSL,
	)
}

// Load reads configuration from the environment, applying defaults.
func Load() (*Config, error) {
	dbPort, err := strconv.Atoi(getenv("DBPORT", "5432"))
	if err != nil {
		return nil, fmt.Errorf("invalid DBPORT: %w", err)
	}

	return &Config{
		ListenAddr:   getenv("LISTEN_ADDR", ":8000"),
		DBHost:       getenv("DBHOST", "localhost"),
		DBPort:       dbPort,
		DBUser:       getenv("DBUSER", "encapi"),
		DBPass:       getenv("DBPASS", "encapi"),
		DBName:       getenv("DBNAME", "encapi"),
		DBSSL:        getenv("DBSSL", "disable"),
		WriteToken:   os.Getenv("ENCAPI_WRITE_TOKEN"),
		DistroAPIURL: os.Getenv("ENCAPI_DISTRO_API_URL"),
	}, nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
