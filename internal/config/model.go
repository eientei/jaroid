package config

// Redis connection part of configuration
type Redis struct {
	Address  string `yaml:"address"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

// Private part of configuration
type Private struct {
	Token  string `yaml:"token"`
	Admins string `yaml:"admins"`
	Prefix string `yaml:"prefix"`
	Data   string `yaml:"data"`
	Redis  Redis  `yaml:"redis"`
}

// Server specific part of configuration
type Server struct {
	GuildID string   `yaml:"id"`
	Admins  []string `yaml:"admins"`
	Prefix  string   `yaml:"prefix"`
}

// Root of configuration
type Root struct {
	Private Private  `yaml:"private"`
	Servers []Server `yaml:"servers"`
}
