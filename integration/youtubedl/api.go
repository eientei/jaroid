// Package youtubedl provides downloader service implementation using system youtube-dl
// deprecated
package youtubedl

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/eientei/jaroid/mediaservice"
)

// DefaultFormatRegexp default format parsing regexp
var DefaultFormatRegexp = `^\s*(?P<id>[^[]\S+aac_(?P<audio_bitrate>\d+[bkmg])\S*)\s*(?P<container>(\S+|))` +
	`\s*(?P<width>(\d+|))x*(?P<height>(\d+|))[^,]*,\s*` +
	`(?P<video_codec>[^@]*)@\s*(?P<video_bitrate>\d+[bkmg]*),` +
	`\s*(?P<audio_codec>\S*)`

// Downloader implementation
type Downloader struct {
	ExecutablePath string
	FormatRegexp   string
	CommonArgs     []string
	SaveArgs       []string
	ListArgs       []string
}

func (d *Downloader) readlines(
	ctx context.Context,
	cmd *exec.Cmd,
	r mediaservice.Reporter,
	reg *regexp.Regexp,
	formatID string,
) (lines []string, err error) {
	if r == nil {
		r = mediaservice.NewDummyReporter()
	}

	defer func() {
		r.Close()

		if err == nil {
			err = ctx.Err()
		}
	}()

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return
	}

	bufread := bufio.NewReader(io.MultiReader(stdout, stderr))

	defer func() {
		_ = stdout.Close()
		_ = stderr.Close()
	}()

	err = cmd.Start()
	if err != nil {
		return
	}

	cctx, cancel := context.WithCancel(ctx)

	go func() {
		<-cctx.Done()

		_ = cmd.Process.Kill()
	}()

	defer func() {
		_ = cmd.Wait()

		cancel()
	}()

	lines, err = d.readLinesLoop(bufread, r, reg, formatID)

	return
}

func (d *Downloader) readLinesLoop(
	bufread *bufio.Reader,
	r mediaservice.Reporter,
	reg *regexp.Regexp,
	formatID string,
) (lines []string, err error) {
	var line string

	for {
		line, err = bufread.ReadString('\n')

		line = strings.TrimSpace(line)

		if strings.Contains(line, "requested format not available") {
			err = fmt.Errorf("%w: %s", mediaservice.ErrUnknownFormat, formatID)
		}

		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				err = nil
			}

			if line != "" {
				r.Submit(line, true)
			}

			break
		}

		if line != "" {
			if loggable(reg, line) {
				r.Submit(line, false)
			}

			lines = append(lines, line)
		}
	}

	return
}

func loggable(reg *regexp.Regexp, line string) bool {
	return reg == nil || (!reg.MatchString(line) && !strings.HasPrefix(line, "format code"))
}

// ListFormatsRaw implementation
func (d *Downloader) ListFormatsRaw(
	ctx context.Context,
	opts *mediaservice.ListOptions,
	extraargs ...string,
) (formats []*mediaservice.Format, err error) {
	args := append([]string{}, d.CommonArgs...)
	args = append(args, d.ListArgs...)
	args = append(args, extraargs...)
	formatRegexpt := d.FormatRegexp

	if formatRegexpt == "" {
		formatRegexpt = DefaultFormatRegexp
	}

	formatRegexp, err := regexp.Compile(formatRegexpt)
	if err != nil {
		return nil, err
	}

	cmd := exec.Command(d.ExecutablePath, args...)

	var reporter mediaservice.Reporter

	if opts != nil {
		reporter = opts.Reporter
	}

	lines, err := d.readlines(ctx, cmd, reporter, formatRegexp, "")
	if err != nil {
		return nil, err
	}

	for _, l := range lines {
		matches := formatRegexp.FindStringSubmatch(l)
		if len(matches) == 0 {
			continue
		}

		id := matches[formatRegexp.SubexpIndex("id")]
		container := matches[formatRegexp.SubexpIndex("container")]
		width := matches[formatRegexp.SubexpIndex("width")]
		height := matches[formatRegexp.SubexpIndex("height")]
		videoCodec := matches[formatRegexp.SubexpIndex("video_codec")]
		videoBitrate := matches[formatRegexp.SubexpIndex("video_bitrate")]
		audioCodec := matches[formatRegexp.SubexpIndex("audio_codec")]
		audioBitrate := matches[formatRegexp.SubexpIndex("audio_bitrate")]

		if id == "" {
			continue
		}

		idparts := strings.Split(id, "-")

		if len(idparts) != 2 {
			continue
		}

		w, _ := strconv.ParseUint(width, 10, 64)
		h, _ := strconv.ParseUint(height, 10, 64)

		formats = append(formats, &mediaservice.Format{
			ID:        id,
			Container: mediaservice.Container(container),
			Audio: mediaservice.AudioFormat{
				ID:      idparts[1],
				Codec:   mediaservice.NewAudioCodec(audioCodec),
				Bitrate: mediaservice.HumanSizeParse(audioBitrate),
			},
			Video: mediaservice.VideoFormat{
				ID:      idparts[0],
				Codec:   mediaservice.NewVideoCodec(videoCodec),
				Bitrate: mediaservice.HumanSizeParse(videoBitrate),
				Width:   w,
				Height:  h,
			},
			Duration: 0,
		})
	}

	return
}

// ListFormats implementation
func (d *Downloader) ListFormats(
	ctx context.Context,
	url string,
	opts *mediaservice.ListOptions,
) (formats []*mediaservice.Format, err error) {
	return d.ListFormatsRaw(ctx, opts, "-F", url)
}

// SaveFormatRaw implementation
func (d *Downloader) SaveFormatRaw(
	ctx context.Context,
	opts *mediaservice.SaveOptions,
	extraargs ...string,
) (lines []string, err error) {
	args := append([]string{}, d.CommonArgs...)
	args = append(args, d.SaveArgs...)
	args = append(args, extraargs...)

	cmd := exec.Command(d.ExecutablePath, args...)

	var reporter mediaservice.Reporter

	if opts != nil {
		reporter = opts.Reporter
	}

	return d.readlines(ctx, cmd, reporter, nil, "")
}

// SaveFormat implementation
func (d *Downloader) SaveFormat(
	ctx context.Context,
	url, formatID, outpath string,
	opts *mediaservice.SaveOptions,
) (err error) {
	var args []string

	if formatID != "" {
		args = append(args, "-f", formatID)
	}

	args = append(args, "-o", outpath, url)

	if opts != nil && len(opts.Subtitles) > 0 {
		args = append(args, "--write-sub", "--sub-lang", strings.Join(opts.Subtitles, ","))
	}

	_, err = d.SaveFormatRaw(ctx, opts, args...)

	return
}
