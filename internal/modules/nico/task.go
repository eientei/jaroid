package nico

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/model"
)

const taskNico = "nico"

var filenameSanitizer = strings.NewReplacer(
	"/", "",
	"\x00", "",
	"\"", "",
	"\\", "",
	"'", "",
)

func filenameSanitize(s string) string {
	s = filenameSanitizer.Replace(s)

	bs := []byte(s)
	if len(bs) > 128 {
		bs = bs[:128]
	}

	return strings.ToValidUTF8(string(bs), "")
}

// TaskDownload provides video download task
type TaskDownload struct {
	GuildID   string `json:"guild_id"`
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
	VideoURL  string `json:"video_url"`
	Format    string `json:"format"`
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
	VideoURL  string `json:"video_url"`
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
	args := append([]string{}, mod.config.Config.Private.Nicovideo.Opts...)
	args = append(args, "-F")
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

	var buf bytes.Buffer

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

			mod.updateMessage(task.GuildID, task.ChannelID, task.MessageID, line)

			break
		}

		if time.Since(last) > time.Second*10 {
			last = time.Now()

			mod.updateMessage(task.GuildID, task.ChannelID, task.MessageID, line)
		}

		if !strings.HasPrefix(line, "[") {
			buf.WriteString(line)
			buf.WriteString("\n")
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

	mod.updateMessage(task.GuildID, task.ChannelID, task.MessageID, "```"+buf.String()+"```")

	return nil
}

func (mod *module) readDownloadVideo(task *TaskDownload, bufread *bufio.Reader) (err error) {
	var line string

	last := time.Now()

	for {
		var tmpline string

		tmpline, err = mod.readLine(bufread)

		if len(tmpline) > 0 {
			line = tmpline
		}

		if strings.Contains(line, "requested format not available") {
			mod.updateMessage(task.GuildID, task.ChannelID, task.MessageID, line)

			return ErrInvalidFormat
		}

		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				err = nil
			}

			mod.updateMessage(task.GuildID, task.ChannelID, task.MessageID, line)

			break
		}

		if time.Since(last) > time.Second*10 {
			last = time.Now()

			mod.updateMessage(task.GuildID, task.ChannelID, task.MessageID, line)
		}
	}

	return err
}

func (mod *module) downloadVideo(task *TaskDownload) (err error) {
	format := task.Format

	args := append([]string{}, mod.config.Config.Private.Nicovideo.Opts...)

	if format == "" {
		format = "max"
	} else {
		format = filenameSanitize(format)
	}

	args = append(args, "-o", mod.config.Config.Private.Nicovideo.Directory+"/%(id)s-"+format+".%(ext)s")

	if task.Format != "" {
		args = append(args, "-f", task.Format)
	}

	args = append(args, task.VideoURL)

	cmd := exec.Command("youtube-dl", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting stdout")

		return err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting stderr")

		return err
	}

	bufread := bufio.NewReader(io.MultiReader(stdout, stderr))

	err = cmd.Start()
	if err != nil {
		mod.config.Log.WithError(err).Error("Starting task")

		return err
	}

	err = mod.readDownloadVideo(task, bufread)
	if err != nil && err != ErrInvalidFormat {
		mod.config.Log.WithError(err).Error("Reading stdout")
	}

	if err == ErrInvalidFormat {
		_ = cmd.Wait()
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
	matches, err := filepath.Glob(mod.config.Config.Private.Nicovideo.Directory + "/" + fileID + "*")
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

		fileID := formatFileID(parts[len(parts)-1], task.Format)

		var fpath string

		if fpath, err = mod.globFind(fileID); err == nil && len(fpath) > 0 {
			mod.downloadSend(task, fpath)
			mod.ackTask(task, id, nil)

			continue downloadLoop
		}

		err = mod.tryPerform(func() error {
			return mod.downloadVideo(task)
		})
		if err != nil {
			mod.config.Log.WithError(err).Error("Downloading video")
			mod.updateMessage(task.GuildID, task.ChannelID, task.MessageID, "Downloading error")
			mod.ackTask(task, id, nil)

			continue
		}

		if fpath, err = mod.globFind(fileID); err == nil && len(fpath) > 0 {
			mod.downloadSend(task, fpath)
		}

		mod.ackTask(task, id, nil)
	}
}

func formatFileID(fileID, format string) string {
	if format == "" {
		return fileID + "-max"
	}

	return fileID + "-" + format
}

func (mod *module) tryPerform(action func() error) (err error) {
	i := 0
	last := time.Now()

	for {
		err = action()
		if err != nil && err != ErrInvalidFormat {
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
