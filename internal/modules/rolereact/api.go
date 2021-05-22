// Package rolereact provides user-assignable roles via reactions
package rolereact

import (
	"bufio"
	"errors"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/bot"
	"github.com/eientei/jaroid/internal/modules/auth"
	"github.com/eientei/jaroid/internal/router"
)

var roleRegexp = regexp.MustCompile(`<@&(\d+)>`)

var emojiRegexp = regexp.MustCompile(`((` +
	`\x{00a9}|\x{00ae}|` +
	`[\x{2000}-\x{3300}]|` +
	`\x{d83c}[\x{d000}-\x{dfff}]|` +
	`\x{d83d}[\x{d000}-\x{dfff}]|` +
	`\x{d83e}[\x{d000}-\x{dfff}]|` +
	`[\x{1F000}-\x{1FFFF}]` +
	`)|<:([^:]+:\d+)>)`)

var (
	// ErrInvalidArgumentNumber is returned on invalid argument number
	ErrInvalidArgumentNumber = errors.New("invalid argument number, use rolereact.help")
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

	config.Discord.AddHandler(mod.handlerReactionAdd)
	config.Discord.AddHandler(mod.handlerReactionRemove)

	group := config.Router.Group("rolereact").SetDescription("reaction-based roles")

	group.Set(auth.RouteConfigKey, &auth.RouteConfig{
		Permissions: discordgo.PermissionAdministrator,
	})

	group.On("rolereact.enable", "enable reaction roles for message id", mod.commandEnable)
	group.On("rolereact.disable", "disable reaction roles for message id", mod.commandDisable)
	group.On("rolereact.help", "provides documentation", mod.commandHelp)

	return nil
}

func (mod *module) Configure(config *bot.Configuration, guild *discordgo.Guild) {

}

func (mod *module) Shutdown(config *bot.Configuration) {

}

type roleEmoji struct {
	role   string
	emojis []string
}

func (mod *module) findRole(roleEmojis []*roleEmoji, msg *discordgo.Message, expect string) string {
	var found bool

	for _, mr := range msg.Reactions {
		if mr.Me && mr.Emoji.APIName() == expect {
			found = true
			break
		}
	}

	if !found {
		return ""
	}

	for _, r := range roleEmojis {
		for _, e := range r.emojis {
			if expect == e {
				return r.role
			}
		}
	}

	return ""
}

func (mod *module) handlerReactionAdd(session *discordgo.Session, messageReactionAdd *discordgo.MessageReactionAdd) {
	msg, err := session.ChannelMessage(messageReactionAdd.ChannelID, messageReactionAdd.MessageID)
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting message", messageReactionAdd.ChannelID, messageReactionAdd.MessageID)
		return
	}

	roleEmojis := mod.parseRoleEmoji(msg.Content)

	role := mod.findRole(roleEmojis, msg, messageReactionAdd.Emoji.APIName())

	if role == "" {
		return
	}

	err = mod.config.Discord.GuildMemberRoleAdd(messageReactionAdd.GuildID, messageReactionAdd.UserID, role)
	if err != nil {
		mod.config.Log.WithError(err).Errorf(
			"adding role %s to %s.%s",
			role,
			messageReactionAdd.GuildID,
			messageReactionAdd.UserID,
		)
	}
}

func (mod *module) handlerReactionRemove(
	session *discordgo.Session,
	messageReactionRemove *discordgo.MessageReactionRemove,
) {
	msg, err := session.ChannelMessage(messageReactionRemove.ChannelID, messageReactionRemove.MessageID)
	if err != nil {
		mod.config.Log.WithError(err).Error(
			"Getting message",
			messageReactionRemove.ChannelID,
			messageReactionRemove.MessageID,
		)

		return
	}

	roleEmojis := mod.parseRoleEmoji(msg.Content)

	role := mod.findRole(roleEmojis, msg, messageReactionRemove.Emoji.APIName())

	if role == "" {
		return
	}

	err = mod.config.Discord.GuildMemberRoleRemove(messageReactionRemove.GuildID, messageReactionRemove.UserID, role)
	if err != nil {
		mod.config.Log.WithError(err).Errorf(
			"removing role %s from %s.%s",
			role,
			messageReactionRemove.GuildID,
			messageReactionRemove.UserID,
		)
	}
}

