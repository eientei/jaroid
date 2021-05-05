// Package join provides handling for new server members
package join

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/bot"
	"github.com/eientei/jaroid/internal/modules/auth"
	"github.com/eientei/jaroid/internal/modules/cleanup"
	"github.com/eientei/jaroid/internal/router"
)

var errNoGreet = errors.New("no greet")

// New provides module instacne
func New() bot.Module {
	return &module{}
}

type module struct {
	config *bot.Configuration
}

func (mod *module) Initialize(config *bot.Configuration) error {
	mod.config = config

	if mod.config.Config.Private.Nicovideo.Backoff == 0 {
		mod.config.Config.Private.Nicovideo.Backoff = time.Hour
	}

	config.Discord.AddHandler(mod.handlerGreet)
	config.Discord.AddHandler(mod.handlerAutorole)

	group := config.Router.Group("join").SetDescription("join config")
	group.On("join.test", "performs test of join handler", mod.commandTest).Set(auth.RouteConfigKey, &auth.RouteConfig{
		Permissions: discordgo.PermissionAdministrator,
	})

	return nil
}

func (mod *module) loadIgnorePatterns(guildID string) (m map[string]*regexp.Regexp) {
	prefix := guildID + ".join.ignore."

	rs, err := mod.config.Client.Keys(prefix + "*").Result()
	if err != nil {
		mod.config.Log.WithError(err).Error("fetching ignored join patterns")
	}

	m = make(map[string]*regexp.Regexp)

	for _, k := range rs {
		v, err := mod.config.Client.Get(k).Result()
		if err != nil {
			mod.config.Log.WithError(err).Errorf("getting key %s", k)
			continue
		}

		reg, err := regexp.Compile(v)
		if err != nil {
			mod.config.Log.WithError(err).Errorf("compiling pattern %s", reg)
			continue
		}

		m[strings.TrimPrefix(k, prefix)] = reg
	}

	return
}

func (mod *module) Configure(config *bot.Configuration, guild *discordgo.Guild) {
}

func (mod *module) Shutdown(config *bot.Configuration) {

}

func (mod *module) commandTest(ctx *router.Context) error {
	ctx.Message.Member.GuildID = ctx.Message.GuildID
	ctx.Message.Member.User = ctx.Message.Author

	mod.handlerGreet(ctx.Session, &discordgo.GuildMemberAdd{
		Member: ctx.Message.Member,
	})

	return nil
}

func (mod *module) handlerAutorole(session *discordgo.Session, guildMemberAdd *discordgo.GuildMemberAdd) {
	if mod.matchUserPatterns(guildMemberAdd.GuildID, guildMemberAdd.User.ID) {
		return
	}

	roleID, err := mod.config.Repository.ConfigGet(guildMemberAdd.GuildID, "join", "assign.role")
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting role to assign")
		return
	}

	if roleID == "" {
		return
	}

	err = session.GuildMemberRoleAdd(guildMemberAdd.GuildID, guildMemberAdd.User.ID, roleID)
	if err != nil {
		mod.config.Log.WithError(err).Error("Assigning role to user")
		return
	}
}

func (mod *module) matchUserPatterns(guildID, userID string) bool {
	s, _ := mod.config.Repository.ConfigGet(guildID, "join", "mintime")

	var dur time.Duration

	var err error

	if s != "" {
		dur, err = time.ParseDuration(s)
		if err != nil {
			mod.config.Log.WithError(err).Errorf("error parsing duration: %s", s)

			return false
		}
	}

	ts, err := discordgo.SnowflakeTimestamp(userID)
	if err != nil {
		mod.config.Log.WithError(err).Errorf("error parsing snowflakeID: %s", userID)

		return false
	}

	if dur > 0 && time.Since(ts) < dur {
		mod.config.Log.Infof("user was created too recently %s: %v < %v", userID, time.Since(ts), dur)

		return false
	}

	user, err := mod.config.Discord.User(userID)
	if err != nil {
		mod.config.Log.WithError(err).Errorf("Getting user %s", userID)

		return false
	}

	patterns := mod.loadIgnorePatterns(guildID)
	for _, p := range patterns {
		if p.MatchString(user.Username) {
			mod.config.Log.Infof("matched user ID %s username %s against %s", user.ID, user.Username, p.String())

			return true
		}
	}

	return false
}

func (mod *module) loadGreetingChannelText(guildID string) (greetChannelID, greetText string, err error) {
	greetChannelID, err = mod.config.Repository.ConfigGet(guildID, "join", "greet.channel")
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting greeting channel")
		return
	}

	if greetChannelID == "" {
		return "", "", errNoGreet
	}

	greetText, err = mod.config.Repository.ConfigGet(guildID, "join", "greet.text")
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting greeting text")
		return
	}

	if greetText == "" {
		return "", "", errNoGreet
	}

	return
}

func (mod *module) handlerGreet(session *discordgo.Session, guildMemberAdd *discordgo.GuildMemberAdd) {
	if mod.matchUserPatterns(guildMemberAdd.GuildID, guildMemberAdd.User.ID) {
		return
	}

	greetChannelID, greetText, err := mod.loadGreetingChannelText(guildMemberAdd.GuildID)
	if err != nil {
		return
	}

	greetText = strings.ReplaceAll(greetText, "USER", fmt.Sprintf("<@!%s>", guildMemberAdd.User.ID))
	greetText = strings.ReplaceAll(greetText, "\\n", "\n")

	msg, err := session.ChannelMessageSend(greetChannelID, greetText)
	if err != nil {
		mod.config.Log.WithError(err).Error("Sending greeting")
		return
	}

	attach, err := mod.config.Repository.ConfigGet(guildMemberAdd.GuildID, "join", "greet.attach")
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting greeting attachment")
		return
	}

	var attachmsg *discordgo.Message

	if attach != "" {
		attachmsg, err = session.ChannelMessageSendEmbed(greetChannelID, &discordgo.MessageEmbed{
			URL: attach,
			Image: &discordgo.MessageEmbedImage{
				URL: attach,
			},
		})
		if err != nil {
			mod.config.Log.WithError(err).Error("Sending greeting")
			return
		}
	}

	autoremoveRaw, err := mod.config.Repository.ConfigGet(guildMemberAdd.GuildID, "join", "greet.autoremove")
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting greeting autoremove")
		return
	}

	if autoremoveRaw == "" {
		return
	}

	autoremove, err := time.ParseDuration(autoremoveRaw)
	if err != nil {
		mod.config.Log.WithError(err).Error("Parsing autoremove duration")
		return
	}

	err = mod.config.Repository.TaskEnqueue(&cleanup.Task{
		GuildID:   msg.GuildID,
		ChannelID: msg.ChannelID,
		MessageID: msg.ID,
	}, autoremove, 0)
	if err != nil {
		mod.config.Log.WithError(err).Error("Placing autoremove task")
	}

	if attachmsg != nil {
		err = mod.config.Repository.TaskEnqueue(&cleanup.Task{
			GuildID:   attachmsg.GuildID,
			ChannelID: attachmsg.ChannelID,
			MessageID: attachmsg.ID,
		}, autoremove, 0)
		if err != nil {
			mod.config.Log.WithError(err).Error("Placing attach autoremove task")
		}
	}
}
