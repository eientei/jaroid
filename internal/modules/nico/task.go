package nico

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/model"
)

const taskNico = "nico"

var vidbitrateExtractor = regexp.MustCompile(`@\s*([0-9]+)(\S*)`)

var audbitrateExtractor = regexp.MustCompile(`aac_([0-9]+)(\S*)bps`)

var humanFileSizeExtractor = regexp.MustCompile(`^\s*([0-9]+)(\S*)`)

var whitespaceSplitter = regexp.MustCompile(`\s+`)

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
	GuildID   string  `json:"guild_id"`
	ChannelID string  `json:"channel_id"`
	MessageID string  `json:"message_id"`
	VideoURL  string  `json:"video_url"`
	Target    float64 `json:"target"`
	Force     bool    `json:"force"`
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
	uri := mod.config.Config.Private.Nicovideo.Public + "/" + base
	sb := &strings.Builder{}
	_, _ = sb.WriteString("Downloaded as ")
	_, _ = sb.WriteString(uri)
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

type nicovideoThumb struct {
	XMLName xml.Name `xml:"nicovideo_thumb_response"`
	Thumb   struct {
		Length string `xml:"length"`
	} `xml:"thumb"`
}

func getNicovideoLength(id string) (time.Duration, error) {
	resp, err := http.Get("https://ext.nicovideo.jp/api/getthumbinfo/" + url.PathEscape(id))
	if err != nil {
		return 0, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	var thumb nicovideoThumb

	err = xml.NewDecoder(resp.Body).Decode(&thumb)
	if err != nil {
		return 0, err
	}

	if thumb.Thumb.Length == "" {
		return 0, nil
	}

	dur := time.Duration(0)
	unit := time.Second

	parts := strings.Split(thumb.Thumb.Length, ":")

	for i := len(parts) - 1; i >= 0; i-- {
		v, err := strconv.ParseUint(parts[i], 10, 64)
		if err != nil {
			return 0, err
		}

		dur += unit * time.Duration(v)
		unit *= 60
	}

	return dur, nil
}

func parseHumanSize(num, suffix string) int {
	value, err := strconv.ParseUint(num, 10, 64)
	if err != nil {
		return 0
	}

	m := strings.ToLower(suffix)

	switch {
	case m == "", strings.HasPrefix(m, "b"):
		return int(value)
	case strings.HasPrefix(m, "k"):
		return int(value) * 1024
	case strings.HasPrefix(m, "m"):
		return int(value) * 1024 * 1024
	case strings.HasPrefix(m, "g"):
		return int(value) * 1024 * 1024 * 1024
	}

	return 0
}

func findHumanSize(s string) int {
	matches := humanFileSizeExtractor.FindStringSubmatch(s)

	if len(matches) < 3 {
		return 0
	}

	return parseHumanSize(matches[1], matches[2])
}

func findBitrate(s string) int {
	vidmatches := vidbitrateExtractor.FindStringSubmatch(s)

	if len(vidmatches) < 3 {
		return 0
	}

	audmatches := audbitrateExtractor.FindStringSubmatch(s)

	return parseHumanSize(vidmatches[1], vidmatches[2]) + parseHumanSize(audmatches[1], audmatches[2])
}

var humanFileSuffixes = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}

func humanFileSize(size float64) string {
	i := 0

	for i < len(humanFileSuffixes) && size > 1024 {
		size /= 1024
		i++
	}

	return fmt.Sprintf("%5.1f%s", size, humanFileSuffixes[i])
}

func (mod *module) readListFormatsVideo(stdout io.ReadCloser, task *TaskList) (lines []string, err error) {
	var line string

	last := time.Now()

	bufread := bufio.NewReader(stdout)

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
			lines = append(lines, line)
		}
	}

	return
}

func (mod *module) listFormatsVideo(task *TaskList) (err error) {
	baseid := path.Base(task.VideoURL)

	dur, err := getNicovideoLength(baseid)
	if err != nil {
		return err
	}

	args := append([]string{}, mod.config.Config.Private.Nicovideo.Opts...)
	args = append(args, "-F")
	args = append(args, task.VideoURL)

	cmd := exec.Command("youtube-dl", args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		mod.config.Log.WithError(err).Error("Getting stdout")

		return err
	}

	err = cmd.Start()
	if err != nil {
		mod.config.Log.WithError(err).Error("Starting task")

		return err
	}

	lines, err := mod.readListFormatsVideo(stdout, task)
	if err != nil {
		mod.config.Log.WithError(err).Error("Reading stdout")

		return err
	}

	err = cmd.Wait()
	if err != nil {
		mod.config.Log.WithError(err).Error("Finishing executing")

		return err
	}

	buf, suggest, min := processLengthLines(lines, dur, task.Target)

	if task.Target > 0 && suggest.size == 0 {
		est := strings.TrimSpace(humanFileSize(min.size))
		note := fmt.Sprintf("Smallest format available: %s - est. %s", min.name, est)
		mod.updateMessage(task.GuildID, task.ChannelID, task.MessageID, note)

		if !task.Force {
			return nil
		}

		suggest = min
	}

	if task.Target > 0 {
		est := strings.TrimSpace(humanFileSize(suggest.size))
		note := fmt.Sprintf("Starting download... (%s, %s)", suggest.name, est)
		mod.updateMessage(task.GuildID, task.ChannelID, task.MessageID, note)

		return mod.config.Repository.TaskEnqueue(&TaskDownload{
			GuildID:   task.GuildID,
			ChannelID: task.ChannelID,
			MessageID: task.MessageID,
			VideoURL:  task.VideoURL,
			Format:    suggest.name,
		}, 0, 0)
	}

	mod.updateMessage(task.GuildID, task.ChannelID, task.MessageID, "```"+buf+"```")

	return nil
}

type formatMatch struct {
	name string
	size float64
}

func processLengthLines(lines []string, dur time.Duration, target float64) (out string, suggest, min formatMatch) {
	var maxlength int

	for _, l := range lines {
		if len(l) > maxlength {
			maxlength = len(l)
		}
	}

	var maxFitting, minimum float64

	var buf bytes.Buffer

	for _, l := range lines {
		if strings.HasPrefix(l, "format code") {
			buf.WriteString(l + " size(estimate)\n")
			continue
		}

		bitrate := findBitrate(l)

		if bitrate == 0 {
			buf.WriteString(l + "\n")
			continue
		}

		size := dur.Seconds() * (float64(bitrate) / 8)

		estimate := humanFileSize(size)

		if minimum == 0 || size < minimum {
			minimum = size
			parts := whitespaceSplitter.Split(l, -1)
			min = formatMatch{
				name: parts[0],
				size: size,
			}
		}

		if size > maxFitting && size <= target {
			maxFitting = size
			parts := whitespaceSplitter.Split(l, -1)
			suggest = formatMatch{
				name: parts[0],
				size: size,
			}
		}

		l += strings.Repeat(" ", maxlength-len(l))
		l += " " + estimate

		buf.WriteString(l + "\n")
	}

	return buf.String(), suggest, min
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
