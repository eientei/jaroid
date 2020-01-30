package bot

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

func (bot *Bot) handlerMessageCreate(session *discordgo.Session, messageCreate *discordgo.MessageCreate) {
	if s, ok := bot.servers[messageCreate.GuildID]; ok {
		if strings.HasPrefix(messageCreate.Content, s.prefix) {
			_ = bot.Router.Dispatch(session, s.prefix, session.State.User.ID, messageCreate.Message)
		}
	}
}

func (bot *Bot) handlerMessageUpdate(session *discordgo.Session, messageUpdate *discordgo.MessageUpdate) {
	msg, err := session.ChannelMessage(messageUpdate.ChannelID, messageUpdate.ID)
	if err != nil {
		bot.Log.WithError(err).Error("Getting message", messageUpdate.ID)
		return
	}

	for _, r := range msg.Reactions {
		if r.Me {
			return
		}
	}

	if s, ok := bot.servers[messageUpdate.GuildID]; ok {
		if strings.HasPrefix(messageUpdate.Content, s.prefix) {
			_ = bot.Router.Dispatch(session, s.prefix, session.State.User.ID, messageUpdate.Message)
		}
	}
}

func (bot *Bot) handlerGuildCreate(_ *discordgo.Session, guildCreate *discordgo.GuildCreate) {
	for _, m := range bot.Modules {
		m.Configure(&bot.Configuration, guildCreate.Guild)
	}

	bot.configure(guildCreate.Guild)
}
