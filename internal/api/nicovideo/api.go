// Package nicovideo provides nicovideo API client
package nicovideo

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseVideoURI = "https://api.search.nicovideo.jp/api/v2/video/contents/search"

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

// New creates new API client
func New() *Client {
	return &Client{
		HTTP:    &http.Client{},
		BaseURI: baseVideoURI,
		Headers: http.Header{
			"user-agent": []string{"goclient/0.1 golang nicovideo api client"},
		},
		Context: "goclient-" + strconv.FormatUint(uint64(rand.Uint32()), 10),
	}
}

// Client implements nicovideo API client
type Client struct {
	HTTP    *http.Client
	BaseURI string
	Headers http.Header
	Context string
}

// Filter search
type Filter struct {
	Field    Field
	Operator Operator
	Values   []string
}

// Search query
type Search struct {
	Fields  []Field  // Return fields
	Filters []Filter // Query filters
	Offset  int      // Offset in entries
	Limit   int      // Limit in entries
}

// SearchResult from search
type SearchResult struct {
	Meta struct {
		Status       int    `json:"status"`
		ErrorCode    string `json:"errorCode"`
		ErrorMessage string `json:"errorMessage"`
		TotalCount   int    `json:"totalCount"`
		ID           string `json:"id"`
	} `json:"meta"`
	Data []*SearchItem `json:"data"`
}

// SearchItem from search
type SearchItem struct {
	SearchItemRaw

	StartTime       time.Time
	LastCommentTime time.Time
	CategoryTags    []string
	Tags            []string
	TagsExact       []string
	LockTagsExact   []string
}

// SearchItemRaw used for json response decoding
type SearchItemRaw struct {
	ContentID       string `json:"contentId"`
	Title           string `json:"title"`
	Description     string `json:"description"`
	UserID          int    `json:"userId"`
	ViewCounter     int    `json:"viewCounter"`
	MylistCounter   int    `json:"mylistCounter"`
	LengthSeconds   int    `json:"lengthSeconds"`
	ThumbnailURL    string `json:"thumbnailUrl"`
	StartTime       string `json:"startTime"`
	ThreadID        int    `json:"threadId"`
	CommentCounter  int    `json:"commentCounter"`
	LastCommentTime string `json:"lastCommentTime"`
	CategoryTags    string `json:"categoryTags"`
	ChannelID       int    `json:"channelId"`
	Tags            string `json:"tags"`
	TagsExact       string `json:"tagsExact"`
	LockTagsExact   string `json:"lockTagsExact"`
	Genre           string `json:"genre"`
	GenreKeyword    string `json:"genre.keyword"`
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

func (client *Client) postprocessSearch(res *SearchResult) {
	for _, d := range res.Data {
		if d.SearchItemRaw.StartTime != "" {
			d.StartTime, _ = time.Parse(time.RFC3339, d.SearchItemRaw.StartTime)
		}

		if d.SearchItemRaw.LastCommentTime != "" {
			d.LastCommentTime, _ = time.Parse(time.RFC3339, d.SearchItemRaw.LastCommentTime)
		}

		if d.SearchItemRaw.Tags != "" {
			d.Tags = strings.Split(d.SearchItemRaw.Tags, " ")
		}

		if d.SearchItemRaw.TagsExact != "" {
			d.TagsExact = strings.Split(d.SearchItemRaw.TagsExact, " ")
		}

		if d.SearchItemRaw.CategoryTags != "" {
			d.CategoryTags = strings.Split(d.SearchItemRaw.CategoryTags, " ")
		}

		if d.SearchItemRaw.LockTagsExact != "" {
			d.LockTagsExact = strings.Split(d.SearchItemRaw.LockTagsExact, " ")
		}
	}
}

// Search using given options
func (client *Client) Search(
	query string,
	sortField Field,
	sortDirection SortDirection,
	targets []Field,
	opts *Search,
) (res *SearchResult, err error) {
	values := &url.Values{}
	values.Set("q", query)
	values.Set("targets", strings.Join(fields(targets), ","))
	values.Set("_sort", string(sortDirection)+string(sortField))

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

	req, err := http.NewRequest(http.MethodGet, client.BaseURI+"?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}

	req.Header = client.Headers

	resp, err := client.HTTP.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() {
		if e := resp.Body.Close(); err == nil {
			err = e
		}
	}()

	res = &SearchResult{}

	err = json.NewDecoder(resp.Body).Decode(res)
	if err != nil {
		return nil, err
	}

	client.postprocessSearch(res)

	return
}
