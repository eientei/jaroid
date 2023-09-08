package nico

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/discordbot/model"
	"github.com/eientei/jaroid/mediaservice"
	"github.com/eientei/jaroid/nicopost"
)

const taskNico = "nico"

var confirmRegexp = regexp.MustCompile(`Smallest format available for <([^<]*)> - (\S*)`)

// TaskDownload provides video download task
type TaskDownload struct {
	GuildID   string          `json:"guild_id"`
	ChannelID string          `json:"channel_id"`
	MessageID string          `json:"message_id"`
	VideoURL  string          `json:"video_url"`
	Format    string          `json:"format"`
	UserID    string          `json:"user_id"`
	Subs      string          `json:"subs"`
	Data      json.RawMessage `json:"data"`
	Estimate  uint64          `json:"estimate"`
	Post      bool            `json:"post"`
	Preview   bool            `json:"preview"`
}

// Scope returns task scope
func (TaskDownload) Scope() string {
	return taskNico
}

// Name returns task name
func (TaskDownload) Name() string {
	return "download"
}

// TaskList provides list of video formats available
type TaskList struct {
	GuildID   string `json:"guild_id"`
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
	UserID    string `json:"user_id"`
	VideoURL  string `json:"video_url"`
	Subs      string `json:"subs"`
	Post      bool   `json:"post"`
}

// Scope returns task scope
func (TaskList) Scope() string {
	return taskNico
}

// Name returns task name
func (TaskList) Name() string {
	return "list"
}

// TaskCleanup provides message and file removal delayed task
type TaskCleanup struct {
	GuildID   string `json:"guild_id"`
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
	FilePath  string `json:"file_path"`
}

// Scope returns task scope
func (TaskCleanup) Scope() string {
	return taskNico
}

// Name returns task name
func (TaskCleanup) Name() string {
	return "cleanup"
}

// TaskPleromaPost posts video to pleroma instance
type TaskPleromaPost struct {
	GuildID     string `json:"guild_id"`
	ChannelID   string `json:"channel_id"`
	MessageID   string `json:"message_id"`
	VideoURL    string `json:"video_url"`
	FilePath    string `json:"file_path"`
	PleromaHost string `json:"pleroma_host"`
	PleromaAuth string `json:"pleroma_auth"`
	Preview     bool   `json:"preview"`
}

// Scope returns task scope
func (TaskPleromaPost) Scope() string {
	return taskNico
}

// Name returns task name
func (TaskPleromaPost) Name() string {
	return "pleroma_post"
}

func (mod *module) ackTask(task model.Task, id string, err error) {
	if err != nil {
		mod.config.Log.WithError(err).Error("Dequeuing")

		if r, ok := err.(*discordgo.RESTError); ok && r.Response.StatusCode == 404 {
			err = nil
		}
	}

	if err == nil {
		err = mod.config.Repository.TaskAck(task, id)
		if err != nil {
			mod.config.Log.WithError(err).Error("Acking task", id)
		}
	}
}

func subtitleFilename(s, subs string) string {
	return strings.ReplaceAll(s, ".mp4", "."+subs+".ass")
}

func (mod *module) downloadSend(task *TaskDownload, fpath string) {
	time.Sleep(time.Second)

	base := filepath.Base(fpath)
	uri := mod.config.Config.Private.Nicovideo.Public + "/" + base
	sb := &strings.Builder{}
	_, _ = sb.WriteString("Downloaded as ")
	_, _ = sb.WriteString(uri)
	_, _ = sb.WriteString(" file will be deleted after " + mod.config.Config.Private.Nicovideo.Period.String())

	if task.Subs != "" {
		sb.WriteString("\ndanmaku subtitles: " + subtitleFilename(uri, task.Subs))
	}

	var err error

	if !task.Preview {
		_, err = mod.config.Discord.ChannelMessageEdit(task.ChannelID, task.MessageID, sb.String())
	}

	if err != nil {
		mod.config.Log.WithError(err).Error("Editing message", task.GuildID, task.ChannelID, task.MessageID)
	}

	if task.Post {
		mod.pleromaPostEnqueue(task, fpath)
	}
}

