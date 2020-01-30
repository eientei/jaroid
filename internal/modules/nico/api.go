// Package nico provides bot module for querying nicovideo
package nico

import (
	"encoding/base64"
	"encoding/json"
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

// Used emojis
const (
	emojiOne      = "\x31\xE2\x83\xA3"
	emojiTwo      = "\x32\xE2\x83\xA3"
	emojiThree    = "\x33\xE2\x83\xA3"
	emojiFour     = "\x34\xE2\x83\xA3"
	emojiFive     = "\x35\xE2\x83\xA3"
	emojiForward  = "\xE2\x96\xB6"
	emojiBackward = "\xE2\x97\x80"
)

// New provides module instacne
func New() bot.Module {
	return &module{
		client: nicovideo.New(),
	}
}

type module struct {
	config *bot.Configuration
	client *nicovideo.Client
}

func (mod *module) Initialize(config *bot.Configuration) error {
	mod.config = config

	config.Discord.AddHandler(mod.handlerReactionAdd)

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

func (mod *module) renderSelection(session *discordgo.Session, msg *discordgo.Message, lines []string, n int) {
	idx := strings.Index(lines[n], "https://www.nicovideo.jp/watch/")
	if idx < 0 {
		return
	}

	firstparts := strings.SplitN(lines[n][idx:], " ", 3)
	if len(firstparts) != 3 {
		return
	}

	postedidx := strings.Index(lines[n+1], " ")
	posted := lines[n+1][:postedidx]
	tags := lines[n+2]

	sb := &strings.Builder{}
	_, _ = sb.WriteString(strings.TrimSuffix(firstparts[0], ">") + "\n")
	_, _ = sb.WriteString("posted: " + posted + "\n")
	_, _ = sb.WriteString("length: " + firstparts[1] + "\n")
	_, _ = sb.WriteString("tags: " + tags + "\n")
	_, _ = sb.WriteString(firstparts[2])

	_, err := session.ChannelMessageEdit(msg.ChannelID, msg.ID, sb.String())
	if err != nil {
		mod.config.Log.WithError(err).Error("Editing message", msg.ChannelID, msg.ID)
		return
	}

	err = session.MessageReactionsRemoveAll(msg.ChannelID, msg.ID)
	if err != nil {
		mod.config.Log.WithError(err).Error("Removing emojis", msg.ChannelID, msg.ID)
		return
	}
}

func (mod *module) handlerReactionAdd(session *discordgo.Session, messageReactionAdd *discordgo.MessageReactionAdd) {
	msg, err := session.ChannelMessage(messageReactionAdd.ChannelID, messageReactionAdd.MessageID)
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting message", messageReactionAdd.ChannelID, messageReactionAdd.MessageID)
		return
	}

	if msg.Author.ID != session.State.User.ID {
		return
	}

	prefix := "nico:" + messageReactionAdd.UserID + ":"
	if !strings.HasPrefix(msg.Content, prefix) {
		return
	}

	content := strings.TrimPrefix(msg.Content, prefix)

	idx := strings.Index(content, "\n")
	if idx < 0 {
		return
	}

	bs, err := base64.StdEncoding.DecodeString(content[:idx])
	if err != nil {
		mod.config.Log.WithError(err).Error("Decoding message", messageReactionAdd.ChannelID, messageReactionAdd.MessageID)
		return
	}

	search := &nicovideo.Search{}

	err = json.Unmarshal(bs, search)
	if err != nil {
		mod.config.Log.WithError(err).
			Error("Unmarshaling message", messageReactionAdd.ChannelID, messageReactionAdd.MessageID)
		return
	}

	lines := strings.Split(content[idx+1:], "\n")

	switch messageReactionAdd.Emoji.Name {
	case emojiOne, emojiTwo, emojiThree, emojiFour, emojiFive:
		n := mod.parseNumber(messageReactionAdd.Emoji.Name) * 3
		if n+2 >= len(lines) {
			return
		}

		mod.renderSelection(session, msg, lines, n)

		return
	case emojiForward:
		search.Offset += 5
	case emojiBackward:
		if search.Offset < 5 {
			return
		}

		search.Offset -= 5
	}

	content, _, err = mod.listRender(messageReactionAdd.UserID, search)
	if err != nil {
		mod.config.Log.WithError(err).Error("Rendering list", messageReactionAdd.ChannelID, messageReactionAdd.MessageID)
		return
	}

	_, err = session.ChannelMessageEdit(msg.ChannelID, msg.ID, content)
	if err != nil {
		mod.config.Log.WithError(err).Error("Editing message", messageReactionAdd.ChannelID, messageReactionAdd.MessageID)
		return
	}
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

func (mod *module) parseSearch(args router.Args, fields []nicovideo.Field, offset, limit int) (s *nicovideo.Search) {
	s = &nicovideo.Search{
		Fields: fields,
		Offset: offset,
		Limit:  limit,
	}

	s.SortDirection = nicovideo.SortDesc
	s.SortField = nicovideo.FieldViewCounter

	for _, a := range args[1:] {
		switch {
		case (strings.HasPrefix(a, "-") || strings.HasPrefix(a, "+")) && mod.isField(a[1:]):
			s.SortDirection, s.SortField = nicovideo.SortDirection(a[0]), nicovideo.Field(a[1:])
		case strings.HasPrefix(a, "%") && mod.isField(a[1:]):
			s.Targets = append(s.Targets, nicovideo.Field(a[1:]))
		case strings.HasPrefix(a, "$") && strings.Contains(a, "="):
			parts := strings.Split(a[1:], "=")
			if !mod.isField(parts[0]) {
				s.Query += " " + a
				continue
			}

			if mod.isComparableField(parts[0]) {
				switch {
				case strings.Contains(parts[1], ".."):
					s.Filters = append(s.Filters, nicovideo.Filter{
						Field:    nicovideo.Field(parts[0]),
						Operator: nicovideo.OperatorRange,
						Values:   strings.Split(parts[1], ".."),
					})

					continue
				case strings.HasPrefix(parts[1], ">"):
					s.Filters = append(s.Filters, nicovideo.Filter{
						Field:    nicovideo.Field(parts[0]),
						Operator: nicovideo.OperatorGTE,
						Values:   []string{parts[1][1:]},
					})

					continue
				case strings.HasPrefix(parts[1], "<"):
					s.Filters = append(s.Filters, nicovideo.Filter{
						Field:    nicovideo.Field(parts[0]),
						Operator: nicovideo.OperatorLTE,
						Values:   []string{parts[1][1:]},
					})

					continue
				}
			}

			s.Filters = append(s.Filters, nicovideo.Filter{
				Field:    nicovideo.Field(parts[0]),
				Operator: nicovideo.OperatorEqual,
				Values:   []string{parts[1]},
			})
		default:
			s.Query += " " + a
		}
	}

	if len(s.Targets) == 0 {
		s.Targets = []nicovideo.Field{
			nicovideo.FieldTitle,
			nicovideo.FieldTags,
			nicovideo.FieldDescription,
		}
	}

	return
}

func (mod *module) formatLength(t int) string {
	return fmt.Sprintf("%d:%02d", t/60, t%60)
}

func (mod *module) formatTags(tags []string) (s string) {
	for i, t := range tags {
		if i > 0 {
			s += " "
		}

		s += "`" + t + "`"
	}

	return
}

func (mod *module) singleRender(res *nicovideo.SearchItem) string {
	sb := &strings.Builder{}
	_, _ = sb.WriteString("https://www.nicovideo.jp/watch/" + res.ContentID)
	_, _ = sb.WriteString("\nposted: " + res.SearchItemRaw.StartTime)
	_, _ = sb.WriteString("\nlength: " + mod.formatLength(res.LengthSeconds))
	_, _ = sb.WriteString("\ntags: " + mod.formatTags(res.Tags))
	_, _ = sb.WriteString("\nviews: " + strconv.FormatInt(int64(res.ViewCounter), 10))
	_, _ = sb.WriteString(" mylists: " + strconv.FormatInt(int64(res.MylistCounter), 10))

	return sb.String()
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

	_, err = ctx.Reply(mod.singleRender(res.Data[0]))

	return err
}

func (mod *module) formatNumber(i int) string {
	switch i {
	case 0:
		return emojiOne
	case 1:
		return emojiTwo
	case 2:
		return emojiThree
	case 3:
		return emojiFour
	case 4:
		return emojiFive
	}

	return ""
}

func (mod *module) parseNumber(emoji string) int {
	switch emoji {
	case emojiOne:
		return 0
	case emojiTwo:
		return 1
	case emojiThree:
		return 2
	case emojiFour:
		return 3
	case emojiFive:
		return 4
	}

	return 0
}

func (mod *module) listRender(authorID string, search *nicovideo.Search) (
	content string,
	res *nicovideo.SearchResult,
	err error,
) {
	res, err = mod.client.Search(search)
	if err != nil {
		return "", nil, err
	}

	if len(res.Data) == 0 {
		return "", nil, ErrNothingFound
	}

	bs, err := json.Marshal(search)
	if err != nil {
		return "", nil, err
	}

	query := base64.StdEncoding.EncodeToString(bs)

	sb := &strings.Builder{}
	_, _ = sb.WriteString("nico:" + authorID + ":" + query + "\n")

	for i, v := range res.Data {
		_, _ = sb.WriteString(mod.formatNumber(i) + " <https://www.nicovideo.jp/watch/" + v.ContentID + "> ")
		_, _ = sb.WriteString(mod.formatLength(v.LengthSeconds) + " views " + strconv.FormatInt(int64(v.ViewCounter), 10))
		_, _ = sb.WriteString(" mylists " + strconv.FormatInt(int64(v.MylistCounter), 10) + "\n")
		_, _ = sb.WriteString(v.SearchItemRaw.StartTime + " " + v.Title + "\n")
		_, _ = sb.WriteString(mod.formatTags(v.Tags) + "\n")
	}

	pages := res.Meta.TotalCount / 5
	if res.Meta.TotalCount%5 != 0 {
		pages++
	}

	page := search.Offset/5 + 1

	_, _ = sb.WriteString("---\n")
	_, _ = sb.WriteString(fmt.Sprintf("Page %d of %d (%d results)", page, pages, res.Meta.TotalCount))

	return sb.String(), res, nil
}

func (mod *module) commandList(ctx *router.Context) error {
	search := mod.parseSearch(ctx.Args, []nicovideo.Field{
		nicovideo.FieldContentID,
		nicovideo.FieldTitle,
		nicovideo.FieldDescription,
		nicovideo.FieldTags,
		nicovideo.FieldStartTime,
		nicovideo.FieldLengthSeconds,
		nicovideo.FieldViewCounter,
		nicovideo.FieldMylistCounter,
	}, 0, 5)

	content, res, err := mod.listRender(ctx.Message.Author.ID, search)
	if err != nil {
		return err
	}

	msg, err := ctx.Reply(content)
	if err != nil {
		return err
	}

	for i := range res.Data {
		err = ctx.Session.MessageReactionAdd(msg.ChannelID, msg.ID, mod.formatNumber(i))
		if err != nil {
			return err
		}
	}

	err = ctx.Session.MessageReactionAdd(msg.ChannelID, msg.ID, emojiBackward)
	if err != nil {
		return err
	}

	err = ctx.Session.MessageReactionAdd(msg.ChannelID, msg.ID, emojiForward)
	if err != nil {
		return err
	}

	return err
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
