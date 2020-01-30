// Package nico provides bot module for querying nicovideo
package nico

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/api/nicovideo"
	"github.com/eientei/jaroid/internal/bot"
	"github.com/eientei/jaroid/internal/router"
)

var (
	// ErrNothingFound is returned when no content found
	ErrNothingFound = errors.New("nothing found")
)

// New provides module instacne
func New() bot.Module {
	return &module{
		client: nicovideo.New(),
	}
}

type module struct {
	client *nicovideo.Client
}

func (mod *module) Initialize(config *bot.Configuration) error {
	group := config.Router.Group("nico").SetDescription("nicovideo API")

	group.OnAlias("nico.search", "search for video", []string{"nico"}, mod.commandSearch)
	group.On("nico.list", "search videos list", mod.commandList)
	group.On("nico.help", "prints help on search terms", mod.commandHelp)

	return nil
}

func (mod *module) Configure(config *bot.Configuration, guild *discordgo.Guild) {

}

func (mod *module) Shutdown(config *bot.Configuration) {

}

func (mod *module) isField(s string) bool {
	switch nicovideo.Field(s) {
	case nicovideo.FieldContentID,
		nicovideo.FieldTitle,
		nicovideo.FieldDescription,
		nicovideo.FieldUserID,
		nicovideo.FieldViewCounter,
		nicovideo.FieldMylistCounter,
		nicovideo.FieldLengthSeconds,
		nicovideo.FieldThumbnailURL,
		nicovideo.FieldStartTime,
		nicovideo.FieldThreadID,
		nicovideo.FieldCommentCounter,
		nicovideo.FieldLastCommentTime,
		nicovideo.FieldCategoryTags,
		nicovideo.FieldChannelID,
		nicovideo.FieldTags,
		nicovideo.FieldTagsExact,
		nicovideo.FieldLockTagsExact,
		nicovideo.FieldGenre,
		nicovideo.FieldGenreKeyword:
		return true
	}

	return false
}

func (mod *module) isComparableField(s string) bool {
	switch nicovideo.Field(s) {
	case nicovideo.FieldViewCounter,
		nicovideo.FieldMylistCounter,
		nicovideo.FieldLengthSeconds,
		nicovideo.FieldStartTime,
		nicovideo.FieldCommentCounter,
		nicovideo.FieldLastCommentTime:
		return true
	}

	return false
}

func (mod *module) parseSearch(args router.Args, fields []nicovideo.Field, offset, limit int) (
	q string,
	sort nicovideo.Field,
	dir nicovideo.SortDirection,
	targets []nicovideo.Field,
	search *nicovideo.Search,
) {
	search = &nicovideo.Search{
		Fields: fields,
		Offset: offset,
		Limit:  limit,
	}

	dir = nicovideo.SortDesc
	sort = nicovideo.FieldViewCounter

	for _, a := range args[1:] {
		switch {
		case (strings.HasPrefix(a, "-") || strings.HasPrefix(a, "+")) && mod.isField(a[1:]):
			dir, sort = nicovideo.SortDirection(a[0]), nicovideo.Field(a[1:])
		case strings.HasPrefix(a, "%") && mod.isField(a[1:]):
			targets = append(targets, nicovideo.Field(a[1:]))
		case strings.HasPrefix(a, "$") && strings.Contains(a, "="):
			parts := strings.Split(a[1:], "=")
			if !mod.isField(parts[0]) {
				q += " " + a
				continue
			}

			if mod.isComparableField(parts[0]) {
				switch {
				case strings.Contains(parts[1], ".."):
					search.Filters = append(search.Filters, nicovideo.Filter{
						Field:    nicovideo.Field(parts[0]),
						Operator: nicovideo.OperatorRange,
						Values:   strings.Split(parts[1], ".."),
					})

					continue
				case strings.HasPrefix(parts[1], ">"):
					search.Filters = append(search.Filters, nicovideo.Filter{
						Field:    nicovideo.Field(parts[0]),
						Operator: nicovideo.OperatorGTE,
						Values:   []string{parts[1][1:]},
					})

					continue
				case strings.HasPrefix(parts[1], "<"):
					search.Filters = append(search.Filters, nicovideo.Filter{
						Field:    nicovideo.Field(parts[0]),
						Operator: nicovideo.OperatorLTE,
						Values:   []string{parts[1][1:]},
					})

					continue
				}
			}

			search.Filters = append(search.Filters, nicovideo.Filter{
				Field:    nicovideo.Field(parts[0]),
				Operator: nicovideo.OperatorEqual,
				Values:   []string{parts[1]},
			})
		default:
			q += " " + a
		}
	}

	if len(targets) == 0 {
		targets = []nicovideo.Field{
			nicovideo.FieldTitle,
			nicovideo.FieldTags,
			nicovideo.FieldDescription,
		}
	}

	return
}