func (mod *module) updateMessage(guildID, channelID, messageID, line string) {
	if len(line) == 0 {
		return
	}

	_, err := mod.config.Discord.ChannelMessageEdit(channelID, messageID, line)
	if err != nil {
		mod.config.Log.WithError(err).Error("Editing message", guildID, channelID, messageID)
	}
}

func (mod *module) listFormatsVideo(task *TaskList) (err error) {
	reporter := mediaservice.NewReporter(time.Second*10, 1, os.Stdin)

	go func() {
		for r := range reporter.Messages() {
			_, err = mod.config.Discord.ChannelMessageEdit(task.ChannelID, task.MessageID, r)
			if err != nil {
				mod.config.Log.WithError(err).Error("updating message")
			}
		}
	}()

	formats, err := mod.config.Nicovideo.ListFormats(context.Background(), task.VideoURL, &mediaservice.ListOptions{
		Reporter: reporter,
	})
	if err != nil {
		return err
	}

	buf := nicopost.ProcessFormats(formats)

	mod.updateMessage(task.GuildID, task.ChannelID, task.MessageID, "```"+buf+"```")

	return nil
}

func (mod *module) downloadVideo(ctx context.Context, id string, task *TaskDownload) (fmtname string, err error) {
	err = os.MkdirAll(mod.config.Config.Private.Nicovideo.Directory, 0777)
	if err != nil {
		return "", err
	}

	output := nicopost.SaveFilepath(mod.config.Config.Private.Nicovideo.Directory, task.VideoURL, task.Format)

	opts := &mediaservice.SaveOptions{
		Reporter: mediaservice.NewReporter(time.Second*10, 1, os.Stdin),
	}

	if task.Subs != "" {
		opts.Subtitles = append(opts.Subtitles, task.Subs)
	}

	go func() {
		for r := range opts.Reporter.Messages() {
			_, err = mod.config.Discord.ChannelMessageEdit(task.ChannelID, task.MessageID, id+" [downloading] "+r)
			if err != nil {
				mod.config.Log.WithError(err).Error("updating message")
			}
		}
	}()

	fmtname, err = mod.config.Nicovideo.SaveFormat(ctx, task.VideoURL, task.Format, output, true, task.Data, opts)
	if err != nil {
		opts.Reporter.Submit("ERROR: "+err.Error(), true)

		mod.config.Log.WithError(err).Error("downloading file")
	}

	return
}

func (mod *module) startList() {
	task := &TaskList{}

	for {
		id, err := mod.config.Repository.TaskDequeue(task, time.Second)
		if err != nil {
			mod.config.Log.WithError(err).Error("Dequeuing")
			continue
		}

		if id == "" {
			continue
		}

		parts := strings.Split(task.VideoURL, "/")
		if len(parts) == 0 {
			mod.ackTask(task, id, nil)

			continue
		}

		err = mod.tryPerform(func() error {
			return mod.listFormatsVideo(task)
		})
		if err != nil {
			mod.config.Log.WithError(err).Error("Listing formats for video")
			mod.updateMessage(task.GuildID, task.ChannelID, task.MessageID, "Listing formats error")
		}

		mod.ackTask(task, id, nil)
	}
}

func (mod *module) startDownloadError(err error, task *TaskDownload) {
	if err == context.Canceled {
		mod.updateMessage(task.GuildID, task.ChannelID, task.MessageID, "Cancelled")

		return
	}

	if err == context.DeadlineExceeded {
		mod.updateMessage(
			task.GuildID,
			task.ChannelID,
			task.MessageID,
			"Video took more than 1h to download, repeat request to resume within next 24h before it is deleted",
		)

		return
	}

	mod.config.Log.WithError(err).Error("Downloading video")
	mod.updateMessage(task.GuildID, task.ChannelID, task.MessageID, "Downloading error")
}

