// Package help provides bot module for command help message
package help

import (
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/bot"
	"github.com/eientei/jaroid/internal/router"
)

// New provides module instacne
func New() bot.Module {
	return &module{}
}

type module struct {
}

func (mod *module) Initialize(config *bot.Configuration) error {
	group := config.Router.Group("info").SetDescription("help & status")

	group.On("help", "prints help", mod.commandHelp)

	return nil
}

func (mod *module) Configure(config *bot.Configuration, guild *discordgo.Guild) {

}

func (mod *module) Shutdown(config *bot.Configuration) {

}

func (mod *module) renderName(r *router.Route) string {
	if len(r.Alias) == 0 {
		return r.Name
	}

	return r.Name + " | " + strings.Join(r.Alias, " | ")
}

func (mod *module) commandHelp(ctx *router.Context) error {
	max := 0

	for _, v := range ctx.Route.Router.Routes {
		name := mod.renderName(v)
		if len(name) > max {
			max = len(name)
		}
	}

	buf := &strings.Builder{}

	buf.WriteString("```autohotkey\n")

	for _, g := range ctx.Route.Router.Groups {
		_, _ = buf.WriteString("\n==" + strings.ToUpper(g.Name) + "==")

		if len(g.Description) > 0 {
			_, _ = buf.WriteString(" ")
			_, _ = buf.WriteString(g.Description)
		}

		_, _ = buf.WriteString("\n")

		for _, v := range g.Routes {
			name := mod.renderName(v)
			_, _ = buf.WriteString(strings.Repeat(" ", max-len(name)))
			_, _ = buf.WriteString(name)
			_, _ = buf.WriteString(": ")
			_, _ = buf.WriteString(v.Description)
			buf.WriteString("\n")
		}
	}

	buf.WriteString("```")

	return ctx.ReplyEmbed(buf.String())
}
