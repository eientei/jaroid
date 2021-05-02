// Package nico provides bot module for querying nicovideo
package nico

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/api/nicovideo"
	"github.com/eientei/jaroid/internal/bot"
	"github.com/eientei/jaroid/internal/modules/auth"
	"github.com/eientei/jaroid/internal/router"
)

var (
	// ErrNothingFound is returned when no content found
	ErrNothingFound = errors.New("nothing found")
	// ErrInvalidArgumentNumber is returned on invalid argument number
	ErrInvalidArgumentNumber = errors.New("invalid argument number")
	// ErrInvalidURL is returned when invalid url submitted to download
	ErrInvalidURL = errors.New("invalid url")
	// ErrInvalidFormat inidactes invalid video format
	ErrInvalidFormat = errors.New("invalid format id")
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
		client:  nicovideo.New(),
		servers: make(map[string]bool),
	}
}

type module struct {
	config  *bot.Configuration
	client  *nicovideo.Client
	servers map[string]bool
}

func (mod *module) Initialize(config *bot.Configuration) error {
	mod.config = config

	config.Discord.AddHandler(mod.handlerReactionAdd)

	group := config.Router.Group("nico").SetDescription("nicovideo API")

	group.OnAlias("nico.search", "search for video", []string{"nico"}, mod.commandSearch)
	group.On("nico.list", "search videos list", mod.commandList)
	group.On("nico.feed", "start nico feed", mod.commandFeed).Set(auth.RouteConfigKey, &auth.RouteConfig{
		Permissions: discordgo.PermissionAdministrator,
	})
	group.On("nico.download", "download video", mod.commandDownload)
	group.On("nico.help", "prints nico help", mod.commandHelp)

	go mod.backgroundFeed()
	go mod.startDownload()
	go mod.startList()
	go mod.startCleanup()

	return nil
}

func (mod *module) Configure(config *bot.Configuration, guild *discordgo.Guild) {
	mod.servers[guild.ID] = true
}

func (mod *module) Shutdown(config *bot.Configuration) {

}

func (mod *module) backgroundServer(s, name string) {
	feed := &feed{}

	t, err := mod.config.Repository.ConfigGet(s, "nico", name)
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting nico feed key", s, name)
		return
	}

	if t == "" {
		mod.config.Log.WithError(err).Error("Empty nico feed key", s, name)
		return
	}

	err = json.Unmarshal([]byte(t), feed)
	if err != nil {
		mod.config.Log.WithError(err).Error("Unmarshaling nico feed key", s, name)
		return
	}

	err = mod.executeFeed(feed)
	if err != nil {
		mod.config.Log.WithError(err).Error("Executing nico feed key", s, name)
		return
	}

	bs, err := json.Marshal(feed)
	if err != nil {
		mod.config.Log.WithError(err).Error("Marshaling nico feed key", s, name)
		return
	}

	err = mod.config.Repository.ConfigSet(s, "nico", name, string(bs))
	if err != nil {
		mod.config.Log.WithError(err).Error("Setting nico feed key", s, name)
		return
	}
}

func (mod *module) backgroundFeed() {
	for {
		for s := range mod.servers {
			prefix := s + ".nico."

			rs, err := mod.config.Client.Keys(prefix + "*").Result()
			if err != nil {
				continue
			}

			for _, name := range rs {
				name = strings.TrimPrefix(name, prefix)
				mod.backgroundServer(s, name)
			}
		}

		time.Sleep(time.Minute)
	}
}

type feed struct {
	ChannelID string             `json:"channel_id"`
	Query     string             `json:"query"`
	Executed  time.Time          `json:"executed"`
	Last      time.Time          `json:"last"`
	Targets   []nicovideo.Field  `json:"targets"`
	Filters   []nicovideo.Filter `json:"filters"`
	Period    time.Duration      `json:"period"`
}

