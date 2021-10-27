package nicovideo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Field of content entry
type Field string

// Known fields
const (
	FieldContentID       Field = "contentId"
	FieldTitle           Field = "title"
	FieldDescription     Field = "description"
	FieldUserID          Field = "userId"
	FieldViewCounter     Field = "viewCounter"
	FieldMylistCounter   Field = "mylistCounter"
	FieldLengthSeconds   Field = "lengthSeconds"
	FieldThumbnailURL    Field = "thumbnailUrl"
	FieldStartTime       Field = "startTime"
	FieldThreadID        Field = "threadID"
	FieldCommentCounter  Field = "commentCounter"
	FieldLastCommentTime Field = "lastCommentTime"
	FieldCategoryTags    Field = "categoryTags"
	FieldChannelID       Field = "channelId"
	FieldTags            Field = "tags"
	FieldTagsExact       Field = "tagsExact"
	FieldLockTagsExact   Field = "lockTagsExact"
	FieldGenre           Field = "genre"
	FieldGenreKeyword    Field = "genre.keyword"
)

// SortDirection to sort entries
type SortDirection string

// Known sort directions
const (
	SortAsc  SortDirection = "+"
	SortDesc SortDirection = "-"
)

// Sort configuration
type Sort struct {
	Direction SortDirection
	Field     Field
}

// Operator for field
type Operator string

// Known operators
const (
	OperatorGTE   Operator = "gte"   // translates to F > values[0]
	OperatorLTE   Operator = "lte"   // translates to F < values[0]
	OperatorEqual Operator = "equal" // translates to F = values[N]
	OperatorRange Operator = "range" // translates to F > values[0] && F < values[1]
)

// Filter search
type Filter struct {
	Field    Field    `json:"field"`
	Operator Operator `json:"operator"`
	Values   []string `json:"values"`
}

// Search query
type Search struct {
	Query         string        `json:"query"`          // Search query
	SortField     Field         `json:"sort_field"`     // Sort field
	SortDirection SortDirection `json:"sort_direction"` // Sort directions
	Targets       []Field       `json:"targets"`        // Targets to search in
	Fields        []Field       `json:"fields"`         // Return fields
	Filters       []Filter      `json:"filters"`        // Query filters
	Offset        int           `json:"offset"`         // Offset in entries
	Limit         int           `json:"limit"`          // Limit in entries
}

// Result from search
type Result struct {
	Data []*Item `json:"data"`
	Meta struct {
		ErrorCode    string `json:"errorCode"`
		ErrorMessage string `json:"errorMessage"`
		ID           string `json:"id"`
		Status       int    `json:"status"`
		TotalCount   int    `json:"totalCount"`
	} `json:"meta"`
}

// Item from search
type Item struct {
	StartTime       time.Time
	LastCommentTime time.Time
	CategoryTags    []string
	Tags            []string
	TagsExact       []string
	LockTagsExact   []string
	ItemRaw
}

// ItemRaw used for json response decoding
type ItemRaw struct {
	ContentID       string `json:"contentId"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	ThumbnailURL    string `json:"thumbnailUrl"`
	StartTime       string `json:"startTime"`
	LastCommentTime string `json:"lastCommentTime"`
	CategoryTags    string `json:"categoryTags"`
	Tags            string `json:"tags"`
	TagsExact       string `json:"tagsExact"`
	LockTagsExact   string `json:"lockTagsExact"`
	Genre           string `json:"genre"`
	GenreKeyword    string `json:"genre.keyword"`
	UserID          int    `json:"userId"`
	ViewCounter     int    `json:"viewCounter"`
	MylistCounter   int    `json:"mylistCounter"`
	LengthSeconds   int    `json:"lengthSeconds"`
	ThreadID        int    `json:"threadId"`
	CommentCounter  int    `json:"commentCounter"`
	ChannelID       int    `json:"channelId"`
}

func fields(fs []Field) (ss []string) {
	ss = make([]string, len(fs))
	for i, f := range fs {
		ss[i] = string(f)
	}

	return
}

func (client *Client) filters(values *url.Values, filters []Filter) {
	for _, f := range filters {
		switch f.Operator {
		case OperatorEqual:
			for i, v := range f.Values {
				values.Add(fmt.Sprintf("filters[%s][%d]", f.Field, i), v)
			}
		case OperatorRange:
		loop:
			for i, v := range f.Values {
				switch i {
				case 0:
					values.Add(fmt.Sprintf("filters[%s][gte]", f.Field), v)
				case 1:
					values.Add(fmt.Sprintf("filters[%s][lte]", f.Field), v)
				default:
					break loop
				}
			}
		case OperatorGTE:
			for _, v := range f.Values {
				values.Add(fmt.Sprintf("filters[%s][gte]", f.Field), v)
				break
			}
		case OperatorLTE:
			for _, v := range f.Values {
				values.Add(fmt.Sprintf("filters[%s][lte]", f.Field), v)
				break
			}
		}
	}
}

func (client *Client) postprocessSearch(res *Result) {
	for _, d := range res.Data {
		if d.ItemRaw.StartTime != "" {
			d.StartTime, _ = time.Parse(time.RFC3339, d.ItemRaw.StartTime)
		}

		if d.ItemRaw.LastCommentTime != "" {
			d.LastCommentTime, _ = time.Parse(time.RFC3339, d.ItemRaw.LastCommentTime)
		}

		if d.ItemRaw.Tags != "" {
			d.Tags = strings.Split(d.ItemRaw.Tags, " ")
		}

		if d.ItemRaw.TagsExact != "" {
			d.TagsExact = strings.Split(d.ItemRaw.TagsExact, " ")
		}

		if d.ItemRaw.CategoryTags != "" {
			d.CategoryTags = strings.Split(d.ItemRaw.CategoryTags, " ")
		}

		if d.ItemRaw.LockTagsExact != "" {
			d.LockTagsExact = strings.Split(d.ItemRaw.LockTagsExact, " ")
		}
	}
}

// Search using given options
func (client *Client) Search(ctx context.Context, opts *Search) (res *Result, err error) {
	values := &url.Values{}
	values.Set("q", opts.Query)
	values.Set("targets", strings.Join(fields(opts.Targets), ","))
	values.Set("_sort", string(opts.SortDirection)+string(opts.SortField))

	if opts != nil {
		if opts.Offset > 0 {
			values.Set("_offset", strconv.FormatInt(int64(opts.Offset), 10))
		}

		if opts.Limit > 0 {
			values.Set("_limit", strconv.FormatInt(int64(opts.Limit), 10))
		}

		if len(opts.Fields) > 0 {
			values.Set("fields", strings.Join(fields(opts.Fields), ","))
		}

		client.filters(values, opts.Filters)
	}

	if client.Context != "" {
		values.Set("_context", client.Context)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, client.BaseURI+"?"+values.Encode(), nil)
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

	res = &Result{}

	err = json.NewDecoder(resp.Body).Decode(res)
	if err != nil {
		return nil, err
	}

	client.postprocessSearch(res)

	return
}
