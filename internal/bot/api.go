// Package bot provides main bot implementation
package bot

import (
	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/config"
	"github.com/eientei/jaroid/internal/model"
	"github.com/go-redis/redis/v7"
	"github.com/sirupsen/logrus"
)

// Options provide configuration for bot
type Options struct {
	Discord *discordgo.Session
	Client  *redis.Client
	Config  *config.Root
	Log     *logrus.Logger
}

// NewBot provides new instance of bot
func NewBot(options Options) *Bot {
	bot := &Bot{
		options:    options,
		servers:    make(map[string]*server),
		repository: model.NewRepository(options.Client),
	}
	bot.router = bot.newRouter()

	return bot
}
