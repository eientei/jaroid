package bot

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// Bot is a main implementation of bot
type Bot struct {
	Configuration
	m           *sync.RWMutex
	servers     map[string]*server
	roleModules []RoleModule
}

// Serve starts bot serving loop and blocks until exit
func (bot *Bot) Serve() error {
	err := bot.Discord.Open()
	if err != nil {
		return err
	}

	bot.Log.Info("Running")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	for _, m := range bot.Modules {
		m.Shutdown(&bot.Configuration)
	}

	return bot.Discord.Close()
}
