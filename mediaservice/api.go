// Package mediaservice contains API for media content downloader as well as child implementation packages
package mediaservice

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrUnknownFormat is returned when unknown media format is selected
	ErrUnknownFormat = errors.New("unknown format")

	formatSizeRegex = regexp.MustCompile(`^\s*([0-9]+)((?i)[bkmg]?)(!?)\s*$`)
)

// ErrFormatSuggest is returned when no format is smaller than specified constraint with smallest available
// as suggestion
type ErrFormatSuggest struct {
	*Format
}

// Error implementation
func (v *ErrFormatSuggest) Error() string {
	return "Smallest available format is larger than requested constraint"
}

// AudioCodec enumeration
type AudioCodec string

// Know AudioCodec vlues
var (
	AudioCodecAAC  AudioCodec = "aac"
	AudioCodecOGG  AudioCodec = "ogg"
	AudioCodecOPUS AudioCodec = "opus"
)

// NewAudioCodec returns AudioCodec from string
func NewAudioCodec(s string) AudioCodec {
	return AudioCodec(strings.ToLower(s))
}

// AudioFormat details
type AudioFormat struct {
	ID         string
	Codec      AudioCodec
	Bitrate    uint64
	Samplerate uint64
	Channels   uint8
}

// VideoCodec enumeration
type VideoCodec string

// Know VideoCodec vlues
const (
	VideoCodecH264 VideoCodec = "h264"
	VideoCodecVP8  VideoCodec = "vp8"
	VideoCodecVP9  VideoCodec = "vp9"
)

// NewVideoCodec returns VideoCodec from string
func NewVideoCodec(s string) VideoCodec {
	return VideoCodec(strings.ToLower(s))
}

// VideoFormat details
type VideoFormat struct {
	ID      string
	Codec   VideoCodec
	Bitrate uint64
	Width   uint64
	Height  uint64
}

// Container enumeration
type Container string

// Know container vlues
const (
	ContainerMP4  Container = "mp4"
	ContainerWEBM Container = "webm"
)

// NewContainer returns Container from string
func NewContainer(s string) Container {
	return Container(strings.ToLower(s))
}

// Format details
type Format struct {
	ID        string
	Container Container
	Audio     AudioFormat
	Video     VideoFormat
	Duration  time.Duration
}

// SizeEstimate returns file size estimate based on duration and bitrate
func (f *Format) SizeEstimate() uint64 {
	br := f.Audio.Bitrate + f.Video.Bitrate

	return (uint64(f.Duration.Seconds()) * br) / 8
}

// ListOptions options for ListFormats
type ListOptions struct {
	Reporter Reporter
}

// GetReporter or default dummy
func (s *ListOptions) GetReporter() Reporter {
	if s == nil || s.Reporter == nil {
		return NewDummyReporter()
	}

	return s.Reporter
}

// SaveOptions options for SaveFormat
type SaveOptions struct {
	Reporter  Reporter
	Subtitles []string
}

// GetReporter or default dummy
func (s *SaveOptions) GetReporter() Reporter {
	if s == nil || s.Reporter == nil {
		return NewDummyReporter()
	}

	return s.Reporter
}

// Downloader provides list of available formats and performs download of selected format
type Downloader interface {
	ListFormats(
		ctx context.Context,
		url string,
		opts *ListOptions,
	) ([]*Format, error)

	SaveFormat(
		ctx context.Context,
		url, formatID, outpath string,
		reuse bool,
		data []byte,
		opts *SaveOptions,
	) (string, error)
}

// MatchesHumanSize returns true if given string conforms to human size format
func MatchesHumanSize(s string) bool {
	return formatSizeRegex.MatchString(s)
}

// HumanSizeParse parses human-formatted size as bytes
func HumanSizeParse(s string) uint64 {
	parts := formatSizeRegex.FindStringSubmatch(s)
	if len(parts) < 3 {
		return 0
	}

	return HumanSizeParseParts(parts[1], parts[2], 1024)
}

// HumanSizeParseParts parses parts of human-formatted size as bytes
func HumanSizeParseParts(num, suffix string, base float64) uint64 {
	value, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0
	}

	m := strings.ToLower(suffix)

	switch {
	case m == "", strings.HasPrefix(m, "b"):
		return uint64(value)
	case strings.HasPrefix(m, "k"):
		return uint64(value * base)
	case strings.HasPrefix(m, "m"):
		return uint64(value * base * base)
	case strings.HasPrefix(m, "g"):
		return uint64(value * base * base * base)
	}

	return 0
}

var humanFileSuffixes = []string{"B", "KiB", "MiB", "GiB", "TiB", "PiB"}

// HumanSizeFormat fromats size in bytes as human-readable file size
func HumanSizeFormat(size float64) string {
	i := 0

	for i < len(humanFileSuffixes) && size > 1024 {
		size /= 1024
		i++
	}

	return fmt.Sprintf("%5.1f%s", size, humanFileSuffixes[i])
}

func parseFormatSize(formatID string) (dsize uint64, wildcard bool, tgt string, err error) {
	switch {
	case formatID == "inf" || formatID == "max" || formatID == "":
		dsize = math.MaxUint64
	case formatSizeRegex.MatchString(formatID):
		parts := formatSizeRegex.FindStringSubmatch(formatID)
		sizestr, sizespecstr, wildstr := parts[1], parts[2], parts[3]

		dsize = HumanSizeParseParts(sizestr, sizespecstr, 1024)
		if dsize == 0 {
			err = strconv.ErrSyntax

			return
		}

		wildcard = wildstr == "!"
	default:
		parts := strings.Split(formatID, "-")

		if len(parts) != 2 {
			err = fmt.Errorf("%w: %s", ErrUnknownFormat, formatID)

			return
		}

		vformat, aformat := parts[0], parts[1]

		tgt = strings.TrimPrefix(vformat, "archive_") + "-" + strings.TrimPrefix(aformat, "archive_")
	}

	return
}

func findFormatSize(formats []*Format, tgt string, dsize uint64) (aformatid, vformatid string, idx int) {
	for i := len(formats) - 1; i >= 0; i-- {
		f := formats[i]

		switch {
		case tgt != "":
			if f.ID == tgt {
				aformatid, vformatid = f.Audio.ID, f.Video.ID
				idx = i

				return
			}
		default:
			if f.SizeEstimate() < dsize {
				aformatid, vformatid = f.Audio.ID, f.Video.ID

				idx = i

				return
			}
		}
	}

	return
}

// SelectFormat suitable to formatid selector
func SelectFormat(
	formats []*Format,
	formatID string,
) (aformatid, vformatid string, idx int, err error) {
	dsize, wildcard, tgt, err := parseFormatSize(formatID)
	if err != nil {
		return
	}

	aformatid, vformatid, idx = findFormatSize(formats, tgt, dsize)

	if tgt == "" && aformatid == "" && vformatid == "" && len(formats) > 0 {
		f := formats[0]

		if !wildcard {
			err = &ErrFormatSuggest{
				Format: f,
			}

			return
		}

		aformatid, vformatid = f.Audio.ID, f.Video.ID
		idx = 0
	}

	if vformatid == "" || aformatid == "" {
		err = fmt.Errorf("%w: %s", ErrUnknownFormat, formatID)
	}

	return
}
