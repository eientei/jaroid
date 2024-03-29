package router

import (
	"regexp"
	"sort"
)

// Group groups a number of routes
type Group struct {
	Router      *Router
	Name        string
	Description string
	Data        map[string]interface{}
	RouteSorter RouteSorterFunc
	Routes      []*Route
	Middleware  []MiddlewareFunc
}

// SetDescription sets description for group
func (group *Group) SetDescription(description string) *Group {
	group.Description = description
	return group
}

// On adds route to group using name matcher
func (group *Group) On(name, desc string, handler HandlerFunc) (route *Route) {
	route = group.Router.Route(nameMatcher(name), name, desc, handler)

	group.AddRoute(route)

	return route
}

// OnAlias adds route to group using alias name matcher
func (group *Group) OnAlias(name, desc string, alias []string, help bool, handler HandlerFunc) (route *Route) {
	route = group.Router.Route(nameAliasMatcher(name, alias), name, desc, handler)
	route.Alias = alias
	route.AliasHelp = help

	group.AddRoute(route)

	return route
}

// OnRegex adds route to group using regex matcher
func (group *Group) OnRegex(name, desc string, reg *regexp.Regexp, handler HandlerFunc) (route *Route) {
	route = group.Router.Route(regexMatcher(reg), name, desc, handler)

	group.AddRoute(route)

	return route
}

// OnCustom adds route to group using custom matcher
func (group *Group) OnCustom(name, desc string, matcher MatcherFunc, handler HandlerFunc) (route *Route) {
	route = group.Router.Route(matcher, name, desc, handler)

	group.AddRoute(route)

	return route
}

// AddRoute appends route maintaining sorting order
func (group *Group) AddRoute(route *Route) {
	i := sort.Search(len(group.Routes), func(i int) bool {
		return group.RouteSorter(group.Routes[i], route)
	})
	if i == len(group.Routes) || group.Routes[i].Name != route.Name {
		group.Routes = append(group.Routes[:i], append([]*Route{route}, group.Routes[i:]...)...)
		route.Groups = append(route.Groups, group)
	}
}

// Set sets group data entry
func (group *Group) Set(k string, v interface{}) *Group {
	group.Data[k] = v
	return group
}

// Get returns group data entry
func (group *Group) Get(k string) interface{} {
	return group.Data[k]
}