func (mod *module) executeFeedSearch(feed *feed) (search *nicovideo.Search) {
	search = &nicovideo.Search{}

	search.Query = feed.Query
	search.Targets = feed.Targets
	search.Fields = []nicovideo.Field{
		nicovideo.FieldContentID,
		nicovideo.FieldTags,
		nicovideo.FieldStartTime,
		nicovideo.FieldLengthSeconds,
		nicovideo.FieldViewCounter,
		nicovideo.FieldMylistCounter,
	}
	search.Filters = feed.Filters
	search.SortField = nicovideo.FieldStartTime
	search.SortDirection = nicovideo.SortDesc
	search.Limit = mod.config.Config.Private.Nicovideo.Limit

	if !feed.Last.IsZero() {
		search.Filters = append(search.Filters, nicovideo.Filter{
			Field:    nicovideo.FieldStartTime,
			Operator: nicovideo.OperatorGTE,
			Values:   []string{feed.Last.Add(time.Second).Format(time.RFC3339)},
		})
	}

	return
}

func (mod *module) executeFeed(feed *feed) error {
	if time.Since(feed.Executed) < feed.Period {
		return nil
	}

	nicobackoff, _ := mod.config.Client.Get("nico_backoff").Result()
	backoff, _ := time.ParseDuration(nicobackoff)

	nicobacked, _ := mod.config.Client.Get("nico_backed").Result()
	backed, _ := time.Parse(time.RFC3339, nicobacked)

	if time.Since(backed) < backoff {
		mod.config.Log.WithFields(logrus.Fields{
			"backoff": backoff,
			"until":   backed.Add(backoff),
			"feed":    *feed,
		}).Warn("awaiting backoff")

		return nil
	}

	res, err := mod.client.Search(mod.executeFeedSearch(feed))
	if err != nil {
		backed = time.Now()

		if backoff == 0 {
			backoff = mod.config.Config.Private.Nicovideo.Backoff
		} else {
			backoff <<= 1
		}

		mod.config.Log.WithFields(logrus.Fields{
			"backoff": backoff,
			"feed":    *feed,
		}).Error("backing off")

		_ = mod.config.Client.Set("nico_backoff", backoff.String(), 0)
		_ = mod.config.Client.Set("nico_backed", backed.Format(time.RFC3339), 0)

		return err
	}

	for i := len(res.Data) - 1; i >= 0; i-- {
		r := res.Data[i]

		_, err = mod.config.Discord.ChannelMessageSend(feed.ChannelID, mod.singleRender(r))
		if err != nil {
			return err
		}

		feed.Last = r.StartTime
	}

	if feed.Last.IsZero() {
		feed.Last = time.Now()
	}

	feed.Executed = time.Now()

	_ = mod.config.Client.Del("nico_backoff")
	_ = mod.config.Client.Del("nico_backed")

	return nil
}

func (mod *module) commandDownload(ctx *router.Context) error {
	if len(ctx.Args) < 2 {
		return ErrInvalidArgumentNumber
	}

	urlraw := ctx.Args[1]

	u, err := url.Parse(urlraw)
	if err != nil {
		return err
	}

	switch u.Hostname() {
	case "www.nicovideo.jp", "nicovideo.jp":
		var m bool

		if m, err = regexp.MatchString("^.*/sm[0-9]*$", urlraw); err != nil || !m {
			return ErrInvalidURL
		}
	default:
		return ErrInvalidURL
	}

	msg, err := ctx.Reply("Starting download...")
	if err != nil {
		return err
	}

	var format string

	if len(ctx.Args) > 2 {
		format = strings.TrimSpace(ctx.Args[2])
	}

	if format == "list" {
		return mod.config.Repository.TaskEnqueue(&TaskList{
			GuildID:   msg.GuildID,
			ChannelID: msg.ChannelID,
			MessageID: msg.ID,
			VideoURL:  urlraw,
		}, 0, 0)
	}

	if humanFileSizeExtractor.MatchString(format) || strings.ToLower(format) == "inf" {
		target := float64(findHumanSize(format))

		if strings.ToLower(format) == "inf" {
			target = math.MaxFloat64
		}

		return mod.config.Repository.TaskEnqueue(&TaskList{
			GuildID:   msg.GuildID,
			ChannelID: msg.ChannelID,
			MessageID: msg.ID,
			VideoURL:  urlraw,
			Target:    target,
		}, 0, 0)
	}

	return mod.config.Repository.TaskEnqueue(&TaskDownload{
		GuildID:   msg.GuildID,
		ChannelID: msg.ChannelID,
		MessageID: msg.ID,
		VideoURL:  urlraw,
		Format:    format,
	}, 0, 0)
}

