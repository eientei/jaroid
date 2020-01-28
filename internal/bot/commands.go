package bot

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/router"
	"github.com/go-redis/redis/v7"
)

const (
	emojiOkButton = "\xf0\x9f\x86\x97"
	emojiX        = "\xe2\x9d\x8c"
)

var (
	// ErrNotAuthorized is returned when user is not authorized to perform operation
	ErrNotAuthorized = errors.New("not authorized")
)

// NewRouter returns new router instance
func (bot *Bot) newRouter() *router.Router {
	r := router.NewRouter()

	r.Middleware = append(r.Middleware, bot.middlewareReply())

	info := r.Group("info")
	info.On("help", "prints help", bot.commandInfoHelp)

	keys := r.Group("keys")
	keys.Middleware = append(keys.Middleware, bot.middlewareAuth(discordgo.PermissionAdministrator))
	keys.On("keyGet", "get internal key", bot.commandKeyGet)
	keys.On("keySet", "set internal key", bot.commandKeySet)
	keys.On("keyDel", "delete internal key", bot.commandKeyDel)
	keys.On("keyList", "list internal keys", bot.commandKeyList)

	return r
}

func (bot *Bot) middlewareReply() router.MiddlewareFunc {
	return func(handler router.HandlerFunc) router.HandlerFunc {
		return func(session *discordgo.Session, msg *discordgo.Message, route *router.Route, args router.Args) error {
			origerr := handler(session, msg, route, args)
			if origerr != nil {
				bot.options.Log.WithError(origerr).
					WithField("route", route).
					WithField("msg", msg).
					Errorf("executing command returned error")

				err := session.MessageReactionAdd(msg.ChannelID, msg.ID, emojiX)
				if err != nil {
					bot.options.Log.WithError(err).Error("Replying with error status")
					return origerr
				}

				_, err = session.ChannelMessageSendEmbed(msg.ChannelID, &discordgo.MessageEmbed{
					Description: origerr.Error(),
				})
				if err != nil {
					bot.options.Log.WithError(err).Error("Replying with error status")

					return origerr
				}

				return origerr
			}

			err := session.MessageReactionAdd(msg.ChannelID, msg.ID, emojiOkButton)
			if err != nil {
				bot.options.Log.WithError(err).Error("Replying with ok status")
			}

			return nil
		}
	}
}

func (bot *Bot) middlewareAuth(permission int) router.MiddlewareFunc {
	return func(handler router.HandlerFunc) router.HandlerFunc {
		return func(session *discordgo.Session, msg *discordgo.Message, route *router.Route, args router.Args) error {
			for _, rid := range msg.Member.Roles {
				if role, err := session.State.Role(msg.GuildID, rid); err == nil {
					if role.Permissions&permission != 0 {
						return handler(session, msg, route, args)
					}
				}
			}

			return ErrNotAuthorized
		}
	}
}

func (bot *Bot) commandKeySet(
	session *discordgo.Session,
	msg *discordgo.Message,
	route *router.Route,
	args router.Args,
) error {
	if len(args) < 3 {
		return fmt.Errorf("invalid argument number")
	}

	key := msg.GuildID + "." + args.Get(1)
	value := args.Join(2)

	err := bot.options.Client.Set(key, value, 0).Err()
	if err != nil {
		return err
	}

	return bot.reload()
}

func (bot *Bot) commandKeyGet(
	session *discordgo.Session,
	msg *discordgo.Message,
	route *router.Route,
	args router.Args,
) error {
	if len(args) < 2 {
		return fmt.Errorf("invalid argument number")
	}

	key := msg.GuildID + "." + args.Get(1)

	value, err := bot.options.Client.Get(key).Result()
	if err == redis.Nil {
		err = nil
	}

	if err != nil {
		return err
	}

	_, err = session.ChannelMessageSendEmbed(msg.ChannelID, &discordgo.MessageEmbed{
		Description: "```\n" + value + "```",
	})

	return err
}

func (bot *Bot) commandKeyDel(
	session *discordgo.Session,
	msg *discordgo.Message,
	route *router.Route,
	args router.Args,
) error {
	if len(args) < 2 {
		return fmt.Errorf("invalid argument number")
	}

	key := msg.GuildID + "." + args.Get(1)

	return bot.options.Client.Del(key).Err()
}

func (bot *Bot) commandKeyList(
	session *discordgo.Session,
	msg *discordgo.Message,
	route *router.Route,
	args router.Args,
) error {
	prefix := msg.GuildID + "."
	key := msg.GuildID + ".*"
	mask := args.Get(1)

	if mask != "" {
		key += mask
	}

	slice, err := bot.options.Client.Keys(key).Result()
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

		v, err = bot.options.Client.Get(raw).Result()
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

	_, err = session.ChannelMessageSendEmbed(msg.ChannelID, &discordgo.MessageEmbed{
		Description: buf.String(),
	})

	return err
}

func (bot *Bot) commandInfoHelp(
	session *discordgo.Session,
	msg *discordgo.Message,
	route *router.Route,
	args router.Args,
) error {
	max := 0
	for _, v := range route.Router.Routes {
		if len(v.Name) > max {
			max = len(v.Name)
		}
	}

	buf := &strings.Builder{}

	buf.WriteString("```\n")

	for _, g := range route.Router.Groups {
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

	_, err := session.ChannelMessageSendEmbed(msg.ChannelID, &discordgo.MessageEmbed{
		Description: buf.String(),
	})

	return err
}
