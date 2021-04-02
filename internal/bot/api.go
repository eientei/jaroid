// Package bot provides main bot implementation
package bot

import (
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/config"
	"github.com/eientei/jaroid/internal/model"
	"github.com/eientei/jaroid/internal/router"
	"github.com/go-redis/redis/v7"
	"github.com/sirupsen/logrus"
)

// Options provide configuration options for bot
type Options struct {
	Discord *discordgo.Session
	Client  *redis.Client
	Config  *config.Root
	Log     *logrus.Logger
	Modules []Module
}

// Configuration store configuration for bot
type Configuration struct {
	Discord    *discordgo.Session
	Client     *redis.Client
	Config     *config.Root
	Log        *logrus.Logger
	Router     *router.Router
	Repository *model.Repository
	servers    map[string]*server
	Modules    []Module
}

func (conf *Configuration) configure(guild *discordgo.Guild) {
	s, ok := conf.servers[guild.ID]
	if !ok {
		s = &server{}
		conf.servers[guild.ID] = s
	}

	prefix, err := conf.Repository.ConfigGet(guild.ID, "global", "prefix")
	if err != nil {
		conf.Log.WithError(err).Error("Getting server prefix", guild.ID)
		return
	}

	if prefix == "" {
		for _, s := range conf.Config.Servers {
			if s.GuildID == guild.ID {
				prefix = s.Prefix
			}
		}
	}

	if prefix == "" {
		prefix = conf.Config.Private.Prefix
	}

	if prefix == "" {
		prefix = "!"
	}

	if conf.Config.Private.Nicovideo.Backoff == 0 {
		conf.Config.Private.Nicovideo.Backoff = time.Hour
	}

	s.prefix = prefix

	err = conf.Repository.ConfigSet(guild.ID, "global", "prefix", prefix)
	if err != nil {
		conf.Log.WithError(err).Error("Saving server prefix", guild.ID)
	}
}

// Reload performs reload of all configuration values in configured modules
func (conf *Configuration) Reload() {
	for k := range conf.servers {
		guild, err := conf.Discord.Guild(k)
		if err != nil {
			conf.Log.WithError(err).Error("Getting guild", k)
			continue
		}

		for _, m := range conf.Modules {
			m.Configure(conf, guild)
		}

		conf.configure(guild)
	}
}

// Module interface incapsulates methods for distinct functionality
type Module interface {
	Initialize(bot *Configuration) error
	Configure(bot *Configuration, server *discordgo.Guild)
	Shutdown(bot *Configuration)
}

// NewBot provides new instance of bot
func NewBot(options Options) (*Bot, error) {
	if options.Log == nil {
		options.Log = logrus.New()
	}

	bot := &Bot{
		Configuration: Configuration{
			Discord:    options.Discord,
			Client:     options.Client,
			Config:     options.Config,
			Log:        options.Log,
			Router:     router.NewRouter(),
			Repository: model.NewRepository(options.Client),
			Modules:    options.Modules,
			servers:    make(map[string]*server),
		},
	}

	for _, m := range bot.Modules {
		err := m.Initialize(&bot.Configuration)
		if err != nil {
			return nil, err
		}
	}

	bot.Discord.AddHandler(bot.handlerGuildCreate)
	bot.Discord.AddHandler(bot.handlerMessageCreate)
	bot.Discord.AddHandler(bot.handlerMessageUpdate)

	return bot, nil
}
