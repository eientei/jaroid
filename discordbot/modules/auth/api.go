// Package auth provides bot module middleware for authentication on bot commands
package auth

import (
	"errors"

	"github.com/eientei/jaroid/discordbot/bot"
	"github.com/eientei/jaroid/discordbot/router"

	"github.com/bwmarrin/discordgo"
)

// RouteConfigKey is used in route/group data configuration
const RouteConfigKey = "auth"

var (
	// ErrNotAuthorized is returned when user is not authorized to execute this command
	ErrNotAuthorized = errors.New("not authorized")
)

// RouteConfig holds authentication requirements for given route or route group
type RouteConfig struct {
	RoleIDs     []string
	RoleNames   []string
	Permissions int
}

// New provides module instacne
func New() bot.Module {
	return &module{}
}

type module struct {
	config *bot.Configuration
}

func (mod *module) Initialize(config *bot.Configuration) error {
	mod.config = config
	config.Router.AppendMiddleware(mod.middlewareAuth)

	return nil
}

func (mod *module) Configure(*bot.Configuration, *discordgo.Guild) {

}

func (mod *module) Shutdown(*bot.Configuration) {

}

func (mod *module) checkPermissions(ctx *router.Context, auth *RouteConfig) bool {
	return mod.config.AuthorHasPermission(ctx.Message, auth.Permissions, auth.RoleIDs, auth.RoleNames)
}

func (mod *module) middlewareAuth(handler router.HandlerFunc) router.HandlerFunc {
	return func(ctx *router.Context) error {
		raw := ctx.Route.Get(RouteConfigKey)

		var auth *RouteConfig

		switch v := raw.(type) {
		case *RouteConfig:
			auth = v
		case RouteConfig:
			auth = &v
		default:
			return handler(ctx)
		}

		if mod.checkPermissions(ctx, auth) {
			return handler(ctx)
		}

		return ErrNotAuthorized
	}
}
