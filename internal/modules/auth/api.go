// Package auth provides bot module middleware for authentication on bot commands
package auth

import (
	"errors"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/bot"
	"github.com/eientei/jaroid/internal/router"
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

func (mod *module) Configure(config *bot.Configuration, guild *discordgo.Guild) {

}

func (mod *module) Shutdown(config *bot.Configuration) {

}

func (mod *module) checkPermissions(ctx *router.Context, auth *RouteConfig) bool {
	for _, r := range ctx.Message.Member.Roles {
		role, err := ctx.Session.State.Role(ctx.Message.GuildID, r)
		if err != nil {
			mod.config.Log.WithError(err).Error("Loading role", ctx.Message.GuildID, r)
			continue
		}

		if auth.Permissions != 0 && role.Permissions&auth.Permissions != 0 {
			return true
		}

		for _, ri := range auth.RoleIDs {
			if ri == r {
				return true
			}
		}

		for _, rn := range auth.RoleNames {
			if role.Name == rn {
				return true
			}
		}
	}

	return false
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
