package config

import (
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	Env string `env:"ENV" envDefault:"dev"`

	HTTPPort string `env:"HTTP_PORT" envDefault:"8080"`
	GRPCPort string `env:"GRPC_PORT" envDefault:"9090"`

	// WriteDatabaseURL points at the master (commands, transactions, outbox).
	WriteDatabaseURL string `env:"WRITE_DATABASE_URL,required"`
	// ReadDatabaseURL points at a replica (queries). Empty in dev → falls back
	// to the write DSN so a single Postgres works locally.
	ReadDatabaseURL string `env:"READ_DATABASE_URL" envDefault:""`

	RedisAddr string `env:"REDIS_ADDR" envDefault:"localhost:6379"`

	// MinIO / S3-compatible object store.
	MinIOEndpoint  string `env:"MINIO_ENDPOINT" envDefault:"localhost:9000"`
	MinIOAccessKey string `env:"MINIO_ACCESS_KEY" envDefault:"minioadmin"`
	MinIOSecretKey string `env:"MINIO_SECRET_KEY" envDefault:"minioadmin"`
	MinIOBucket    string `env:"MINIO_BUCKET" envDefault:"webcomics"`
	MinIOUseSSL    bool   `env:"MINIO_USE_SSL" envDefault:"false"`
	// PublicEndpoint is what we hand back to browsers in presigned URLs.
	// Inside compose the API talks to "minio:9000"; the browser needs "localhost:9000".
	MinIOPublicEndpoint string `env:"MINIO_PUBLIC_ENDPOINT" envDefault:""`

	// APIKey enables X-API-Key auth on /api/* + /metrics when non-empty.
	// Leave unset for single-user/local dev.
	APIKey string `env:"API_KEY" envDefault:""`

	// WebhookURL is POSTed when a run reaches a terminal state. Body is
	// {event, run_id, run, ts}. If WebhookSecret is set, requests carry
	// X-Webhook-Signature: sha256=<hex> (HMAC over the body).
	WebhookURL    string `env:"WEBHOOK_URL" envDefault:""`
	WebhookSecret string `env:"WEBHOOK_SECRET" envDefault:""`

	// Provider API keys — used by /api/balances on the dashboard.
	OpenRouterAPIKey string `env:"OPENROUTER_API_KEY" envDefault:""`
	FalAPIKey        string `env:"FAL_KEY" envDefault:""`
	ElevenLabsAPIKey string `env:"ELEVENLABS_API_KEY" envDefault:""`
	PixabayAPIKey    string `env:"PIXABAY_API_KEY" envDefault:""`
	// YouTubeAPIKey enables the upload-metrics ticker to query the YT
	// Data API v3 (videos.list) for public counts. No key → ticker skips
	// YT rows and logs a warning. Get one in Google Cloud Console.
	YouTubeAPIKey string `env:"YOUTUBE_API_KEY" envDefault:""`

	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"15s"`
}

func Load() (*Config, error) {
	_ = godotenv.Load()
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
