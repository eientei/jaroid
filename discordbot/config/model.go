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

// NicovideoAuth authentication details
type NicovideoAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Nicovideo download configuration
type Nicovideo struct {
	Directory string        `yaml:"directory"`
	Public    string        `yaml:"public"`
	Auth      NicovideoAuth `yaml:"auth"`
	Period    time.Duration `yaml:"period"`
	Backoff   time.Duration `yaml:"backoff"`
	Limit     int           `yaml:"limit"`
}

// Pleroma nicomodule configuration
type Pleroma struct {
	Host string `yaml:"host"`
	Auth string `yaml:"auth"`
}

// Private part of configuration
type Private struct {
	Token        string            `yaml:"token"`
	Admins       string            `yaml:"admins"`
	Prefix       string            `yaml:"prefix"`
	ModulePrefix map[string]string `yaml:"module_prefix"`
	Data         string            `yaml:"data"`
	Redis        Redis             `yaml:"redis"`
	Nicovideo    Nicovideo         `yaml:"nicovideo"`
}

// Server specific part of configuration
type Server struct {
	GuildID      string            `yaml:"id"`
	Prefix       string            `yaml:"prefix"`
	ModulePrefix map[string]string `yaml:"module_prefix"`
	LogDB        string            `yaml:"logdb"`
	Pleroma      Pleroma           `yaml:"pleroma"`
	Admins       []string          `yaml:"admins"`
}

// Root of configuration
type Root struct {
	Servers []Server `yaml:"servers"`
	Private Private  `yaml:"private"`
}
