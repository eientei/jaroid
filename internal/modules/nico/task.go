package nico

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/model"
)

// TaskDownload provides video download task
type TaskDownload struct {
	GuildID   string `json:"guild_id"`
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
	VideoURL  string `json:"video_url"`
}

// Scope returns task scope
func (TaskDownload) Scope() string {
	return "nico"
}

// Name returns task name
func (TaskDownload) Name() string {
	return "download"
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
	return "nico"
}

// Name returns task name
func (TaskCleanup) Name() string {
	return "cleanup"
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

func (mod *module) readLine(bufread *bufio.Reader) (line string, err error) {
	for {
		bs, prefix, err := bufread.ReadLine()
		if err != nil {
			if err != io.EOF && err != io.ErrUnexpectedEOF {
				return "", err
			}

			prefix = false
		}

		line += string(bs)

		if !prefix {
			return line, err
		}
	}
}

func (mod *module) downloadSend(task *TaskDownload, fpath string) {
	base := filepath.Base(fpath)
	url := mod.config.Config.Private.Nicovideo.Public + "/" + base
	sb := &strings.Builder{}
	_, _ = sb.WriteString("Downloaded as ")
	_, _ = sb.WriteString(url)
	_, _ = sb.WriteString(" file will be deleted after " + mod.config.Config.Private.Nicovideo.Period.String())
	_, err := mod.config.Discord.ChannelMessageEdit(task.ChannelID, task.MessageID, sb.String())

	if err != nil {
		mod.config.Log.WithError(err).Error("Editing message", task.GuildID, task.ChannelID, task.MessageID)
	}

	err = mod.config.Repository.TaskEnqueue(&TaskCleanup{
		GuildID:   task.GuildID,
		ChannelID: task.ChannelID,
		MessageID: task.MessageID,
		FilePath:  fpath,
	}, mod.config.Config.Private.Nicovideo.Period, 0)
	if err != nil {
		mod.config.Log.WithError(err).Error("Scheduling cleanup", task.GuildID, task.ChannelID, task.MessageID)
	}
}

func (mod *module) updateMessage(task *TaskDownload, line string) {
	if len(line) == 0 {
		return
	}

	_, err := mod.config.Discord.ChannelMessageEdit(task.ChannelID, task.MessageID, line)
	if err != nil {
		mod.config.Log.WithError(err).Error("Editing message", task.GuildID, task.ChannelID, task.MessageID)
	}
}

func (mod *module) downloadVideo(task *TaskDownload) (err error) {
	args := append([]string{}, mod.config.Config.Private.Nicovideo.Opts...)
	args = append(args, "-o", mod.config.Config.Private.Nicovideo.Directory+"/%(id)s-max.%(ext)s")
	args = append(args, task.VideoURL)

	cmd := exec.Command("youtube-dl", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting stdout")

		return err
	}

	bufread := bufio.NewReader(stdout)

	err = cmd.Start()
	if err != nil {
		mod.config.Log.WithError(err).Error("Starting task")

		return err
	}

	var line string

	last := time.Now()

	for {
		var tmpline string

		tmpline, err = mod.readLine(bufread)

		if len(tmpline) > 0 {
			line = tmpline
		}

		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				err = nil
			}

			mod.updateMessage(task, line)

			break
		}

		if time.Since(last) > time.Second*10 {
			last = time.Now()

			mod.updateMessage(task, line)
		}
	}

	if err != nil {
		mod.config.Log.WithError(err).Error("Reading stdout")

		return err
	}

	err = cmd.Wait()
	if err != nil {
		mod.config.Log.WithError(err).Error("Finishing executing")

		return err
	}

	return nil
}

func (mod *module) globFind(fileID string) (string, error) {
	matches, err := filepath.Glob(mod.config.Config.Private.Nicovideo.Directory + "/" + fileID + "-*")
	if err != nil {
		mod.config.Log.WithError(err).Error("Globbing")

		return "", err
	}

	for _, m := range matches {
		if !strings.HasSuffix(m, ".part") {
			return m, nil
		}
	}

	return "", nil
}

func (mod *module) startDownload() {
	task := &TaskDownload{}

downloadLoop:
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

		fileID := parts[len(parts)-1]

		var fpath string

		if fpath, err = mod.globFind(fileID); err == nil && len(fpath) > 0 {
			mod.downloadSend(task, fpath)
			mod.ackTask(task, id, nil)

			continue downloadLoop
		}

		err = mod.tryDownload(task)
		if err != nil {
			mod.config.Log.WithError(err).Error("Downloading video")
			mod.updateMessage(task, "Downloading error")
			mod.ackTask(task, id, nil)

			continue
		}

		if fpath, err = mod.globFind(fileID); err == nil && len(fpath) > 0 {
			mod.downloadSend(task, fpath)
		}

		mod.ackTask(task, id, nil)
	}
}

func (mod *module) tryDownload(task *TaskDownload) (err error) {
	i := 0
	last := time.Now()

	for {
		err = mod.downloadVideo(task)
		if err != nil {
			if time.Since(last) < time.Second*30 {
				i++
			} else {
				i = 0
			}

			last = time.Now()

			if i >= 3 {
				return err
			}
		} else {
			return nil
		}
	}
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
