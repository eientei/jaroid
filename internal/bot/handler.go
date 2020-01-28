package bot

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

func (bot *Bot) messageCreate(session *discordgo.Session, messageCreate *discordgo.MessageCreate) {
	if s, ok := bot.servers[messageCreate.GuildID]; ok {
		if strings.HasPrefix(messageCreate.Content, s.prefix) {
			_ = bot.router.Dispatch(session, s.prefix, session.State.User.ID, messageCreate.Message)
		}
	}
}

func (bot *Bot) messageUpdate(session *discordgo.Session, messageUpdate *discordgo.MessageUpdate) {
	msg, err := session.ChannelMessage(messageUpdate.ChannelID, messageUpdate.ID)
	if err != nil {
		bot.options.Log.WithError(err).Error("Getting message", messageUpdate.ID)
		return
	}

	for _, r := range msg.Reactions {
		if r.Me {
			return
		}
	}

	if s, ok := bot.servers[messageUpdate.GuildID]; ok {
		if strings.HasPrefix(messageUpdate.Content, s.prefix) {
			_ = bot.router.Dispatch(session, s.prefix, session.State.User.ID, messageUpdate.Message)
		}
	}
}

func (bot *Bot) guildMemberAdd(_ *discordgo.Session, guildMemberAdd *discordgo.GuildMemberAdd) {
	if s, ok := bot.servers[guildMemberAdd.GuildID]; ok {
		bot.mergeMember(s, guildMemberAdd.Member)
	}
}

func (bot *Bot) guildMembersChunk(_ *discordgo.Session, guildMembersChunk *discordgo.GuildMembersChunk) {
	if server, ok := bot.servers[guildMembersChunk.GuildID]; ok {
		for _, m := range guildMembersChunk.Members {
			bot.mergeMember(server, m)
		}
	}
}

func (bot *Bot) mergeMember(server *server, m *discordgo.Member) {
	u, ok := server.users[m.User.ID]
	if !ok {
		u = &user{
			ID:    m.User.ID,
			roles: make(map[string]*role),
		}
		server.users[m.User.ID] = u
	}

	for _, ri := range m.Roles {
		u.roles = make(map[string]*role)
		if r, ok := server.roles[ri]; ok {
			u.roles[ri] = r
		}
	}
}

func (bot *Bot) guildCreate(_ *discordgo.Session, guildCreate *discordgo.GuildCreate) {
	err := bot.mergeServer(guildCreate.Guild)
	if err != nil {
		bot.options.Log.WithError(err).Error("Merging server", guildCreate.Guild)
	}
}

func (bot *Bot) mergeServer(guild *discordgo.Guild) error {
	s, ok := bot.servers[guild.ID]
	if !ok {
		s = &server{
			ID:     guild.ID,
			values: make(map[string]map[string]string),
			users:  make(map[string]*user),
			roles:  make(map[string]*role),
		}
		bot.servers[guild.ID] = s
	}

	prefix, err := bot.repository.ConfigGet(guild.ID, "global", "prefix")
	if err != nil {
		return err
	}

	if prefix == "" {
		prefix = bot.options.Config.Private.Prefix

		err = bot.repository.ConfigSet(guild.ID, "global", "prefix", prefix)
		if err != nil {
			return err
		}
	}

	s.prefix = prefix

	roles, err := bot.options.Discord.GuildRoles(guild.ID)
	if err != nil {
		bot.options.Log.WithError(err).Error("Fetching roles")
		return err
	}

	for _, r := range roles {
		bot.mergeRole(s, r)
	}

	err = bot.options.Discord.RequestGuildMembers(guild.ID, "", 0)
	if err != nil {
		bot.options.Log.WithError(err).Error("Requesting members")
		return err
	}

	return nil
}

func (bot *Bot) mergeRole(s *server, ri *discordgo.Role) {
	_, ok := s.roles[ri.ID]
	if !ok {
		s.roles[ri.ID] = &role{
			ID:    ri.ID,
			users: make(map[string]*user),
		}
	}
}
