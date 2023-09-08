// Package deletereact provides reaction on message deletion
package deletereact

import (
	"strings"
	"sync"
	"time"

	"github.com/eientei/jaroid/discordbot/bot"

	"github.com/jmoiron/sqlx"

	"github.com/bwmarrin/discordgo"
)

// New provides module instance
func New() bot.Module {
	return &module{}
}

type dbcontext struct {
	connect *sqlx.DB
	author  *sqlx.Stmt
}

type module struct {
	config *bot.Configuration
	dbmap  map[string]*dbcontext
	lock   *sync.RWMutex
}

func (mod *module) Initialize(config *bot.Configuration) error {
	mod.config = config
	mod.dbmap = make(map[string]*dbcontext)
	mod.lock = &sync.RWMutex{}

	config.Discord.AddHandler(mod.handlerDelete)

	return nil
}

func (mod *module) Configure(config *bot.Configuration, guild *discordgo.Guild) {
	mod.lock.Lock()
	defer mod.lock.Unlock()

	for _, s := range config.Config.Servers {
		if s.GuildID == guild.ID && s.LogDB != "" {
			db, err := sqlx.Open("postgres", s.LogDB)
			if err != nil {
				config.Log.Errorln("opening database", err)
				continue
			}

			authorStmt, err := db.Preparex(`
select
  author_id,
  content,
  time
from message
where
  content is not null and
  mid = $1
order by id desc limit 1
`)
			if err != nil {
				config.Log.Errorln("preparing statement", err)
				continue
			}

			mod.dbmap[guild.ID] = &dbcontext{
				connect: db,
				author:  authorStmt,
			}
		}
	}
}

func (mod *module) Shutdown(_ *bot.Configuration) {

}

func resolveName(member *discordgo.Member, fallback string) string {
	name := member.Nick

	if name == "" && member.User != nil {
		name = member.User.Username
	}

	if name == "" {
		name = fallback
	}

	return name
}

func (mod *module) handlerDelete(session *discordgo.Session, messageDelete *discordgo.MessageDelete) {
	mod.lock.RLock()
	defer mod.lock.RUnlock()

	references, _ := mod.config.Repository.ConfigGet(messageDelete.GuildID, "deletereact", "reference")

	reference, _ := time.Parse(time.RFC3339, references)

	s, err := mod.config.Repository.ConfigGet(messageDelete.GuildID, "deletereact", "excluded")
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting excluded channels")
		return
	}

	parts := strings.Split(s, ",")
	for _, p := range parts {
		if messageDelete.ChannelID == p {
			return
		}
	}

	db, ok := mod.dbmap[messageDelete.GuildID]
	if !ok {
		return
	}

	template, err := mod.config.Repository.ConfigGet(messageDelete.GuildID, "deletereact", "template")
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting react template")
		return
	}

	if template == "" {
		return
	}

	var authorID, body string

	var messageTime time.Time

	err = db.author.QueryRow(messageDelete.ID).Scan(&authorID, &body, &messageTime)
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting author ID")
		return
	}

	if messageTime.Before(reference) {
		return
	}

	member, err := session.GuildMember(messageDelete.GuildID, authorID)
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting member")
		return
	}

	name := resolveName(member, messageDelete.ID)

	ms, err := session.GuildMembersSearch(messageDelete.GuildID, name, 100)
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting matching member list")
		return
	}

	var count int

	for _, m := range ms {
		if resolveName(m, messageDelete.ID) == name {
			count++
		}
	}

	if count > 1 && member.User != nil && member.User.Username != "" {
		name = member.User.Username + "#" + name
	}

	replacer := strings.NewReplacer(
		"$NAME", name,
		"$BODY", body,
	)

	_, err = session.ChannelMessageSend(messageDelete.ChannelID, replacer.Replace(template))
	if err != nil {
		mod.config.Log.WithError(err).Error("Sending delete reply")
		return
	}
}