func (mod *module) parseRoleEmoji(content string) (roles []*roleEmoji) {
	scan := bufio.NewScanner(strings.NewReader(content))
	for scan.Scan() {
		line := scan.Text()

		matches := roleRegexp.FindStringSubmatch(line)
		if len(matches) < 2 {
			continue
		}

		role := &roleEmoji{
			role: matches[1],
		}

		demojis := emojiRegexp.FindAllStringSubmatch(line, -1)

		for _, m := range demojis {
			if len(m) < 4 {
				continue
			}

			if m[3] != "" {
				role.emojis = append(role.emojis, m[3])
			} else {
				role.emojis = append(role.emojis, m[2])
			}
		}

		roles = append(roles, role)
	}

	return
}

func (mod *module) commandEnable(ctx *router.Context) error {
	var messageID string

	switch {
	case ctx.Message.MessageReference != nil:
		messageID = ctx.Message.MessageReference.MessageID
	case len(ctx.Args) > 1:
		messageID = ctx.Args[1]
	default:
		return ErrInvalidArgumentNumber
	}

	msg, err := mod.config.Discord.ChannelMessage(ctx.Message.ChannelID, messageID)
	if err != nil {
		return err
	}

	roles := mod.parseRoleEmoji(msg.Content)

	for _, r := range roles {
		for _, e := range r.emojis {
			err = mod.config.Discord.MessageReactionAdd(ctx.Message.ChannelID, messageID, e)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (mod *module) removeAllUsers(guildID, channelID, messageID, emoji, role string) error {
	var after string

	for {
		users, err := mod.config.Discord.MessageReactions(
			channelID,
			messageID,
			emoji,
			100,
			"",
			after,
		)
		if err != nil {
			return err
		}

		if len(users) == 0 {
			break
		}

		for _, u := range users {
			err = mod.config.Discord.GuildMemberRoleRemove(guildID, u.ID, role)
			if err != nil {
				mod.config.Log.WithError(err).Errorf(
					"removing role %s from %s.%s",
					role,
					guildID,
					u.ID,
				)
			}
		}

		after = users[0].ID
	}

	return nil
}

func (mod *module) commandDisable(ctx *router.Context) error {
	var messageID string

	switch {
	case ctx.Message.MessageReference != nil:
		messageID = ctx.Message.MessageReference.MessageID
	case len(ctx.Args) > 1:
		messageID = ctx.Args[1]
	default:
		return ErrInvalidArgumentNumber
	}

	msg, err := mod.config.Discord.ChannelMessage(ctx.Message.ChannelID, messageID)
	if err != nil {
		return err
	}

	roleEmojis := mod.parseRoleEmoji(msg.Content)

	for _, r := range msg.Reactions {
		if !r.Me {
			continue
		}

		role := mod.findRole(roleEmojis, msg, r.Emoji.APIName())

		if role != "" {
			err = mod.removeAllUsers(ctx.Message.GuildID, ctx.Message.ChannelID, messageID, r.Emoji.APIName(), role)
			if err != nil {
				return err
			}
		}

		err = mod.config.Discord.MessageReactionRemove(ctx.Message.ChannelID, messageID, r.Emoji.APIName(), "@me")
		if err != nil {
			return err
		}
	}

	return nil
}

func (mod *module) commandHelp(ctx *router.Context) error {
	return ctx.ReplyEmbed("```yaml\n" + `
usage:
> rolereact.enable <messageID>
> rolereact.disable <messageID>

alternatively: reply the message with roles with
rolereact.* command

Message can have N lines, each having a role mention and
a list of emojis, that would grant this role.

As long as role mention and emojis are on the same line,
such emoji would be recognized as assigned mentioned role

When enabled, bot will react with each of role-associated
emojis.

When disabled, bot will remove own reactions and all users
from associated roles.

If you edited the message, call enable again.

message example:
@role1Mention emoji1 some text
@role2Mention emoji2 some text
@role3Mention emoji3 some text
` + "```")
}
