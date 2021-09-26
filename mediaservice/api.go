// Package mediaservice provides general API for external service downloa/search functions
package mediaservice

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	// ErrUnknownFormat is returned when unknown media format is selected
	ErrUnknownFormat = errors.New("unknown format")
)

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
}

// SaveOption functional option
type SaveOption interface {
}

// SaveOptionSubs save subs when downloading
type SaveOptionSubs struct {
	Lang string
}

// Downloader provides list of available formats and performs download of selected format
type Downloader interface {
	ListFormats(
		ctx context.Context,
		url string,
		reporter Reporter,
	) ([]*Format, error)

	SaveFormat(
		ctx context.Context,
		url, formatID, outpath string,
		reporter Reporter,
		opts ...SaveOption,
	) ([]string, error)
}

var numRegexp = regexp.MustCompile(`^\s*([0-9.]+)(\S*)\s*$`)

// MatchesHumanSize returns true if given string conforms to human size format
func MatchesHumanSize(s string) bool {
	return numRegexp.MatchString(s)
}

// HumanSizeParse parses human-formatted size as bytes
func HumanSizeParse(s string) uint64 {
	parts := numRegexp.FindStringSubmatch(s)
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
