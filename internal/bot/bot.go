package bot

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/eientei/jaroid/internal/model"
	"github.com/eientei/jaroid/internal/router"
)

// Bot is a main implementation of bot
type Bot struct {
	options    Options
	servers    map[string]*server
	ctx        context.Context
	router     *router.Router
	repository model.Repository
}

func (bot *Bot) reload() error {
	for _, s := range bot.servers {
		prefix, err := bot.repository.ConfigGet(s.ID, "global", "prefix")
		if err != nil {
			return err
		}

		s.prefix = prefix
	}

	return nil
}

// Serve starts bot serving loop and blocks until exit
func (bot *Bot) Serve(ctx context.Context) error {
	bot.ctx = ctx

	err := bot.registerHandlers()
	if err != nil {
		return err
	}

	err = bot.options.Discord.Open()
	if err != nil {
		return err
	}

	bot.options.Log.Info("Running")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	return bot.options.Discord.Close()
}

func (bot *Bot) registerHandlers() error {
	bot.options.Discord.AddHandler(bot.guildCreate)
	bot.options.Discord.AddHandler(bot.guildMembersChunk)
	bot.options.Discord.AddHandler(bot.guildMemberAdd)
	bot.options.Discord.AddHandler(bot.messageCreate)
	bot.options.Discord.AddHandler(bot.messageUpdate)

	return nil
}
