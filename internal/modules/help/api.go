// Package help provides bot module for command help message
package help

import (
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/bot"
	"github.com/eientei/jaroid/internal/router"
)

// Module provides implementation for bot help command
type Module struct {
}

// Initialize initialized module at start
func (mod *Module) Initialize(config *bot.Configuration) error {
	config.Router.Group("info").On("help", "prints help", mod.commandHelp)
	return nil
}

// Configure configures module for given guild
func (mod *Module) Configure(config *bot.Configuration, guild *discordgo.Guild) {

}

// Shutdown tears-down bot module
func (mod *Module) Shutdown(config *bot.Configuration) {

}

func (mod *Module) commandHelp(ctx *router.Context) error {
	max := 0
	for _, v := range ctx.Route.Router.Routes {
		if len(v.Name) > max {
			max = len(v.Name)
		}
	}

	buf := &strings.Builder{}

	buf.WriteString("```\n")

	for _, g := range ctx.Route.Router.Groups {
		_, _ = buf.WriteString("\n==" + strings.ToUpper(g.Name) + "==\n")
		for _, v := range g.Routes {
			_, _ = buf.WriteString(strings.Repeat(" ", max-len(v.Name)))
			_, _ = buf.WriteString(v.Name)
			_, _ = buf.WriteString(": ")
			_, _ = buf.WriteString(v.Description)
			buf.WriteString("\n")
		}
	}

	buf.WriteString("```")

	return ctx.ReplyEmbed(buf.String())
}
