package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	PositionCheckInterval time.Duration `env:"POSITION_CHECK_INTERVAL" env-default:"30s" validate:"gt=0"`
	PositionStaleAfter    time.Duration `env:"POSITION_STALE_AFTER" env-default:"60s" validate:"gt=0"`
	PositionWorkers       int           `env:"POSITION_WORKERS" env-default:"4" validate:"gt=0"`
	PositionQueueSize     int           `env:"POSITION_QUEUE_SIZE" env-default:"100" validate:"gt=0"`
	PositionBatchSize     int           `env:"POSITION_SCAN_BATCH_SIZE" env-default:"100" validate:"gt=0"`
	PushHTTPURL           string        `env:"PUSH_HTTP_URL" env-required:"true" validate:"required,http_url"`
	PushRequestTimeout    time.Duration `env:"PUSH_REQUEST_TIMEOUT" env-default:"1s" validate:"gt=0"`
	PushKeepAlive         time.Duration `env:"PUSH_KEEP_ALIVE" env-default:"30s" validate:"gt=0"`
	PushIdleConnTimeout   time.Duration `env:"PUSH_IDLE_CONN_TIMEOUT" env-default:"90s" validate:"gt=0"`

	OTELServiceName string `env:"OTEL_SERVICE_NAME" env-required:"true" validate:"required"`
	OTELEndpoint    string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" env-required:"true" validate:"required"`
	OTELProtocol    string `env:"OTEL_EXPORTER_OTLP_PROTOCOL" env-required:"true" validate:"oneof=grpc http/protobuf"`
	OTELSampler     string `env:"OTEL_TRACES_SAMPLER" env-required:"true" validate:"oneof=always_on always_off traceidratio parentbased_always_on parentbased_always_off parentbased_traceidratio"`

	HTTPAddr                string        `env:"HTTP_ADDR" env-required:"true" validate:"required"`
	HTTPReadTimeout         time.Duration `env:"HTTP_READ_TIMEOUT" env-required:"true" validate:"gt=0"`
	HTTPReadHeaderTimeout   time.Duration `env:"HTTP_READ_HEADER_TIMEOUT" env-required:"true" validate:"gt=0"`
	HTTPWriteTimeout        time.Duration `env:"HTTP_WRITE_TIMEOUT" env-required:"true" validate:"gt=0"`
	HTTPIdleTimeout         time.Duration `env:"HTTP_IDLE_TIMEOUT" env-required:"true" validate:"gt=0"`
	LogLevel                string        `env:"LOG_LEVEL" env-required:"true" validate:"oneof=debug info warn error"`
	ShutdownTimeout         time.Duration `env:"SHUTDOWN_TIMEOUT" env-required:"true" validate:"gt=0"`
	DatabaseURL             string        `env:"DATABASE_URL" env-required:"true" validate:"required"`
	DatabaseMaxConns        int32         `env:"DATABASE_MAX_CONNS" env-required:"true" validate:"gt=0"`
	DatabaseMinConns        int32         `env:"DATABASE_MIN_CONNS" env-required:"true" validate:"gte=0,ltefield=DatabaseMaxConns"`
	DatabaseMaxConnLifetime time.Duration `env:"DATABASE_MAX_CONN_LIFETIME" env-required:"true" validate:"gt=0"`
	DatabaseConnectTimeout  time.Duration `env:"DATABASE_CONNECT_TIMEOUT" env-required:"true" validate:"gt=0"`
	DatabaseQueryTimeout    time.Duration `env:"DATABASE_QUERY_TIMEOUT" env-required:"true" validate:"gt=0"`
}

func Load() (Config, error) {
	var cfg Config
	if err := cleanenv.ReadEnv(&cfg); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	cfg.LogLevel = strings.ToLower(cfg.LogLevel)
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config: %w", err)
	}
	return cfg, nil
}

func (c Config) Validate() error {
	_, port, err := net.SplitHostPort(c.HTTPAddr)
	if err != nil {
		return fmt.Errorf("HTTP_ADDR must have host:port format: %w", err)
	}

	portNumber, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("HTTP_ADDR port must be a number: %w", err)
	}
	if portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("HTTP_ADDR port must be between 1 and 65535")
	}
	c.DatabaseURL = strings.TrimSpace(c.DatabaseURL)
	c.LogLevel = strings.ToLower(c.LogLevel)
	validate := validator.New(validator.WithRequiredStructEnabled())
	return validate.Struct(c)
}
