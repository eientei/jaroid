// Package leave provides interface for bot to leave a guild
package leave

import (
	"errors"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/discordbot/bot"
	"github.com/eientei/jaroid/discordbot/modules/auth"
	"github.com/eientei/jaroid/discordbot/router"
)

// New provides module instance
func New() bot.Module {
	return &module{}
}

type module struct {
	config *bot.Configuration
}

func (mod *module) Initialize(config *bot.Configuration) error {
	mod.config = config

	group := config.Router.Group("leave").SetDescription("leaving")
	group.Set(auth.RouteConfigKey, &auth.RouteConfig{
		Permissions: discordgo.PermissionAdministrator,
	})

	group.On("leave", "leave a guild", mod.commandLeave)

	return nil
}

func (mod *module) Configure(*bot.Configuration, *discordgo.Guild) {
}

func (mod *module) Shutdown(*bot.Configuration) {

}

func (mod *module) commandLeave(ctx *router.Context) error {
	var guildID string

	switch {
	case len(ctx.Args) > 1:
		guildID = ctx.Args[1]
	default:
		return errors.New("not found")
	}

	return ctx.Session.GuildLeave(guildID)
}
