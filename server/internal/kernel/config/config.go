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

	EvolutionBaseURL    string
	EvolutionAPIKey     string
	EvolutionWebhookURL string
	WebhookAPIKey       string

	WorkerShards    int
	WorkerQueueSize int
	MediaWorkers    int

	// Días que se conservan los webhooks ya procesados. 0 desactiva la purga.
	// La tabla guarda el payload completo de cada evento y crece del orden de
	// millones de filas al mes: sin purga, el síntoma es el disco lleno.
	EventRetentionDays int

	// Almacenamiento de archivos. Si las credenciales de R2 están completas se
	// usa R2; si no, disco local. Los adjuntos de WhatsApp no viajan en el
	// webhook, así que sin almacenamiento no habría dónde guardarlos.
	R2AccountID    string
	R2AccessKeyID  string
	R2SecretKey    string
	R2Bucket       string
	R2SignedURLTTL time.Duration

	// BlobsDir es el directorio del almacenamiento local. Se ignora con R2.
	BlobsDir string

	// PublicBaseURL es la raíz por la que se alcanza este backend. Solo se usa
	// con almacenamiento local, para construir las URLs firmadas de los archivos.
	//
	// Tiene que ser alcanzable desde DOS sitios: el navegador que muestra el
	// archivo y Evolution, que lo descarga al enviar un adjunto.
	PublicBaseURL string

	JWTSecret      string
	JWTTTL         time.Duration
	AllowedOrigins []string

	// Administrador inicial. Solo se usa si la tabla `users` está vacía: sin él
	// no hay forma de entrar la primera vez, porque no existe registro público y
	// crear usuarios exige ya ser admin.
	SeedAdminEmail    string
	SeedAdminPassword string

	// Requisitos de viaje. Sin clave de Anthropic el módulo arranca igual: las
	// reglas nuevas quedan sin fuentes y todo cuenta como «verificar».
	AnthropicAPIKey string
	// RequisitosContact va en el User-Agent de las consultas a Wikipedia, que
	// pide un contacto a quien la lee de forma automatizada.
	RequisitosContact string
	// PassportIndexEnabled enciende la segunda opinión sobre la visa. Se apaga
	// si sus datos quedan viejos y solo generan conflictos.
	PassportIndexEnabled bool
	// RequisitosResearch enciende la búsqueda del detalle en páginas oficiales.
	// Apagada por defecto: cada búsqueda cuesta entre 1 y 3 dólares.
	RequisitosResearch bool
	// RequisitosResearchModel es el modelo que busca el detalle en las páginas
	// oficiales. Vacío: el de por defecto (Opus).
	RequisitosResearchModel string
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

		EvolutionBaseURL:    env("EVOLUTION_BASE_URL", "http://evolution-api:8080"),
		EvolutionAPIKey:     os.Getenv("EVOLUTION_API_KEY"),
		EvolutionWebhookURL: env("EVOLUTION_WEBHOOK_URL", "http://backend:8080/webhooks/evolution"),
		WebhookAPIKey:       os.Getenv("WEBHOOK_API_KEY"),

		WorkerShards:    envInt("WORKER_SHARDS", 8),
		WorkerQueueSize: envInt("WORKER_QUEUE_SIZE", 512),
		MediaWorkers:    envInt("MEDIA_WORKERS", 4),

		EventRetentionDays: envInt("EVENT_RETENTION_DAYS", 30),

		R2AccountID:    os.Getenv("R2_ACCOUNT_ID"),
		R2AccessKeyID:  os.Getenv("R2_ACCESS_KEY_ID"),
		R2SecretKey:    os.Getenv("R2_SECRET_ACCESS_KEY"),
		R2Bucket:       os.Getenv("R2_BUCKET"),
		R2SignedURLTTL: envDuration("R2_SIGNED_URL_TTL", 15*time.Minute),

		BlobsDir:      env("BLOBS_DIR", "/data/blobs"),
		PublicBaseURL: env("PUBLIC_BASE_URL", "http://localhost:8080"),

		JWTSecret:      os.Getenv("JWT_SECRET"),
		JWTTTL:         envDuration("JWT_TTL", 12*time.Hour),
		AllowedOrigins: splitCSV(env("CORS_ALLOWED_ORIGINS", "http://localhost:3000")),

		SeedAdminEmail:    os.Getenv("SEED_ADMIN_EMAIL"),
		SeedAdminPassword: os.Getenv("SEED_ADMIN_PASSWORD"),

		AnthropicAPIKey:      os.Getenv("ANTHROPIC_API_KEY"),
		RequisitosContact:    os.Getenv("REQUISITOS_CONTACTO"),
		PassportIndexEnabled: envBool("REQUISITOS_PASSPORT_INDEX", true),

		RequisitosResearch:      envBool("REQUISITOS_BUSQUEDA_OFICIAL", false),
		RequisitosResearchModel: os.Getenv("REQUISITOS_MODELO_BUSQUEDA"),
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
	// R2 NO está aquí: sin sus credenciales el sistema usa disco local, que es
	// suficiente para desarrollo y para un despliegue de una sola máquina.
	required := map[string]string{
		"DATABASE_URL":      c.DatabaseURL,
		"EVOLUTION_API_KEY": c.EvolutionAPIKey,
		"JWT_SECRET":        c.JWTSecret,
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
	// O están las cuatro credenciales de R2, o ninguna. Tres de cuatro es un
	// despliegue a medio configurar que arrancaría en local sin que nadie se dé
	// cuenta hasta que alguien busque un archivo que creía en la nube.
	if n := c.r2VarsSet(); n != 0 && n != 4 {
		return fmt.Errorf(
			"configuración de R2 incompleta (%d de 4 variables): definí las cuatro o ninguna", n)
	}
	return nil
}

// UseR2 indica si hay que usar Cloudflare R2 en vez del disco local.
func (c *Config) UseR2() bool { return c.r2VarsSet() == 4 }

func (c *Config) r2VarsSet() int {
	n := 0
	for _, v := range []string{c.R2AccountID, c.R2AccessKeyID, c.R2SecretKey, c.R2Bucket} {
		if strings.TrimSpace(v) != "" {
			n++
		}
	}
	return n
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

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
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
