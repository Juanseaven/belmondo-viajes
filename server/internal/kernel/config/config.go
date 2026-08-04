// Package config carga la configuración desde el entorno. Falla al arrancar, no
// en la primera petición: un secreto ausente debe impedir el despliegue.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env      string
	HTTPAddr string
	LogLevel string

	DatabaseURL      string
	DatabaseMaxConns int32

	RedisAddr string
	RedisDB   int

	EvolutionBaseURL string
	EvolutionAPIKey  string
	WebhookAPIKey    string

	WorkerShards    int
	WorkerQueueSize int
	MediaWorkers    int

	R2AccountID    string
	R2AccessKeyID  string
	R2SecretKey    string
	R2Bucket       string
	R2SignedURLTTL time.Duration

	JWTSecret      string
	JWTTTL         time.Duration
	AllowedOrigins []string

	// Administrador inicial. Solo se usa si la tabla `users` está vacía: sin él
	// no hay forma de entrar la primera vez, porque no existe registro público y
	// crear usuarios exige ya ser admin.
	SeedAdminEmail    string
	SeedAdminPassword string
}

func Load() (*Config, error) {
	c := &Config{
		Env:      env("APP_ENV", "development"),
		HTTPAddr: env("HTTP_ADDR", ":8080"),
		LogLevel: env("LOG_LEVEL", "info"),

		DatabaseURL:      os.Getenv("DATABASE_URL"),
		DatabaseMaxConns: int32(envInt("DATABASE_MAX_CONNS", 20)),

		RedisAddr: env("REDIS_ADDR", "redis:6379"),
		RedisDB:   envInt("REDIS_DB", 1),

		EvolutionBaseURL: env("EVOLUTION_BASE_URL", "http://evolution-api:8080"),
		EvolutionAPIKey:  os.Getenv("EVOLUTION_API_KEY"),
		WebhookAPIKey:    os.Getenv("WEBHOOK_API_KEY"),

		WorkerShards:    envInt("WORKER_SHARDS", 8),
		WorkerQueueSize: envInt("WORKER_QUEUE_SIZE", 512),
		MediaWorkers:    envInt("MEDIA_WORKERS", 4),

		R2AccountID:    os.Getenv("R2_ACCOUNT_ID"),
		R2AccessKeyID:  os.Getenv("R2_ACCESS_KEY_ID"),
		R2SecretKey:    os.Getenv("R2_SECRET_ACCESS_KEY"),
		R2Bucket:       os.Getenv("R2_BUCKET"),
		R2SignedURLTTL: envDuration("R2_SIGNED_URL_TTL", 15*time.Minute),

		JWTSecret:      os.Getenv("JWT_SECRET"),
		JWTTTL:         envDuration("JWT_TTL", 12*time.Hour),
		AllowedOrigins: splitCSV(env("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),

		SeedAdminEmail:    os.Getenv("SEED_ADMIN_EMAIL"),
		SeedAdminPassword: os.Getenv("SEED_ADMIN_PASSWORD"),
	}

	// Si WEBHOOK_API_KEY no se define, se usa la de Evolution: es la que esa
	// misma instancia va a presentar en cada webhook.
	if c.WebhookAPIKey == "" {
		c.WebhookAPIKey = c.EvolutionAPIKey
	}

	return c, c.validate()
}

func (c *Config) validate() error {
	var missing []string
	required := map[string]string{
		"DATABASE_URL":         c.DatabaseURL,
		"EVOLUTION_API_KEY":    c.EvolutionAPIKey,
		"JWT_SECRET":           c.JWTSecret,
		"R2_ACCOUNT_ID":        c.R2AccountID,
		"R2_ACCESS_KEY_ID":     c.R2AccessKeyID,
		"R2_SECRET_ACCESS_KEY": c.R2SecretKey,
		"R2_BUCKET":            c.R2Bucket,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("faltan variables de entorno obligatorias: %s", strings.Join(missing, ", "))
	}
	if len(c.JWTSecret) < 32 {
		return fmt.Errorf("JWT_SECRET debe tener al menos 32 bytes (usa: openssl rand -hex 32)")
	}
	if c.WorkerShards < 1 {
		return fmt.Errorf("WORKER_SHARDS debe ser >= 1")
	}
	return nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
