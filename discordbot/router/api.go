// Package router provides command router
package router

import (
	"strings"

	"github.com/bwmarrin/discordgo"
)

// Args provide abstraction for getting arguments
type Args []string

// Get returns bound-safe argument by index
func (args Args) Get(i int) string {
	if len(args) <= i {
		return ""
	}

	return args[i]
}

// Join joins arguments starting with given index
func (args Args) Join(i int) string {
	return strings.Join(args[i:], " ")
}

// GroupSorterFunc provides sorting for groups
type GroupSorterFunc func(a, b *Group) bool

// RouteSorterFunc provides sorting for routes
type RouteSorterFunc func(a, b *Route) bool

// MatcherFunc implements matching message
type MatcherFunc func(raw string) bool

// MiddlewareFunc implements command wrapping
type MiddlewareFunc func(handler HandlerFunc) HandlerFunc

// HandlerFunc implements command execution
type HandlerFunc func(ctx *Context) error

// Context simplifies request handling
type Context struct {
	Session *discordgo.Session
	Message *discordgo.Message
	Route   *Route
	Args    Args
}

// Reply keeps track of user requests and bot replies
type Reply struct {
	Request  *discordgo.Message
	Response *discordgo.Message
}

// React reacts to original message with emoji
func (ctx *Context) React(emoji string) (err error) {
	err = ctx.Session.MessageReactionAdd(ctx.Message.ChannelID, ctx.Message.ID, emoji)

	return
}

// ReplyEmbed replies to original message with embed
func (ctx *Context) ReplyEmbed(desc string) (err error) {
	var msg *discordgo.Message
	msg, err = ctx.Session.ChannelMessageSendEmbed(ctx.Message.ChannelID, &discordgo.MessageEmbed{
		Description: desc,
	})

	if err != nil {
		return
	}

	ctx.Route.Replies[msg.ID] = &Reply{
		Request:  ctx.Message,
		Response: msg,
	}

	return
}

// ReplyEmbedCustom replies to original message with custom embed
func (ctx *Context) ReplyEmbedCustom(embed *discordgo.MessageEmbed) (err error) {
	var msg *discordgo.Message
	msg, err = ctx.Session.ChannelMessageSendEmbed(ctx.Message.ChannelID, embed)

	if err != nil {
		return
	}

	ctx.Route.Replies[msg.ID] = &Reply{
		Request:  ctx.Message,
		Response: msg,
	}

	return
}

// Reply replies to original message
func (ctx *Context) Reply(desc string) (msg *discordgo.Message, err error) {
	msg, err = ctx.Session.ChannelMessageSend(ctx.Message.ChannelID, desc)

	if err != nil {
		return
	}

	ctx.Route.Replies[msg.ID] = &Reply{
		Request:  ctx.Message,
		Response: msg,
	}

	return
}

// NewRouter returns new router instance
func NewRouter() *Router {
	return &Router{
		Routes: make(map[string]*Route),
		GroupSorter: func(a, b *Group) bool {
			return a.Name >= b.Name
		},
		DefaultRouteSorter: func(a, b *Route) bool {
			return a.Name >= b.Name
		},
	}
}

// Route describes command route
type Route struct {
	Router      *Router
	Name        string
	Description string
	Matcher     MatcherFunc
	Handler     HandlerFunc
	Baked       HandlerFunc
	Data        map[string]interface{}
	Replies     map[string]*Reply
	Middleware  []MiddlewareFunc
	Groups      []*Group
	Alias       []string
	AliasHelp   bool
}

// Set sets route config value
func (route *Route) Set(k string, v interface{}) *Route {
	route.Data[k] = v

	return route
}

// Get returns route (or any of parent groups) config value
func (route *Route) Get(k string) interface{} {
	if v, ok := route.Data[k]; ok {
		return v
	}

	for _, g := range route.Groups {
		if v, ok := g.Data[k]; ok {
			return v
		}
	}

	return nil
}
