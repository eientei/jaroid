package nicovideo

import (
	"context"
	"encoding/xml"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ThumbItemTags represents thumbnail tag list
type ThumbItemTags struct {
	Domain string   `xml:"domain,attr"`
	Tag    []string `xml:"tag"`
}

type thumbItem struct {
	VideoID       string           `xml:"video_id"`
	Title         string           `xml:"title"`
	Description   string           `xml:"description"`
	ThumbnailURL  string           `xml:"thumbnail_url"`
	Length        string           `xml:"length"`
	MovieType     string           `xml:"movie_type"`
	LastResBody   string           `xml:"last_res_body"`
	WatchURL      string           `xml:"watch_url"`
	ThumbType     string           `xml:"thumb_type"`
	Genre         string           `xml:"genre"`
	UserID        string           `xml:"user_id"`
	UserNickname  string           `xml:"user_nickname"`
	UserIconURL   string           `xml:"user_icon_url"`
	FirstRetrieve time.Time        `xml:"first_retrieve"`
	Tags          []*ThumbItemTags `xml:"tags"`
	SizeHigh      int              `xml:"size_high"`
	SizeLow       int              `xml:"size_low"`
	ViewCounter   int              `xml:"view_counter"`
	CommentNum    int              `xml:"comment_num"`
	MylistCounter int              `xml:"mylist_counter"`
	Embeddable    bool             `xml:"embeddable"`
	NoLivePlay    bool             `xml:"no_live_play"`
}

// ThumbItem represents thumbnail item
type ThumbItem struct {
	FirstRetrieve time.Time
	Tags          map[string][]string
	VideoID       string
	Title         string
	Description   string
	ThumbnailURL  string
	MovieType     string
	LastResBody   string
	WatchURL      string
	ThumbType     string
	Genre         string
	UserID        string
	UserNickname  string
	UserIconURL   string
	Length        time.Duration
	SizeHigh      int
	SizeLow       int
	ViewCounter   int
	CommentNum    int
	MylistCounter int
	Embeddable    bool
	NoLivePlay    bool
}

func parseDuration(s string) time.Duration {
	dur := time.Duration(0)
	unit := time.Second

	parts := strings.Split(s, ":")

	for i := len(parts) - 1; i >= 0; i-- {
		v, err := strconv.ParseUint(parts[i], 10, 64)
		if err != nil {
			return 0
		}

		dur += unit * time.Duration(v)
		unit *= 60
	}

	return dur
}

// ThumbInfo returns thumbnail info for nicovideo ID
func (client *Client) ThumbInfo(ctx context.Context, id string) (res *ThumbItem, err error) {
	var dec struct {
		Thumb *thumbItem `xml:"thumb"`
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.ThumbURI+id, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		if e := resp.Body.Close(); err == nil {
			err = e
		}
	}()

	err = xml.NewDecoder(resp.Body).Decode(&dec)
	if err != nil {
		return nil, err
	}

	tags := make(map[string][]string)

	for _, t := range dec.Thumb.Tags {
		tags[t.Domain] = t.Tag
	}

	return &ThumbItem{
		VideoID:       dec.Thumb.VideoID,
		Title:         dec.Thumb.Title,
		Description:   dec.Thumb.Description,
		ThumbnailURL:  dec.Thumb.ThumbnailURL,
		Length:        parseDuration(dec.Thumb.Length),
		MovieType:     dec.Thumb.MovieType,
		LastResBody:   dec.Thumb.LastResBody,
		WatchURL:      dec.Thumb.WatchURL,
		ThumbType:     dec.Thumb.ThumbType,
		Genre:         dec.Thumb.Genre,
		UserID:        dec.Thumb.UserID,
		UserNickname:  dec.Thumb.UserNickname,
		UserIconURL:   dec.Thumb.UserIconURL,
		FirstRetrieve: dec.Thumb.FirstRetrieve,
		Tags:          tags,
		SizeHigh:      dec.Thumb.SizeHigh,
		SizeLow:       dec.Thumb.SizeLow,
		ViewCounter:   dec.Thumb.ViewCounter,
		CommentNum:    dec.Thumb.CommentNum,
		MylistCounter: dec.Thumb.MylistCounter,
		Embeddable:    dec.Thumb.Embeddable,
		NoLivePlay:    dec.Thumb.NoLivePlay,
	}, nil
}
