// Package reply provides bot module for automated emoji and error replies depending on result of execution
package reply

import (
	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/bot"
	"github.com/eientei/jaroid/internal/router"
)

const (
	emojiOkButton = "\xf0\x9f\x86\x97"
	emojiX        = "\xe2\x9d\x8c"
)

// Module provides implementation for automated emoji reactions and replies depending on result of command execution
type Module struct {
	config *bot.Configuration
}

// Initialize initialized module at start
func (mod *Module) Initialize(config *bot.Configuration) error {
	mod.config = config

	config.Router.AppendMiddleware(mod.middlewareCleanup)

	return nil
}

// Configure configures module for given guild
func (mod *Module) Configure(config *bot.Configuration, guild *discordgo.Guild) {

}

// Shutdown tears-down bot module
func (mod *Module) Shutdown(config *bot.Configuration) {

}

func (mod *Module) middlewareCleanup(handler router.HandlerFunc) router.HandlerFunc {
	return func(ctx *router.Context) error {
		origerr := handler(ctx)
		if origerr != nil {
			mod.config.Log.WithError(origerr).
				WithField("route", ctx.Route).
				WithField("msg", ctx.Message).
				Errorf("executing command returned error")

			err := ctx.React(emojiX)
			if err != nil {
				mod.config.Log.WithError(err).Error("Replying with error status")
				return origerr
			}

			err = ctx.ReplyEmbed(origerr.Error())
			if err != nil {
				mod.config.Log.WithError(err).Error("Replying with error status")

				return origerr
			}

			return origerr
		}

		err := ctx.React(emojiOkButton)
		if err != nil {
			mod.config.Log.WithError(err).Error("Replying with ok status")
		}

		return nil
	}
}
