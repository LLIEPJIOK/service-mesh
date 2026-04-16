package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	App   App          `envPrefix:"APP_"`
	Plane ControlPlane `envPrefix:"PLANE_"`
}

type App struct {
	TerminateTimeout time.Duration `env:"TERMINATE_TIMEOUT" envDefault:"5s"`
	ShutdownTimeout  time.Duration `env:"SHUTDOWN_TIMEOUT"  envDefault:"2s"`
}

type ControlPlane struct {
	URL               string        `env:"URL"                 envDefault:":8080"`
	ReadTimeout       time.Duration `env:"READ_TIMEOUT"        envDefault:"1s"`
	ReadHeaderTimeout time.Duration `env:"READ_HEADER_TIMEOUT" envDefault:"1s"`
}

func Load() (*Config, error) {
	config := &Config{}

	if err := env.Parse(config); err != nil {
		return nil, fmt.Errorf("failed to parse env: %w", err)
	}

	return config, nil
}
