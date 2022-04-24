package router

import (
	"encoding/csv"
	"errors"
	"regexp"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"
)

var (
	// ErrNotMatched is returned when unknown command is issued
	ErrNotMatched = errors.New("command not matched")
)

// Router implements routing dispatch
type Router struct {
	Routes             map[string]*Route
	Groups             []*Group
	GroupSorter        GroupSorterFunc
	DefaultRouteSorter RouteSorterFunc
	Middleware         []MiddlewareFunc
}

// Dispatch tries to find matching route and execute it
func (router *Router) Dispatch(
	session *discordgo.Session,
	prefixes map[string]string,
	userID string,
	msg *discordgo.Message,
) (err error) {
	if msg.Author == nil || msg.Author.ID == userID {
		return nil
	}

	prefix := prefixes[""]

	var excludes []string

	for ex, p := range prefixes {
		if p != "" && ex != "" {
			excludes = append(excludes, ex)
		}
	}

	matched, err := router.dispatch(session, excludes, "", prefix, msg)
	if err != nil || matched {
		return
	}

	for _, g := range excludes {
		matched, err = router.dispatch(session, nil, g, prefixes[g], msg)
		if err != nil || matched {
			return
		}
	}

	return ErrNotMatched
}

func (router *Router) dispatch(
	session *discordgo.Session,
	excludegroups []string, only, prefix string,
	msg *discordgo.Message,
) (matched bool, err error) {
	raw := msg.Content
	if !strings.HasPrefix(raw, prefix) {
		return false, nil
	}

	raw = strings.TrimPrefix(raw, prefix)

	reader := csv.NewReader(strings.NewReader(raw))
	reader.Comma = ' '
	reader.TrimLeadingSpace = true

	args, err := reader.Read()
	if err != nil {
		return false, err
	}

	for _, r := range router.Routes {
		if r.Matcher(raw) {
			if checkExclude(excludegroups, only, r) {
				continue
			}

			matched = true

			if r.Baked == nil {
				var middlewares []MiddlewareFunc

				middlewares = append(middlewares, router.Middleware...)

				for _, g := range r.Groups {
					middlewares = append(middlewares, g.Middleware...)
				}

				middlewares = append(middlewares, r.Middleware...)

				r.Baked = r.Handler
				for i := len(middlewares) - 1; i >= 0; i-- {
					r.Baked = middlewares[i](r.Baked)
				}
			}

			err = r.Baked(&Context{
				Session: session,
				Message: msg,
				Route:   r,
				Args:    args,
			})

			return
		}
	}

	return
}

func checkExclude(excludegroups []string, only string, r *Route) bool {
	if only != "" {
		var matched bool

		for _, g := range r.Groups {
			if g.Name == only {
				matched = true
			}
		}

		if !matched {
			return true
		}
	}

	for _, g := range r.Groups {
		for _, exg := range excludegroups {
			if g.Name == exg {
				return true
			}
		}
	}

	return false
}

// Group returns group with given name
func (router *Router) Group(name string) (cand *Group) {
	cand = &Group{
		Name:        name,
		RouteSorter: router.DefaultRouteSorter,
		Router:      router,
		Data:        make(map[string]interface{}),
	}
	i := sort.Search(len(router.Groups), func(i int) bool {
		return router.GroupSorter(router.Groups[i], cand)
	})

	if i == len(router.Groups) || router.Groups[i].Name != name {
		router.Groups = append(router.Groups[:i], append([]*Group{cand}, router.Groups[i:]...)...)
	} else {
		cand = router.Groups[i]
	}

	return
}

// Route return route with given parameters
func (router *Router) Route(matcher MatcherFunc, name, desc string, handler HandlerFunc) (route *Route) {
	var ok bool
	if route, ok = router.Routes[name]; !ok {
		route = &Route{
			Name:        name,
			Description: desc,
			Matcher:     matcher,
			Handler:     handler,
			Middleware:  nil,
			Router:      router,
			Data:        make(map[string]interface{}),
			Replies:     make(map[string]*Reply),
		}
		router.Routes[name] = route
	}

	return
}

func nameMatcher(name string) MatcherFunc {
	return func(raw string) bool {
		parts := strings.Split(raw, " ")

		return len(parts) > 0 && parts[0] == name
	}
}

func nameAliasMatcher(name string, alias []string) MatcherFunc {
	return func(raw string) bool {
		parts := strings.Split(raw, " ")

		if len(parts) == 0 {
			return false
		}

		if parts[0] == name {
			return true
		}

		for _, a := range alias {
			if parts[0] == a {
				return true
			}
		}

		return false
	}
}

func regexMatcher(reg *regexp.Regexp) MatcherFunc {
	return reg.MatchString
}

// On creates new route in given group using name matcher
func (router *Router) On(group, name, desc string, handler HandlerFunc) (route *Route) {
	return router.Group(group).On(name, desc, handler)
}

// OnAlias creates new route in given group using alias name matcher
func (router *Router) OnAlias(group, name, desc string, alias []string, handler HandlerFunc) (route *Route) {
	return router.Group(group).OnAlias(name, desc, alias, handler)
}

// OnRegex creates new route in given group using regex matcher
func (router *Router) OnRegex(group, name, desc string, reg *regexp.Regexp, handler HandlerFunc) (route *Route) {
	return router.Group(group).OnRegex(name, desc, reg, handler)
}

// OnCustom creates new route in given group using custom matcher
func (router *Router) OnCustom(group, name, desc string, matcher MatcherFunc, handler HandlerFunc) (route *Route) {
	return router.Group(group).OnCustom(name, desc, matcher, handler)
}

// AppendMiddleware append middleware to end of the chain
func (router *Router) AppendMiddleware(middleware MiddlewareFunc) {
	router.Middleware = append(router.Middleware, middleware)
}

// PrependMiddleware append middleware to beginning of the chain
func (router *Router) PrependMiddleware(middleware MiddlewareFunc) {
	router.Middleware = append([]MiddlewareFunc{middleware}, router.Middleware...)
}
