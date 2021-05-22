package bot

import (
	"sync"

	"github.com/bwmarrin/discordgo"
)

var empty = struct{}{}

func (bot *Bot) guild(guildID string) (guild *server) {
	bot.m.RLock()

	guild, ok := bot.servers[guildID]

	bot.m.RUnlock()

	if !ok {
		guild = &server{
			roles:   make(map[string]map[string]struct{}),
			members: make(map[string]map[string]struct{}),
			m:       &sync.RWMutex{},
		}

		bot.m.Lock()

		bot.servers[guildID] = guild

		bot.m.Unlock()
	}

	return
}

func (srv *server) memberBare(memberID string) (roles map[string]struct{}) {
	roles, ok := srv.members[memberID]
	if !ok {
		roles = make(map[string]struct{})
		srv.members[memberID] = roles
	}

	return
}

func (srv *server) roleBare(roleID string) (members map[string]struct{}) {
	members, ok := srv.roles[roleID]
	if !ok {
		members = make(map[string]struct{})
		srv.roles[roleID] = members
	}

	return
}

func (srv *server) hasRole(userID, roleID string) (res bool) {
	srv.m.RLock()

	members, ok := srv.roles[roleID]
	if ok {
		_, res = members[userID]
	}

	srv.m.RUnlock()

	return
}

func (srv *server) hasMembers(roleID string) (res bool) {
	srv.m.RLock()

	members, ok := srv.roles[roleID]
	if ok {
		res = len(members) > 0
	}

	srv.m.RUnlock()

	return
}

func (bot *Bot) handlerMembersChunk(session *discordgo.Session, chunk *discordgo.GuildMembersChunk) {
	guild := bot.guild(chunk.GuildID)

	for _, m := range chunk.Members {
		bot.memberSync(guild, m)
	}
}

func (bot *Bot) memberSync(guild *server, m *discordgo.Member) {
	var rolesAdded, rolesRemoved []string

	removed := make(map[string]struct{})

	guild.m.Lock()

	user := guild.memberBare(m.User.ID)

	for r := range user {
		removed[r] = empty
	}

	for _, r := range m.Roles {
		known := guild.roleBare(r)

		if _, ok := user[r]; !ok {
			rolesAdded = append(rolesAdded, r)
		}

		known[m.User.ID] = empty
		user[r] = empty

		delete(removed, r)
	}

	for r := range removed {
		rolesRemoved = append(rolesRemoved, r)

		known := guild.roleBare(r)

		delete(known, m.User.ID)

		if len(known) == 0 {
			delete(guild.roles, r)
		}

		delete(user, r)

		if len(user) == 0 {
			delete(guild.members, m.User.ID)
		}
	}

	guild.m.Unlock()

	if len(rolesAdded) == 0 && len(rolesRemoved) == 0 {
		return
	}

	for _, h := range bot.roleModules {
		h.RolesChanged(m.GuildID, m.User.ID, rolesAdded, rolesRemoved)
	}
}

func (bot *Bot) handlerMemberAdd(session *discordgo.Session, m *discordgo.GuildMemberAdd) {
	bot.memberSync(bot.guild(m.GuildID), m.Member)
}

func (bot *Bot) handlerMemberRemove(session *discordgo.Session, m *discordgo.GuildMemberRemove) {
	guild, ok := bot.servers[m.GuildID]
	if !ok {
		return
	}

	for r := range guild.members[m.User.ID] {
		roles, ok := guild.roles[r]
		if ok {
			delete(roles, m.User.ID)
		}
	}

	delete(guild.members, m.User.ID)
}

func (bot *Bot) handlerMemberUpdate(session *discordgo.Session, m *discordgo.GuildMemberUpdate) {
	bot.memberSync(bot.guild(m.GuildID), m.Member)
}
