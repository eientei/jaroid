// Package pin provides bot module to pin/unpin messages
package pin

import (
	"errors"
	"sync"
	"time"

	"github.com/eientei/jaroid/discordbot/bot"
	"github.com/eientei/jaroid/discordbot/router"

	"github.com/bwmarrin/discordgo"
)

var (
	// ErrInvalidArgumentNumber is returned on invalid argument number
	ErrInvalidArgumentNumber = errors.New("invalid argument number")
	// ErrInvalidMessageID is returned on invalid message id
	ErrInvalidMessageID = errors.New("invalid message id")
	// ErrAlreadyPinned is returned on pinning already pinned message
	ErrAlreadyPinned = errors.New("already pinned")
	// ErrNotPinned is returned on unpinning not pinned message
	ErrNotPinned = errors.New("not  pinned")
	// ErrNotAllowed is returned when calling user do not have emoji editing role
	ErrNotAllowed = errors.New("not allowed")
	// ErrTooEarly is returned when unpin is attempted too early after pin
	ErrTooEarly = errors.New("too early")
	// ErrTooFrequent is returned when unpin is attempted too frequently
	ErrTooFrequent = errors.New("too frequent")
)

// New provides module instance
func New() bot.Module {
	return &module{
		mutex: &sync.Mutex{},
	}
}

type module struct {
	pins   map[string][]*discordgo.Message
	mutex  *sync.Mutex
	config *bot.Configuration
}

func (mod *module) Initialize(config *bot.Configuration) error {
	mod.config = config
	mod.pins = make(map[string][]*discordgo.Message)

	config.Discord.AddHandler(mod.handlerChannelPinsUpdate)

	group := config.Router.Group("pins").SetDescription("manage pins")

	group.OnAlias("pin", "pin message by ID", []string{"anus"}, false, mod.commandPin)
	group.OnAlias("unpin", "unpin message by ID", []string{"deanus", "unanus"}, false, mod.commandUnpin)

	return nil
}

func (mod *module) Configure(config *bot.Configuration, guild *discordgo.Guild) {
	prefix, err := config.Repository.ConfigGet(guild.ID, "pin", "prefix")
	if err != nil {
		config.Log.WithError(err).Error("Getting pin prefix", guild.ID)

		return
	}

	if prefix != "" {
		config.SetPrefix(guild.ID, "pin", prefix)
	}
}

func (mod *module) Shutdown(config *bot.Configuration) {

}

func (mod *module) currentPins(channelID string) ([]*discordgo.Message, error) {
	mod.mutex.Lock()
	defer mod.mutex.Unlock()

	pins, ok := mod.pins[channelID]
	if ok {
		return pins, nil
	}

	msgs, err := mod.config.Discord.ChannelMessagesPinned(channelID)
	if err != nil {
		return nil, err
	}

	mod.pins[channelID] = msgs

	return msgs, nil
}

func (mod *module) handlerChannelPinsUpdate(
	session *discordgo.Session,
	channelPinsUpdate *discordgo.ChannelPinsUpdate,
) {
	mod.mutex.Lock()
	defer mod.mutex.Unlock()

	delete(mod.pins, channelPinsUpdate.ChannelID)
}

func (mod *module) checkPermissions(ctx *router.Context) (bool, error) {
	role, err := mod.config.Repository.ConfigGet(ctx.Message.GuildID, "pins", "role")
	if err != nil {
		return false, err
	}

	if role == "" {
		return false, nil
	}

	for _, r := range ctx.Message.Member.Roles {
		if r == role {
			return true, nil
		}
	}

	return false, nil
}

func (mod *module) commandPin(ctx *router.Context) error {
	ok, err := mod.checkPermissions(ctx)
	if err != nil {
		return err
	}

	if !ok {
		return ErrNotAllowed
	}

	key := ctx.Message.GuildID + "." + ctx.Message.Author.ID + ".pin"

	v := mod.config.Client.Incr(key).Val()
	if v == 1 {
		mod.config.Client.Expire(key, time.Hour)
	}

	if v > 1 {
		switch {
		case v < 5:
			return ErrTooFrequent
		case v < 10:
			return errors.New("still too frequent")
		case v < 20:
			return errors.New("занякал")
		case v < 50:
			return errors.New("в игнор ребёнка")
		}
	}

	var messageID string

	switch {
	case ctx.Message.MessageReference != nil:
		messageID = ctx.Message.MessageReference.MessageID
	case len(ctx.Args) > 1:
		messageID = ctx.Args[1]
	default:
		return ErrInvalidArgumentNumber
	}

	msg, err := ctx.Session.ChannelMessage(ctx.Message.ChannelID, messageID)
	if err != nil {
		return ErrInvalidMessageID
	}

	msgs, err := mod.currentPins(ctx.Message.ChannelID)
	if err != nil {
		return err
	}

	for _, m := range msgs {
		if m.ID == messageID {
			return ErrAlreadyPinned
		}
	}

	mod.config.Client.Set(msg.GuildID+"."+messageID+".pinned", msg.Author.ID, time.Hour*24*7)

	return ctx.Session.ChannelMessagePin(ctx.Message.ChannelID, messageID)
}

func (mod *module) commandUnpin(ctx *router.Context) error {
	ok, err := mod.checkPermissions(ctx)
	if err != nil {
		return err
	}

	if !ok {
		return ErrNotAllowed
	}

	key := ctx.Message.GuildID + "." + ctx.Message.Author.ID + ".unpin"

	v := mod.config.Client.Incr(key).Val()
	if v == 1 {
		mod.config.Client.Expire(key, time.Hour)
	}

	if v > 1 {
		switch {
		case v < 5:
			return ErrTooFrequent
		case v < 10:
			return errors.New("still too frequent")
		case v < 20:
			return errors.New("занякал")
		case v < 50:
			return errors.New("в игнор ребёнка")
		}
	}

	var messageID string

	switch {
	case ctx.Message.MessageReference != nil:
		messageID = ctx.Message.MessageReference.MessageID
	case len(ctx.Args) > 1:
		messageID = ctx.Args[1]
	default:
		return ErrInvalidArgumentNumber
	}

	msg, err := ctx.Session.ChannelMessage(ctx.Message.ChannelID, messageID)
	if err != nil {
		return ErrInvalidMessageID
	}

	if mod.config.Client.Exists(msg.GuildID+"."+messageID+".pinned").Val() != 0 {
		return ErrTooEarly
	}

	msgs, err := mod.currentPins(ctx.Message.ChannelID)
	if err != nil {
		return err
	}

	found := false

	for _, m := range msgs {
		if m.ID == messageID {
			found = true
			break
		}
	}

	if !found {
		return ErrNotPinned
	}

	return ctx.Session.ChannelMessageUnpin(ctx.Message.ChannelID, messageID)
}