func (mod *module) commandFeed(ctx *router.Context) error {
	if len(ctx.Args) < 4 {
		return ErrInvalidArgumentNumber
	}

	name, period, channelID := ctx.Args[1], ctx.Args[2], ctx.Args[3]
	ctx.Args = ctx.Args[3:]

	channel, err := ctx.Session.Channel(channelID)
	if err != nil {
		return err
	}

	search := mod.parseSearch(ctx.Args, []nicovideo.Field{}, 0, 20)

	t, err := mod.config.Repository.ConfigGet(ctx.Message.GuildID, "nico", name)
	if err != nil {
		return err
	}

	feed := &feed{}

	if t != "" {
		err = json.Unmarshal([]byte(t), feed)
		if err != nil {
			return err
		}
	}

	feed.ChannelID = channel.ID
	feed.Targets = search.Targets
	feed.Query = search.Query
	feed.Filters = search.Filters

	feed.Period, err = time.ParseDuration(period)
	if err != nil {
		return err
	}

	err = mod.executeFeed(feed)
	if err != nil {
		return err
	}

	bs, err := json.Marshal(feed)
	if err != nil {
		return err
	}

	return mod.config.Repository.ConfigSet(ctx.Message.GuildID, "nico", name, string(bs))
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
		n := parseNumber(messageReactionAdd.Emoji.Name) * 3
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

func (mod *module) singleRender(res *nicovideo.SearchItem) string {
	sb := &strings.Builder{}
	_, _ = sb.WriteString("https://www.nicovideo.jp/watch/" + res.ContentID)
	_, _ = sb.WriteString("\nposted: " + res.SearchItemRaw.StartTime)
	_, _ = sb.WriteString("\nlength: " + formatLength(res.LengthSeconds))
	_, _ = sb.WriteString("\ntags: " + formatTags(res.Tags))
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
		_, _ = sb.WriteString(formatNumber(i) + " <https://www.nicovideo.jp/watch/" + v.ContentID + "> ")
		_, _ = sb.WriteString(formatLength(v.LengthSeconds) + " views " + strconv.FormatInt(int64(v.ViewCounter), 10))
		_, _ = sb.WriteString(" mylists " + strconv.FormatInt(int64(v.MylistCounter), 10) + "\n")
		_, _ = sb.WriteString(v.SearchItemRaw.StartTime + " " + v.Title + "\n")
		_, _ = sb.WriteString(formatTags(v.Tags) + "\n")
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
		err = ctx.Session.MessageReactionAdd(msg.ChannelID, msg.ID, formatNumber(i))
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

const nicoCommandHelp = "```yaml\n" + `
>>> nico.download <url> [format code | list] 
>>> nico.download <url> [size | inf] 

Download a video from niconico, in given format
(if specified), or list available formats.

example:
# download video with default format
> nico.download https://www.nicovideo.jp/watch/sm00

example:
# list video formats
> nico.download https://www.nicovideo.jp/watch/sm00 list

example:
# download video with format code f1
> nico.download https://www.nicovideo.jp/watch/sm00 f1

example:
# download video with maximum est. size less than 50 MB 
> nico.download https://www.nicovideo.jp/watch/sm00 50M

example:
# download video with maximum est.size
> nico.download https://www.nicovideo.jp/watch/sm00 inf
` + "```"

const nicoFilterHelp = "```yaml\n" + `
>>> nico.search <filters>

Search for videos using given filters and sortings

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
- $tags=value1                        # equal
- $tags=value1 $tags=value2           # equal to either of
- $mylistCounter=1000                 # equal
- $mylistCounter=>1000                # greater or equal
- $mylistCounter=<1000                # less or equal
- $mylistCounter=1000..2000           # range
- $startTime=2020-01-30               # equal to date
- $startTime=2019-01-01..2020-01-01   # time/date in range 
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
` + "```"

func (mod *module) commandHelp(ctx *router.Context) error {
	err := ctx.ReplyEmbed(nicoCommandHelp)
	if err != nil {
		return err
	}

	return ctx.ReplyEmbed(nicoFilterHelp)
}
