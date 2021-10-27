// Package config provides bot module for managing per-server configuration
package config

import (
	"errors"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/discordbot/bot"
	"github.com/eientei/jaroid/discordbot/modules/auth"
	"github.com/eientei/jaroid/discordbot/router"
	redis "github.com/go-redis/redis/v7"
)

var (
	// ErrInvalidArgumentNumber is retuned when invalid number of arguments is supplied
	ErrInvalidArgumentNumber = errors.New("invalid argument number")
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

	group := config.Router.Group("config").SetDescription("internal configuration")
	group.Set(auth.RouteConfigKey, &auth.RouteConfig{
		Permissions: discordgo.PermissionAdministrator,
	})

	group.On("config.get", "gets config value", mod.configGet)
	group.On("config.set", "sets config value", mod.configSet)
	group.On("config.del", "deletes config value", mod.configDel)
	group.On("config.list", "lists config values", mod.configList)
	group.On("config.tasks", "lists task stats", mod.configTasks)

	return nil
}

func (mod *module) Configure(config *bot.Configuration, guild *discordgo.Guild) {

}

func (mod *module) Shutdown(config *bot.Configuration) {

}

func (mod *module) configGet(ctx *router.Context) error {
	if len(ctx.Args) < 2 {
		return ErrInvalidArgumentNumber
	}

	key := ctx.Message.GuildID + "." + ctx.Args.Get(1)

	value, err := mod.config.Client.Get(key).Result()
	if err == redis.Nil {
		err = nil
	}

	if err != nil {
		return err
	}

	return ctx.ReplyEmbed("```\n" + value + "```")
}

func (mod *module) configSet(ctx *router.Context) error {
	if len(ctx.Args) < 3 {
		return ErrInvalidArgumentNumber
	}

	key := ctx.Message.GuildID + "." + ctx.Args.Get(1)
	value := ctx.Args.Join(2)

	err := mod.config.Client.Set(key, value, 0).Err()
	if err != nil {
		return err
	}

	mod.config.Reload()

	return nil
}

func (mod *module) configDel(ctx *router.Context) error {
	if len(ctx.Args) < 2 {
		return ErrInvalidArgumentNumber
	}

	key := ctx.Message.GuildID + "." + ctx.Args.Get(1)

	return mod.config.Client.Del(key).Err()
}

func (mod *module) configList(ctx *router.Context) error {
	prefix := ctx.Message.GuildID + "."
	key := ctx.Message.GuildID + ".*"
	mask := ctx.Args.Get(1)

	if mask != "" {
		key += mask
	}

	slice, err := mod.config.Client.Keys(key).Result()
	if err != nil {
		return err
	}

	max := 0

	for _, s := range slice {
		if !strings.HasPrefix(s, prefix) {
			continue
		}

		s = strings.TrimPrefix(s, prefix)
		l := len(s)

		if l > max {
			max = l
		}
	}

	buf := &strings.Builder{}

	buf.WriteString("```\n")

	for _, s := range slice {
		raw := s

		if !strings.HasPrefix(s, prefix) {
			continue
		}

		s = strings.TrimPrefix(s, prefix)
		_, _ = buf.WriteString(strings.Repeat(" ", max-len(s)))
		_, _ = buf.WriteString(s)
		_, _ = buf.WriteString(": ")

		var v string

		v, err = mod.config.Client.Get(raw).Result()
		if err == redis.Nil {
			err = nil
		}

		if err != nil {
			return err
		}

		_, _ = buf.WriteString(v)
		_, _ = buf.WriteString("\n")
	}

	buf.WriteString("```")

	return ctx.ReplyEmbed(buf.String())
}

func (mod *module) configTasks(ctx *router.Context) error {
	slice, err := mod.config.Client.Keys("task.*").Result()
	if err != nil {
		return err
	}

	max := 0

	for _, s := range slice {
		if len(s) > max {
			max = len(s)
		}
	}

	buf := &strings.Builder{}

	buf.WriteString("```\n")

	for _, s := range slice {
		_, _ = buf.WriteString(strings.Repeat(" ", max-len(s)))
		_, _ = buf.WriteString(s)
		_, _ = buf.WriteString(": ")

		var v int64

		v, err = mod.config.Client.XLen(s).Result()
		if err == redis.Nil {
			err = nil
		}

		if err != nil {
			return err
		}

		_, _ = buf.WriteString(strconv.FormatInt(v, 10))
		_, _ = buf.WriteString("\n")
	}

	buf.WriteString("```")

	return ctx.ReplyEmbed(buf.String())
}
