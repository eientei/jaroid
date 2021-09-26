// Package cleanup provides bot module for automated removal of bot replies
package cleanup

import (
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/discordbot/bot"
	"github.com/eientei/jaroid/discordbot/router"
)

// New provides module instance
func New() bot.Module {
	return &module{}
}

type module struct {
	cleanupDelay map[string]time.Duration
	config       *bot.Configuration
}

func (mod *module) Initialize(config *bot.Configuration) error {
	mod.cleanupDelay = make(map[string]time.Duration)
	mod.config = config
	config.Router.AppendMiddleware(mod.middlewareCleanup)

	go mod.start()

	return nil
}

func (mod *module) Configure(config *bot.Configuration, guild *discordgo.Guild) {
	s, err := config.Repository.ConfigGet(guild.ID, "cleanup", "delay")
	if err != nil {
		config.Log.WithError(err).Error("Getting cleanup delay")
		return
	}

	if s == "" {
		return
	}

	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		config.Log.WithError(err).Error("Parsing delay value")
		return
	}

	mod.cleanupDelay[guild.ID] = time.Duration(v)
}

func (mod *module) Shutdown(config *bot.Configuration) {

}

func (mod *module) middlewareCleanup(handler router.HandlerFunc) router.HandlerFunc {
	return func(ctx *router.Context) error {
		origerr := handler(ctx)

		for k, r := range ctx.Route.Replies {
			if mod.cleanupDelay[ctx.Message.GuildID] > 0 {
				_, err := mod.config.Repository.TaskEnqueue(&Task{
					GuildID:   r.Response.GuildID,
					ChannelID: r.Response.ChannelID,
					MessageID: r.Response.ID,
				}, mod.cleanupDelay[ctx.Message.GuildID], 0)
				if err != nil {
					mod.config.Log.WithError(err).WithField("response", r.Response).Error("Enqueueing response cleanup")
				}
			}

			delete(ctx.Route.Replies, k)
		}

		return origerr
	}
}
