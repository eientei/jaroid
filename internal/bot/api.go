// Package bot provides main bot implementation
package bot

import (
	"errors"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/config"
	"github.com/eientei/jaroid/internal/model"
	"github.com/eientei/jaroid/internal/router"
	"github.com/go-redis/redis/v7"
	"github.com/sirupsen/logrus"
)

// ErrNoReply special error value to avoid auto-reply
var ErrNoReply = errors.New("noreply")

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
	bot        *Bot
	Modules    []Module
}

// HasRole returns true if user has specified role
func (conf *Configuration) HasRole(guildID, userID, roleID string) bool {
	guild := conf.bot.guild(guildID)

	return guild.hasRole(userID, roleID)
}

// HasMembers returns true if role exists and has non-zero number of members
func (conf *Configuration) HasMembers(guildID, roleID string) bool {
	guild := conf.bot.guild(guildID)

	return guild.hasMembers(roleID)
}

// Reload provides config reloading interface to modules
func (conf *Configuration) Reload() {
	conf.bot.Reload()
}

func (bot *Bot) configure(s *server, guild *discordgo.Guild) {
	prefix, err := bot.Repository.ConfigGet(guild.ID, "global", "prefix")
	if err != nil {
		bot.Log.WithError(err).Error("Getting server prefix", guild.ID)
		return
	}

	if prefix == "" {
		for _, srv := range bot.Config.Servers {
			if srv.GuildID == guild.ID {
				prefix = srv.Prefix
			}
		}
	}

	if prefix == "" {
		prefix = bot.Config.Private.Prefix
	}

	if prefix == "" {
		prefix = "!"
	}

	if bot.Config.Private.Nicovideo.Backoff == 0 {
		bot.Config.Private.Nicovideo.Backoff = time.Hour
	}

	if bot.Config.Private.Nicovideo.Limit == 0 {
		bot.Config.Private.Nicovideo.Limit = 100
	}

	s.prefix = prefix

	err = bot.Repository.ConfigSet(guild.ID, "global", "prefix", prefix)
	if err != nil {
		bot.Log.WithError(err).Error("Saving server prefix", guild.ID)
	}
}

// Reload performs reload of all configuration values in configured modules
func (bot *Bot) Reload() {
	for k := range bot.servers {
		guild, err := bot.Discord.Guild(k)
		if err != nil {
			bot.Log.WithError(err).Error("Getting guild", k)
			continue
		}

		for _, m := range bot.Modules {
			m.Configure(&bot.Configuration, guild)
		}

		bot.configure(bot.guild(guild.ID), guild)
	}
}

// Module interface incapsulates methods for distinct functionality
type Module interface {
	Initialize(bot *Configuration) error
	Configure(bot *Configuration, server *discordgo.Guild)
	Shutdown(bot *Configuration)
}

// RoleModule interace marks modules interested in role changes
type RoleModule interface {
	RolesChanged(guildID, userID string, added, removed []string)
}

// NewBot provides new instance of bot
func NewBot(options Options) (*Bot, error) {
	if options.Log == nil {
		options.Log = logrus.New()
	}

	var roleModules []RoleModule

	for _, m := range options.Modules {
		rm, ok := m.(RoleModule)
		if ok {
			roleModules = append(roleModules, rm)
		}
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
		},
		m:           &sync.RWMutex{},
		roleModules: roleModules,
		servers:     make(map[string]*server),
	}

	bot.Configuration.bot = bot

	for _, m := range bot.Modules {
		err := m.Initialize(&bot.Configuration)
		if err != nil {
			return nil, err
		}
	}

	bot.Discord.AddHandler(bot.handlerGuildCreate)
	bot.Discord.AddHandler(bot.handlerMessageCreate)
	bot.Discord.AddHandler(bot.handlerMessageUpdate)
	bot.Discord.AddHandler(bot.handlerMemberAdd)
	bot.Discord.AddHandler(bot.handlerMembersChunk)
	bot.Discord.AddHandler(bot.handlerMemberRemove)
	bot.Discord.AddHandler(bot.handlerMemberUpdate)

	return bot, nil
}
