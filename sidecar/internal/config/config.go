package config

import (
	"fmt"
	"time"

	clientcfg "github.com/LLIEPJIOK/sidecar/pkg/client/config"
	"github.com/LLIEPJIOK/sidecar/pkg/middleware/ratelimiter"
	"github.com/caarlos0/env/v11"
)

type Config struct {
	App         App                `envPrefix:"APP_"`
	SideCar     SideCar            `envPrefix:"SIDECAR_"`
	Client      clientcfg.Config   `envPrefix:"CLIENT_"`
	RateLimiter ratelimiter.Config `envPrefix:"RATELIMITER_"`
}

type App struct {
	TerminateTimeout time.Duration `env:"TERMINATE_TIMEOUT" envDefault:"5s"`
	ShutdownTimeout  time.Duration `env:"SHUTDOWN_TIMEOUT"  envDefault:"2s"`
}

type SideCar struct {
	Target            string        `env:"TARGET,required"`
	ServiceName       string        `env:"SERVICE_NAME,required"`
	Port              int           `env:"PORT"                  envDefault:"8080"`
	ReadTimeout       time.Duration `env:"READ_TIMEOUT"          envDefault:"1s"`
	ReadHeaderTimeout time.Duration `env:"READ_HEADER_TIMEOUT"   envDefault:"1s"`
}

func Load() (*Config, error) {
	config := &Config{}

	if err := env.Parse(config); err != nil {
		return nil, fmt.Errorf("failed to parse env: %w", err)
	}

	return config, nil
}
