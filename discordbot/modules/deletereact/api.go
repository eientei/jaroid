// Package deletereact provides reaction on message deletion
package deletereact

import (
	"strings"
	"sync"

	"github.com/jmoiron/sqlx"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/discordbot/bot"
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
  content
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

func (mod *module) Shutdown(config *bot.Configuration) {

}

func (mod *module) handlerDelete(session *discordgo.Session, messageDelete *discordgo.MessageDelete) {
	mod.lock.RLock()
	defer mod.lock.RUnlock()

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

	err = db.author.QueryRow(messageDelete.ID).Scan(&authorID, &body)
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting author ID")
		return
	}

	member, err := session.GuildMember(messageDelete.GuildID, authorID)
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting member")
		return
	}

	name := member.Nick

	if name == "" && member.User != nil {
		name = member.User.Username
	}

	if name == "" {
		name = messageDelete.ID
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
