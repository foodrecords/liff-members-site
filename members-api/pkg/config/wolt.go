package config

import (
	"github.com/caarlos0/env"
)

var Wolt *wolt

type wolt struct {
	Token          string `env:"WOLT_TOKEN"`
	DefaultVenueID string `env:"WOLT_DEFAULT_VENUE_ID"`
}

func init() {
	wolt := wolt{}

	if err := env.Parse(&wolt); err != nil {
		panic(err)
	}

	Wolt = &wolt
}