func subsExist(task *TaskDownload, path string) bool {
	if task.Subs == "" {
		return true
	}

	cand := subtitleFilename(path, task.Subs)

	_, err := os.Stat(cand)

	return err == nil
}

func (mod *module) startDownload() {
	task := &TaskDownload{}

	for {
		id, err := mod.config.Repository.TaskDequeue(task, time.Second)
		if err != nil {
			mod.config.Log.WithError(err).Error("Dequeuing")
			continue
		}

		if id == "" {
			continue
		}

		basename := path.Base(task.VideoURL)
		if len(basename) == 0 {
			mod.ackTask(task, id, nil)

			continue
		}

		fileID := nicopost.FormatFileID(basename, task.Format)

		var fpath string

		if fpath, err = nicopost.GlobFind(mod.config.Config.Private.Nicovideo.Directory, fileID); err == nil &&
			len(fpath) > 0 && subsExist(task, fpath) {
			mod.downloadSend(task, fpath)
			mod.ackTask(task, id, nil)

			_ = mod.config.Discord.MessageReactionRemove(task.ChannelID, task.MessageID, emojiStop, "@me")

			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)

		mod.m.Lock()
		mod.task = task
		mod.cancel = cancel
		mod.m.Unlock()

		err = mod.tryPerform(func() (err error) {
			fpath, err = mod.downloadVideo(ctx, id, task)

			return
		})

		cancel()

		_ = mod.config.Discord.MessageReactionRemove(task.ChannelID, task.MessageID, emojiStop, "@me")

		mod.m.Lock()

		mod.task = nil
		mod.cancel = nil

		mod.m.Unlock()

		mod.ackTask(task, id, nil)

		if len(fpath) > 0 {
			_, _, cerr := mod.config.Repository.TaskEnqueue(&TaskCleanup{
				GuildID:   task.GuildID,
				ChannelID: task.ChannelID,
				MessageID: task.MessageID,
				FilePath:  fpath,
			}, mod.config.Config.Private.Nicovideo.Period, 0)
			if cerr != nil {
				mod.config.Log.WithError(cerr).Error(
					"Scheduling cleanup",
					task.GuildID,
					task.ChannelID,
					task.MessageID,
				)
			}

			if task.Subs != "" {
				_, _, cerr = mod.config.Repository.TaskEnqueue(&TaskCleanup{
					GuildID:   task.GuildID,
					ChannelID: task.ChannelID,
					MessageID: task.MessageID,
					FilePath:  subtitleFilename(fpath, task.Subs),
				}, mod.config.Config.Private.Nicovideo.Period, 0)
				if cerr != nil {
					mod.config.Log.WithError(cerr).Error(
						"Scheduling subtitle cleanup",
						task.GuildID,
						task.ChannelID,
						task.MessageID,
					)
				}
			}
		}

		if err != nil {
			mod.startDownloadError(err, task)

			continue
		}

		if len(fpath) > 0 {
			mod.downloadSend(task, fpath)
		}
	}
}

func (mod *module) tryPerform(action func() error) (err error) {
	i := 0
	last := time.Now()

	for i < 3 {
		err = action()
		if err != nil {
			if errors.Is(err, mediaservice.ErrUnknownFormat) ||
				err == context.Canceled || err == context.DeadlineExceeded {
				break
			}

			if time.Since(last) < time.Second*30 {
				i++
			} else {
				i = 0
			}

			last = time.Now()
		} else {
			break
		}
	}

	return
}

func (mod *module) startCleanup() {
	task := &TaskCleanup{}

	for {
		id, err := mod.config.Repository.TaskDequeue(task, time.Second)
		if err != nil {
			mod.config.Log.WithError(err).Error("Dequeuing")
		}

		if id == "" {
			continue
		}

		_ = os.Remove(task.FilePath)

		line := "Downloaded video deleted due to expiration"
		_, err = mod.config.Discord.ChannelMessageEdit(task.ChannelID, task.MessageID, line)
		mod.ackTask(task, id, err)
	}
}
