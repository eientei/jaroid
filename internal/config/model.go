package config

import (
	"time"
)

// Redis connection part of configuration
type Redis struct {
	Address  string `yaml:"address"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// Nicovideo download configuration
type Nicovideo struct {
	Directory string        `yaml:"directory"`
	Public    string        `yaml:"public"`
	Opts      []string      `yaml:"opts"`
	Period    time.Duration `yaml:"period"`
	Backoff   time.Duration `yaml:"backoff"`
	Limit     int           `yaml:"limit"`
}

// Private part of configuration
type Private struct {
	Token     string    `yaml:"token"`
	Admins    string    `yaml:"admins"`
	Prefix    string    `yaml:"prefix"`
	Data      string    `yaml:"data"`
	Redis     Redis     `yaml:"redis"`
	Nicovideo Nicovideo `yaml:"nicovideo"`
}

// Server specific part of configuration
type Server struct {
	GuildID string   `yaml:"id"`
	Prefix  string   `yaml:"prefix"`
	LogDB   string   `yaml:"logdb"`
	Admins  []string `yaml:"admins"`
}

// Root of configuration
type Root struct {
	Servers []Server `yaml:"servers"`
	Private Private  `yaml:"private"`
}
