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
type HandlerFunc func(session *discordgo.Session, msg *discordgo.Message, route *Route, args Args) error

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
	Name        string
	Description string
	Matcher     MatcherFunc
	Handler     HandlerFunc
	Baked       HandlerFunc
	Middleware  []MiddlewareFunc
	Groups      []*Group
	Router      *Router
	Data        map[string]interface{}
}
