// Package config reads configuration from environment variables. No viper,
// no config files, no reflection-based binding -- just os.Getenv and a few
// typed helpers (YAGNI: the entire config surface fits in one struct).
package config

import (
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Port        string
	DatabaseURL string

	JWTSecret         string
	JWTTTLHours       int
	MinIncrementCents int64

	CORSAllowedOrigins string

	SeedDemoData   bool
	MigrateOnStart bool
	LogLevel       string

	RoomCmdBuffer int
	DBMaxConns    int32
}

func Load() (Config, error) {
	c := Config{
		Port:               getEnv("PORT", "8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		JWTSecret:          getEnv("JWT_SECRET", "dev-secret-change-me"),
		JWTTTLHours:        getEnvInt("JWT_TTL_HOURS", 24),
		MinIncrementCents:  getEnvInt64("DEFAULT_MIN_INCREMENT_CENTS", 100),
		CORSAllowedOrigins: getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:4321"),
		SeedDemoData:       getEnvBool("SEED_DEMO_DATA", false),
		MigrateOnStart:     getEnvBool("MIGRATE_ON_START", true),
		LogLevel:           getEnv("LOG_LEVEL", "info"),
		RoomCmdBuffer:      getEnvInt("ROOM_CMD_BUFFER", 256),
		DBMaxConns:         int32(getEnvInt("DB_MAX_CONNS", 25)),
	}
	if c.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	return c, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getEnvInt64(key string, def int64) int64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func getEnvBool(key string, def bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
