// Package logdb provides logging messages in database
package logdb

import (
	"sync"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/bot"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq" // Postgres driver
)

// New provides module instance
func New() bot.Module {
	return &module{}
}

type dbcontext struct {
	connect *sqlx.DB
	save    *sqlx.Stmt
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

	config.Discord.AddHandler(mod.handlerLogCreate)
	config.Discord.AddHandler(mod.handlerLogEdit)
	config.Discord.AddHandler(mod.handlerLogDelete)

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

			saveStmt, err := db.Preparex(`
insert into message(
  author_id,
  channel_id,
  content,
  mid,
  time
) values (
  $1,
  $2,
  $3,
  $4,
  now()
)
`)
			if err != nil {
				config.Log.Errorln("preparing statement", err)
				continue
			}

			mod.dbmap[guild.ID] = &dbcontext{
				connect: db,
				save:    saveStmt,
			}
		}
	}
}

func (mod *module) Shutdown(config *bot.Configuration) {
	mod.lock.Lock()
	defer mod.lock.Unlock()

	for _, d := range mod.dbmap {
		_ = d.connect.Close()
	}
}

func (mod *module) handlerLogCreate(session *discordgo.Session, messageCreate *discordgo.MessageCreate) {
	mod.lock.RLock()
	defer mod.lock.RUnlock()

	db, ok := mod.dbmap[messageCreate.GuildID]
	if !ok {
		return
	}

	if db.save != nil {
		_, err := db.save.Exec(
			messageCreate.Author.ID,
			messageCreate.ChannelID,
			messageCreate.Content,
			messageCreate.ID,
		)
		if err != nil {
			mod.config.Log.Errorln("Saving message", err)
		}
	}
}

func (mod *module) handlerLogEdit(session *discordgo.Session, messageUpdate *discordgo.MessageUpdate) {
	mod.lock.RLock()
	defer mod.lock.RUnlock()

	db, ok := mod.dbmap[messageUpdate.GuildID]
	if !ok {
		return
	}

	msg, err := session.ChannelMessage(messageUpdate.ChannelID, messageUpdate.ID)
	if err != nil {
		mod.config.Log.Errorln("Getting message", err)
	}

	if db.save != nil {
		_, err := db.save.Exec(
			msg.Author.ID,
			msg.ChannelID,
			msg.Content,
			msg.ID,
		)
		if err != nil {
			mod.config.Log.Errorln("Saving message", err)
		}
	}
}

func (mod *module) handlerLogDelete(session *discordgo.Session, messageDelete *discordgo.MessageDelete) {
	mod.lock.RLock()
	defer mod.lock.RUnlock()

	db, ok := mod.dbmap[messageDelete.GuildID]
	if !ok {
		return
	}

	if db.save != nil {
		_, err := db.save.Exec(
			nil,
			messageDelete.ChannelID,
			nil,
			messageDelete.ID,
		)
		if err != nil {
			mod.config.Log.Errorln("Saving message", err)
		}
	}
}
