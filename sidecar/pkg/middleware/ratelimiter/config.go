package ratelimiter

import "time"

type Config struct {
	Name    string        `env:"NAME"`
	MaxHits int           `env:"MAX_HITS" envDefault:"10"`
	Window  time.Duration `env:"WINDOW"   envDefault:"1m"`
}
