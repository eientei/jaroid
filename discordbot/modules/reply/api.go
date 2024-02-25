// Package reply provides bot module for automated emoji and error replies depending on result of execution
package reply

import (
	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/discordbot/bot"
	"github.com/eientei/jaroid/discordbot/router"
)

const (
	emojiOkButton = "\xf0\x9f\x86\x97"
	emojiX        = "\xe2\x9d\x8c"
)

// New provides module instacne
func New() bot.Module {
	return &module{}
}

type module struct {
	config *bot.Configuration
}

func (mod *module) Initialize(config *bot.Configuration) error {
	mod.config = config

	config.Router.AppendMiddleware(mod.middlewareReply)

	return nil
}

func (mod *module) Configure(*bot.Configuration, *discordgo.Guild) {

}

func (mod *module) Shutdown(*bot.Configuration) {

}

func (mod *module) middlewareReply(handler router.HandlerFunc) router.HandlerFunc {
	return func(ctx *router.Context) error {
		origerr := handler(ctx)
		if origerr == bot.ErrNoReply {
			return nil
		}

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
