// Package join provides handling for new server members
package join

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/bot"
	"github.com/eientei/jaroid/internal/modules/auth"
	"github.com/eientei/jaroid/internal/modules/cleanup"
	"github.com/eientei/jaroid/internal/router"
)

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

func (mod *module) handlerGreet(session *discordgo.Session, guildMemberAdd *discordgo.GuildMemberAdd) {
	greetChannelID, err := mod.config.Repository.ConfigGet(guildMemberAdd.GuildID, "join", "greet.channel")
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting greeting channel")
		return
	}

	if greetChannelID == "" {
		return
	}

	greetText, err := mod.config.Repository.ConfigGet(guildMemberAdd.GuildID, "join", "greet.text")
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting greeting text")
		return
	}

	if greetText == "" {
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
