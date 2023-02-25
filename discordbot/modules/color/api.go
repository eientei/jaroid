// Package color provides colored roles management
package color

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/discordbot/bot"
	"github.com/eientei/jaroid/discordbot/router"
	colorful "github.com/lucasb-eyer/go-colorful"
)

var (
	// ErrInvalidArgumentNumber is returned on invalid argument number
	ErrInvalidArgumentNumber = errors.New("invalid argument number, use color.help")
)

type server struct {
	colorroles map[string]*discordgo.Role
}

// New provides module instacne
func New() bot.Module {
	return &module{
		servers: make(map[string]*server),
	}
}

type module struct {
	config  *bot.Configuration
	servers map[string]*server
}

func (mod *module) Initialize(config *bot.Configuration) error {
	mod.config = config

	group := config.Router.Group("color").SetDescription("color roles")
	group.OnAlias("color.set", "sets colored role", []string{"color"}, true, mod.commandSet)
	group.On("color.remove", "removes colored role", mod.commandRemove)
	group.On("color.help", "provides documentation", mod.commandHelp)

	return nil
}

func (mod *module) RolesChanged(guildID, userID string, added, removed []string) {
	guild, ok := mod.servers[guildID]
	if !ok {
		return
	}

	for _, r := range added {
		if _, ok := guild.colorroles[r]; ok {
			continue
		}

		role, err := mod.config.Discord.State.Role(guildID, r)
		if err != nil {
			continue
		}

		if !strings.HasPrefix(role.Name, "color") {
			continue
		}

		guild.colorroles[r] = role
	}

	for _, r := range removed {
		role, ok := guild.colorroles[r]
		if !ok {
			continue
		}

		if !mod.config.HasMembers(guildID, r) {
			mod.config.Log.Infof("deleting role %s.%s (%s)", guildID, role.ID, role.Name)

			err := mod.config.Discord.GuildRoleDelete(guildID, r)
			if err != nil {
				mod.config.Log.Errorf("deleting role %s.%s: %v", guildID, r, err)
			}

			delete(guild.colorroles, r)
		}
	}
}

func (mod *module) Configure(config *bot.Configuration, guild *discordgo.Guild) {
	if _, ok := mod.servers[guild.ID]; !ok {
		mod.servers[guild.ID] = &server{
			colorroles: make(map[string]*discordgo.Role),
		}
	}

	prefix, err := config.Repository.ConfigGet(guild.ID, "color", "prefix")
	if err != nil {
		config.Log.WithError(err).Error("Getting color prefix", guild.ID)

		return
	}

	if prefix != "" {
		config.SetPrefix(guild.ID, "color", prefix)
	}
}

func (mod *module) Shutdown(config *bot.Configuration) {

}

func (mod *module) createrole(guildID string, c colorful.Color) (role *discordgo.Role, err error) {
	hex := c.Hex()

	r, g, b := c.RGB255()

	v := int(r)<<16 | int(g)<<8 | int(b)

	hoist := false

	permissions := int64(0)

	mentionable := false

	role, err = mod.config.Discord.GuildRoleCreate(guildID, &discordgo.RoleParams{
		Name:        "color" + hex,
		Color:       &v,
		Hoist:       &hoist,
		Permissions: &permissions,
		Mentionable: &mentionable,
	})
	if err != nil {
		return nil, fmt.Errorf("creating role: %w", err)
	}

	return
}

func (mod *module) setcolor(session *discordgo.Session, guildID, userID string, c colorful.Color) (err error) {
	guild, ok := mod.servers[guildID]
	if !ok {
		return nil
	}

	var role, old *discordgo.Role

	hex := c.Hex()

	for _, r := range guild.colorroles {
		if r.Name == "color"+hex {
			role = r
		}

		if mod.config.HasRole(guildID, userID, r.ID) {
			old = r
		}
	}

	if role == nil {
		role, err = mod.createrole(guildID, c)
		if err != nil {
			return fmt.Errorf("creating role: %w", err)
		}
	}

	if old != nil {
		err = session.GuildMemberRoleRemove(guildID, userID, old.ID)
		if err != nil {
			return fmt.Errorf("removing old role: %w", err)
		}
	}

	err = session.GuildMemberRoleAdd(guildID, userID, role.ID)
	if err != nil {
		return fmt.Errorf("adding new role: %w", err)
	}

	return nil
}

func (mod *module) commandRemove(ctx *router.Context) error {
	guild, ok := mod.servers[ctx.Message.GuildID]
	if !ok {
		return nil
	}

	for cr := range guild.colorroles {
		err := mod.config.Discord.GuildMemberRoleRemove(ctx.Message.GuildID, ctx.Message.Author.ID, cr)
		if err != nil {
			return fmt.Errorf("removing role: %w", err)
		}
	}

	return nil
}

func (mod *module) commandSet(ctx *router.Context) error {
	if len(ctx.Args) < 3 {
		return ErrInvalidArgumentNumber
	}

	lightnessRaw := ctx.Args.Get(1)
	hueRaw := ctx.Args.Get(2)

	lightness, err := strconv.ParseInt(lightnessRaw, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid lightness percentage: %w", err)
	}

	lightnessMin, err := mod.config.Repository.ConfigGet(ctx.Message.GuildID, "color", "lightness.min")
	if err == nil && lightnessMin != "" {
		var min int64

		if min, err = strconv.ParseInt(lightnessMin, 10, 64); err == nil {
			if lightness < min {
				return fmt.Errorf("lightness is less than minimal value: %d", min)
			}
		}
	}

	hue, err := strconv.ParseInt(hueRaw, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid hue degree: %w", err)
	}

	c := colorful.Hsl(float64(hue), 1.0, float64(lightness)/100.0)

	return mod.setcolor(ctx.Session, ctx.Message.GuildID, ctx.Message.Author.ID, c)
}

func (mod *module) commandHelp(ctx *router.Context) error {
	return ctx.ReplyEmbed("```yaml\n" + `
usage:
> color.set <lightness> <hue>

lightness:
> this is HSL last component, lightness, how close 
> resulting color is to bright white. ranged from 
> 0 to 100 in percents, but additionally limited by
> lower value to avoid dark nicks

hue:
> this is HSL first component, hue, the rotation
> on chromatic spectrum disc. ranged from 0 to
> 360 as degrees, with meaining 0 and 360 being
> red, 120 being green and 240 being blue, you
> can have any integer value in between.

> saturation is always assumed to be 100%
> meaning full chromatic value as opposed to
> black-and-white greyscale

example:
> color.set 70 340
` + "```")
}
