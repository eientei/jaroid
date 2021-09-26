// Package nicopost provides utils for creating nicovideo-themed posts
package nicopost

import (
	"bytes"
	"fmt"
	"math"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/eientei/jaroid/fedipost"
	"github.com/eientei/jaroid/fedipost/config"
	"github.com/eientei/jaroid/fedipost/media"
	"github.com/eientei/jaroid/fedipost/statuses"
	"github.com/eientei/jaroid/mediaservice"
	"github.com/eientei/jaroid/nicovideo/search"
)

// MakeNicovideoStatus returns new fediverse status for provided video url/path and post template
func MakeNicovideoStatus(
	conf *fedipost.Config,
	client *search.Client,
	videoURL, videoPath, tmpl string,
) (*statuses.CreateStatus, error) {
	if tmpl == "" {
		tmpl = config.DefaultTemplate
	}

	t, err := template.New("").Funcs(template.FuncMap{
		"makeTag": statuses.MakeTag,
		"makeTags": func(ss []string) (tags []string) {
			for _, s := range ss {
				tags = append(tags, statuses.MakeTag(s))
			}

			return
		},
		"join": func(delim string, ss []string) string {
			return strings.Join(ss, delim)
		},
	}).Parse(tmpl)
	if err != nil {
		return nil, err
	}

	name := path.Base(videoURL)

	res, err := client.ThumbInfo(name)
	if err != nil {
		return nil, err
	}

	vars := map[string]interface{}{
		"url":      videoURL,
		"file":     videoPath,
		"filename": path.Base(videoPath),
		"info":     res,
	}

	buf := &bytes.Buffer{}

	err = t.Execute(buf, vars)
	if err != nil {
		return nil, err
	}

	mediaID, err := media.UploadFile(conf, videoPath)
	if err != nil {
		return nil, err
	}

	return &statuses.CreateStatus{
		Status:      buf.String(),
		ContentType: "text/markdown",
		MediaIDs:    []string{mediaID},
	}, nil
}

var filenameSanitizer = strings.NewReplacer(
	"/", "",
	"\x00", "",
	"\"", "",
	"\\", "",
	"'", "",
)

// FilenameSanitize returns sanitized filename
func FilenameSanitize(s string) string {
	s = filenameSanitizer.Replace(s)

	bs := []byte(s)
	if len(bs) > 128 {
		bs = bs[:128]
	}

	return strings.ToValidUTF8(string(bs), "")
}

// SaveFilepath return save filepath for provided format
func SaveFilepath(savedir, format string) string {
	fmn := format

	if fmn == "" {
		fmn = "max"
	} else {
		fmn = FilenameSanitize(fmn)
	}

	return filepath.Join(savedir, "%(id)s-"+fmn+".%(ext)s")
}

// GlobFind tries to find existing file with provided parent dir and file format id
func GlobFind(dir, fileFormatID string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, fileFormatID) + "*")
	if err != nil {
		return "", err
	}

	fmt.Println(matches, dir, fileFormatID)

	for _, m := range matches {
		if !strings.HasSuffix(m, ".part") && !strings.HasSuffix(m, ".ass") {
			return m, nil
		}
	}

	return "", nil
}

// FormatFileID formats video file id joined with media format id
func FormatFileID(fileID, format string) string {
	if format == "" {
		return fileID + "-max"
	}

	return fileID + "-" + format
}

// FormatMatch found match
type FormatMatch struct {
	Name string
	Size float64
}

// ProcessFormats finds closest lesser or equal format and smallest avilable format, as well listing of all formats
func ProcessFormats(
	lines []*mediaservice.Format,
	dur time.Duration,
	target float64,
) (out string, suggest, min FormatMatch) {
	sort.Slice(lines, func(i, j int) bool {
		return lines[i].Video.Bitrate+lines[i].Audio.Bitrate < lines[j].Video.Bitrate+lines[j].Audio.Bitrate
	})

	var maxlength [8]int

	var maxFitting, minimum float64

	var vals [][]string

	for _, l := range lines {
		bitrate := l.Video.Bitrate + l.Audio.Bitrate

		res := fmt.Sprintf("%dx%d", l.Video.Width, l.Video.Height)

		vidrate := fmt.Sprintf("%dk", l.Video.Bitrate/1024)
		audrate := fmt.Sprintf("%dk", l.Audio.Bitrate/1024)

		size := dur.Seconds() * (float64(bitrate) / 8)
		estimate := mediaservice.HumanSizeFormat(size)

		maxlength[0] = int(math.Max(float64(maxlength[0]), float64(len(l.ID))))
		maxlength[1] = int(math.Max(float64(maxlength[1]), float64(len(l.Container))))
		maxlength[2] = int(math.Max(float64(maxlength[2]), float64(len(res))))
		maxlength[3] = int(math.Max(float64(maxlength[3]), float64(len(l.Video.Codec))))
		maxlength[4] = int(math.Max(float64(maxlength[4]), float64(len(vidrate))))
		maxlength[5] = int(math.Max(float64(maxlength[5]), float64(len(l.Audio.Codec))))
		maxlength[6] = int(math.Max(float64(maxlength[6]), float64(len(audrate))))
		maxlength[7] = int(math.Max(float64(maxlength[7]), float64(len(estimate))))

		vals = append(vals, []string{
			l.ID,
			string(l.Container),
			res,
			string(l.Video.Codec),
			vidrate,
			string(l.Audio.Codec),
			audrate,
			estimate,
		})

		if minimum == 0 || size < minimum {
			minimum = size
			min = FormatMatch{
				Name: l.ID,
				Size: size,
			}
		}

		if size > maxFitting && size <= target {
			maxFitting = size
			suggest = FormatMatch{
				Name: l.ID,
				Size: size,
			}
		}
	}

	headers := []string{
		"id",
		"container",
		"resolution",
		"vcodec",
		"vbitrate",
		"acodec",
		"abitrate",
		"size(estimate)",
	}

	for i, h := range headers {
		maxlength[i] = int(math.Max(float64(maxlength[i]), float64(len(h))))
	}

	var buf bytes.Buffer

	for i, h := range headers {
		buf.WriteString(h)
		buf.WriteString(strings.Repeat(" ", maxlength[i]-len(h)+1))
	}

	buf.WriteString("\n")

	for _, l := range vals {
		for i, v := range l {
			buf.WriteString(v)
			buf.WriteString(strings.Repeat(" ", maxlength[i]-len(v)+1))
		}

		buf.WriteString("\n")
	}

	return buf.String(), suggest, min
}
