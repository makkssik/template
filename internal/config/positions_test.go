package config_test

import (
	"testing"
	"time"

	"github.com/makkssik/template/internal/config"
)

func TestPositionConfiguration(t *testing.T) {
	valid := config.Config{
		HTTPAddr:                "localhost:8080",
		HTTPReadTimeout:         time.Second,
		HTTPReadHeaderTimeout:   time.Second,
		HTTPWriteTimeout:        time.Second,
		HTTPIdleTimeout:         time.Second,
		LogLevel:                "info",
		ShutdownTimeout:         time.Second,
		DatabaseURL:             "postgres://localhost/tripgo",
		DatabaseMaxConns:        2,
		DatabaseMaxConnLifetime: time.Minute,
		DatabaseConnectTimeout:  time.Second,
		DatabaseQueryTimeout:    time.Second,
		OTELServiceName:         "trip-service",
		OTELEndpoint:            "http://localhost:4318",
		OTELProtocol:            "http/protobuf",
		OTELSampler:             "always_on",
		PositionCheckInterval:   time.Second,
		PositionStaleAfter:      time.Minute,
		PositionWorkers:         4,
		PositionQueueSize:       100,
		PositionBatchSize:       100,
		PushHTTPURL:             "http://localhost:8090",
		PushRequestTimeout:      time.Second,
		PushKeepAlive:           30 * time.Second,
		PushIdleConnTimeout:     90 * time.Second,
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		change func(*config.Config)
	}{
		{
			name: "interval",
			change: func(c *config.Config) {
				c.PositionCheckInterval = 0
			},
		},
		{
			name: "stale",
			change: func(c *config.Config) {
				c.PositionStaleAfter = -time.Second
			},
		},
		{
			name: "workers",
			change: func(c *config.Config) {
				c.PositionWorkers = 0
			},
		},
		{
			name: "queue",
			change: func(c *config.Config) {
				c.PositionQueueSize = -1
			},
		},
		{
			name: "batch",
			change: func(c *config.Config) {
				c.PositionBatchSize = 0
			},
		},
		{
			name: "URL",
			change: func(c *config.Config) {
				c.PushHTTPURL = ""
			},
		},
		{
			name: "protocol",
			change: func(c *config.Config) {
				c.PushHTTPURL = "ftp://localhost"
			},
		},
		{
			name: "timeout",
			change: func(c *config.Config) {
				c.PushRequestTimeout = 0
			},
		},
		{
			name: "zero keep alive",
			change: func(c *config.Config) {
				c.PushKeepAlive = 0
			},
		},
		{
			name: "negative keep alive",
			change: func(c *config.Config) {
				c.PushKeepAlive = -time.Second
			},
		},
		{
			name: "zero idle connection timeout",
			change: func(c *config.Config) {
				c.PushIdleConnTimeout = 0
			},
		},
		{
			name: "negative idle connection timeout",
			change: func(c *config.Config) {
				c.PushIdleConnTimeout = -time.Second
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := valid
			tc.change(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid position configuration accepted")
			}
		})
	}
}
