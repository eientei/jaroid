// Package color provides colored roles management
package color

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/bot"
	"github.com/eientei/jaroid/internal/router"
	"github.com/lucasb-eyer/go-colorful"
)

var (
	// ErrInvalidArgumentNumber is returned on invalid argument number
	ErrInvalidArgumentNumber = errors.New("invalid argument number, use color.help")
)

type server struct {
	roles   map[string]map[string]bool
	members map[string][]string
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

	config.Discord.AddHandler(mod.handlerMembersChunk)
	config.Discord.AddHandler(mod.handlerMemberUpdate)
	config.Discord.AddHandler(mod.handlerMemberAdd)
	config.Discord.AddHandler(mod.handlerMemberRemove)

	group := config.Router.Group("color").SetDescription("color roles")
	group.OnAlias("color.set", "sets colored role", []string{"color"}, mod.commandSet)
	group.On("color.remove", "removes colored role", mod.commandRemove)
	group.On("color.help", "provides documentation", mod.commandHelp)

	return nil
}

func (mod *module) handlerMembersChunk(session *discordgo.Session, chunk *discordgo.GuildMembersChunk) {
	if _, ok := mod.servers[chunk.GuildID]; ok {
		for _, m := range chunk.Members {
			mod.memberSync(m)
		}
	}
}

func (mod *module) memberSync(m *discordgo.Member) {
	if guild, ok := mod.servers[m.GuildID]; ok {
		for _, r := range guild.members[m.User.ID] {
			if known, ok := guild.roles[r]; ok {
				delete(known, m.User.ID)
			}
		}

		for _, r := range m.Roles {
			known, ok := guild.roles[r]
			if !ok {
				known = make(map[string]bool)
				guild.roles[r] = known
			}

			known[m.User.ID] = true
		}

		guild.members[m.User.ID] = m.Roles
	}
}

func (mod *module) handlerMemberAdd(session *discordgo.Session, m *discordgo.GuildMemberAdd) {
	mod.memberSync(m.Member)
}

func (mod *module) handlerMemberRemove(session *discordgo.Session, m *discordgo.GuildMemberRemove) {
	rolemap, err := mod.currentColorRoles(m.GuildID)
	if err != nil {
		mod.config.Log.WithError(err).Error("loading roles")
		return
	}

	if guild, ok := mod.servers[m.GuildID]; ok {
		if member, ok := guild.members[m.User.ID]; ok {
			for _, r := range member {
				if _, ok := rolemap[r]; ok {
					err := mod.deleterole(m.GuildID, m.User.ID, r)
					if err != nil {
						mod.config.Log.WithError(err).Error("deleting role", r)
					}
				}
			}
		}
	}

	m.Member.Roles = []string{}
	mod.memberSync(m.Member)
}

func (mod *module) handlerMemberUpdate(session *discordgo.Session, m *discordgo.GuildMemberUpdate) {
	mod.memberSync(m.Member)
}

func (mod *module) Configure(config *bot.Configuration, guild *discordgo.Guild) {
	if _, ok := mod.servers[guild.ID]; !ok {
		mod.servers[guild.ID] = &server{
			roles:   make(map[string]map[string]bool),
			members: make(map[string][]string),
		}

		err := config.Discord.RequestGuildMembers(guild.ID, "", 0, false)
		if err != nil {
			mod.config.Log.WithError(err).Error("requesting members", guild)
		}
	}
}

func (mod *module) Shutdown(config *bot.Configuration) {

}

func (mod *module) deleterole(guildID, userID, roleID string) (err error) {
	if guild, ok := mod.servers[guildID]; ok {
		if r, ok := guild.roles[roleID]; ok {
			delete(r, userID)

			if len(r) == 0 {
				err = mod.config.Discord.GuildRoleDelete(guildID, roleID)
				if err != nil {
					return fmt.Errorf("deleting old role: %w", err)
				}

				delete(guild.roles, roleID)
			}
		}
	}

	return nil
}

func (mod *module) createrole(guildID string, c colorful.Color) (role *discordgo.Role, err error) {
	hex := c.Hex()

	role, err = mod.config.Discord.GuildRoleCreate(guildID)
	if err != nil {
		return nil, fmt.Errorf("creating role: %w", err)
	}

	r, g, b := c.RGB255()

	v := int(r)<<16 | int(g)<<8 | int(b)

	return mod.config.Discord.GuildRoleEdit(guildID, role.ID, "color"+hex, v, false, 0, false)
}

func (mod *module) setcolor(session *discordgo.Session, guildID, userID string, c colorful.Color) error {
	rolemap, err := mod.currentColorRoles(guildID)
	if err != nil {
		return err
	}

	var role, old *discordgo.Role

	hex := c.Hex()

	for _, r := range rolemap {
		if r.Name == "color"+hex {
			role = r
		}
	}

	member, err := session.GuildMember(guildID, userID)
	if err != nil {
		return fmt.Errorf("getting member: %w", err)
	}

	for _, mr := range member.Roles {
		if mrole, ok := rolemap[mr]; ok {
			old = mrole
			break
		}
	}

	if role == nil {
		role, err = mod.createrole(guildID, c)
		if err != nil {
			return fmt.Errorf("creating role: %w", err)
		}
	}

	if old != nil {
		err = mod.config.Discord.GuildMemberRoleRemove(guildID, userID, old.ID)
		if err != nil {
			return fmt.Errorf("removing old role: %w", err)
		}

		err = mod.deleterole(guildID, userID, old.ID)
		if err != nil {
			return fmt.Errorf("deleting role: %w", err)
		}
	}

	err = mod.config.Discord.GuildMemberRoleAdd(guildID, userID, role.ID)
	if err != nil {
		return fmt.Errorf("adding new role: %w", err)
	}

	m, err := mod.config.Discord.GuildMember(guildID, userID)
	if err != nil {
		return fmt.Errorf("getting updated member: %w", err)
	}

	mod.memberSync(m)

	return nil
}

func (mod *module) currentColorRoles(guildID string) (rolemap map[string]*discordgo.Role, err error) {
	roles, err := mod.config.Discord.GuildRoles(guildID)
	if err != nil {
		return nil, fmt.Errorf("getting roles: %w", err)
	}

	rolemap = make(map[string]*discordgo.Role)

	for _, r := range roles {
		if !strings.HasPrefix(r.Name, "color") {
			continue
		}

		rolemap[r.ID] = r
	}

	return
}

func (mod *module) commandRemove(ctx *router.Context) error {
	member, err := ctx.Session.GuildMember(ctx.Message.GuildID, ctx.Message.Author.ID)
	if err != nil {
		return fmt.Errorf("getting member: %w", err)
	}

	rolemap, err := mod.currentColorRoles(ctx.Message.GuildID)
	if err != nil {
		return err
	}

	for _, mr := range member.Roles {
		if _, ok := rolemap[mr]; ok {
			err = mod.config.Discord.GuildMemberRoleRemove(ctx.Message.GuildID, ctx.Message.Author.ID, mr)
			if err != nil {
				return fmt.Errorf("removing old role: %w", err)
			}

			err = mod.deleterole(ctx.Message.GuildID, ctx.Message.Author.ID, mr)
			if err != nil {
				return fmt.Errorf("deleting role: %w", err)
			}
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
