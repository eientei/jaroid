// Package bot provides main bot implementation
package bot

import (
	"errors"
	"sync"
	"time"

	"github.com/eientei/jaroid/discordbot/config"
	"github.com/eientei/jaroid/discordbot/model"
	"github.com/eientei/jaroid/discordbot/router"
	"github.com/eientei/jaroid/integration/nicovideo"

	"github.com/bwmarrin/discordgo"
	redis "github.com/go-redis/redis/v7"
	"github.com/sirupsen/logrus"
)

// ErrNoReply special error value to avoid auto-reply
var ErrNoReply = errors.New("noreply")

// Options provide configuration options for bot
type Options struct {
	Discord   *discordgo.Session
	Client    *redis.Client
	Config    *config.Root
	Log       *logrus.Logger
	Nicovideo *nicovideo.Client
	Modules   []Module
}

// Configuration store configuration for bot
type Configuration struct {
	Discord    *discordgo.Session
	Client     *redis.Client
	Config     *config.Root
	Log        *logrus.Logger
	Router     *router.Router
	Repository *model.Repository
	Nicovideo  *nicovideo.Client
	bot        *Bot
	Modules    []Module
}

// HasRole returns true if user has specified role
func (conf *Configuration) HasRole(guildID, userID, roleID string) bool {
	guild := conf.bot.guild(guildID)

	return guild.hasRole(userID, roleID)
}

// SetPrefix sets guild prefix for module
func (conf *Configuration) SetPrefix(guildID, mod, prefix string) {
	guild := conf.bot.guild(guildID)

	guild.prefixes[mod] = prefix
}

func containsString(s string, ss ...string) bool {
	for _, ri := range ss {
		if ri == s {
			return true
		}
	}

	return false
}

func (conf *Configuration) ensureMember(msg *discordgo.Message) (*discordgo.Member, error) {
	if msg.Member != nil {
		return msg.Member, nil
	}

	var err error

	msg.Member, err = conf.Discord.GuildMember(msg.GuildID, msg.Author.ID)
	if err != nil {
		conf.Log.WithError(err).Error("Loading member", msg.GuildID, msg.Author.ID)

		return msg.Member, err
	}

	return msg.Member, nil
}

// HasPermission returns true if user has administrative or matching permissions
func (conf *Configuration) HasPermission(
	msg *discordgo.Message,
	permissions int,
	roleIDs, roleNames []string,
) bool {
	return conf.HasPermissionUserID(msg.Member, msg.GuildID, msg.Author.ID, permissions, roleIDs, roleNames)
}

func (conf *Configuration) HasPermissionUserID(
	member *discordgo.Member,
	guildID, userID string,
	permissions int,
	roleIDs, roleNames []string,
) bool {
	guild, _ := conf.Discord.Guild(guildID)
	if guild != nil && guild.OwnerID == userID {
		return true
	}

	admrole, _ := conf.Repository.ConfigGet(guildID, "auth", "admin.role")

	var err error

	if member == nil {
		member, err = conf.Discord.GuildMember(guildID, userID)
		if err != nil {
			conf.Log.WithError(err).Error("Loading member", guildID, userID)

			return false
		}
	}

	for _, r := range member.Roles {
		var role *discordgo.Role

		role, err = conf.Discord.State.Role(guildID, r)
		if err != nil {
			conf.Log.WithError(err).Error("Loading role", guildID, r)
			continue
		}

		if evalPermissions(role, permissions, r, admrole, roleIDs, roleNames) {
			return true
		}
	}

	return false
}

func evalPermissions(
	role *discordgo.Role,
	permissions int,
	r, admrole string,
	roleIDs, roleNames []string,
) bool {
	if permissions != 0 && role.Permissions&int64(permissions) != 0 {
		return true
	}

	if permissions&discordgo.PermissionAdministrator != 0 && r == admrole {
		return true
	}

	if containsString(r, roleIDs...) || containsString(role.Name, roleNames...) {
		return true
	}

	return false
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

	s.prefixes = map[string]string{
		"": prefix,
	}

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

		bot.configure(bot.guild(guild.ID), guild)

		for _, m := range bot.Modules {
			m.Configure(&bot.Configuration, guild)
		}
	}
}

// Module interface incapsulates methods for distinct functionality
type Module interface {
	Initialize(bot *Configuration) error
	Configure(bot *Configuration, server *discordgo.Guild)
	Shutdown(bot *Configuration)
}

// RoleModule interface marks modules interested in role changes
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
			Nicovideo:  options.Nicovideo,
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