func (mod *module) commandSearch(ctx *router.Context) error {
	res, err := mod.client.Search(mod.parseSearch(ctx.Args, []nicovideo.Field{
		nicovideo.FieldContentID,
		nicovideo.FieldTags,
		nicovideo.FieldStartTime,
		nicovideo.FieldLengthSeconds,
		nicovideo.FieldViewCounter,
		nicovideo.FieldMylistCounter,
	}, 0, 1))
	if err != nil {
		return err
	}

	if len(res.Data) == 0 {
		return ErrNothingFound
	}

	fmt.Println(res.Data[0].ThumbnailURL)

	sb := &strings.Builder{}
	_, _ = sb.WriteString("https://www.nicovideo.jp/watch/" + res.Data[0].ContentID)
	_, _ = sb.WriteString("\nposted: " + res.Data[0].SearchItemRaw.StartTime)
	_, _ = sb.WriteString("\nlength: ")
	_, _ = sb.WriteString(fmt.Sprintf("%d:%02d", res.Data[0].LengthSeconds/60, res.Data[0].LengthSeconds%60))
	_, _ = sb.WriteString("\ntags: ")

	for i, t := range res.Data[0].Tags {
		if i > 0 {
			_, _ = sb.WriteString(" ")
		}

		_, _ = sb.WriteString("`" + t + "`")
	}

	_, _ = sb.WriteString("\nviews: " + strconv.FormatInt(int64(res.Data[0].ViewCounter), 10))
	_, _ = sb.WriteString(" mylists: " + strconv.FormatInt(int64(res.Data[0].MylistCounter), 10))

	return ctx.Reply(sb.String())
}

func (mod *module) commandList(ctx *router.Context) error {
	return nil
}

func (mod *module) commandHelp(ctx *router.Context) error {
	return ctx.ReplyEmbed("```yaml\n" + `
# fields can be used in filters, sorts and targets
fields:
- contentId                           # content string ID
- channelId                           # channel numeric ID
- userId                              # user numeric ID
- title                               # video title
- tags                                # video tags
- genre                               # video genre
- startTime                           # publishing time
- lastCommentTime                     # last comment time
- description                         # description
- viewCounter                         # number of views
- mylistCounter                       # number of mylists
- commentCounter                      # number of comments

# filters can be used with fields
filters:
- $tags=value1                         # equal
- $tags=value1 $tags=value2            # equal to either of
- $mylistCounter=1000                  # equal
- $mylistCounter=>1000                 # greater or equal
- $mylistCounter=<1000                 # less or equal
- $mylistCounter=1000..2000            # range
- $startTime=2020-01-30                # equal to date
- $startTime=2019-01-01..2020-01-01    # time/date in range 
- $startTime=2020-01-30T17:49:51+09:00 # timezone

# targets can be used with fields for freestanding query
targets:
# search by title
- %title
# search by tags
- %tags

# sorts can be used with fields with + (asc) or - (desc)
sorts:>
> +mylistCounter
> -mylistCounter

default:
# used if none of sorts or targets are spceified
> %title %tags %description -viewCounter

example:
# search "cookie" at title only
> cookie %title

example:
# search "cookie" at default
> cookie

example:
# search for cirno at default
# also filtering by tag baka 
# and sort by descending time
> cirno $tags=baka -startTime

example:
# search for "cookie" at default
# having view count > 100
# and sort by descending view count
> cookie $viewCounter=>100 -viewCounter
` + "```")
}
