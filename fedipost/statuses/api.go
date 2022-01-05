// Package statuses provides methods for statuses API
package statuses

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	"github.com/eientei/jaroid/fedipost"
)

// CreateStatus represents fediverse status (post) creation parameters
type CreateStatus struct {
	Status      string   `json:"status,omitempty"`
	ContentType string   `json:"content_type,omitempty"`
	InReplyToID string   `json:"in_reply_to_id,omitempty"`
	MediaIDs    []string `json:"media_ids,omitempty"`
}

// CreatedStatus represents created status
type CreatedStatus struct {
	Body  string `json:"-"`
	ID    string `json:"id"`
	URL   string `json:"url"`
	Error string `json:"error"`
}

var symregex = regexp.MustCompile(`[^\pL\pN_]`)

// MakeTag returns corrsponding tag for porvided string
func MakeTag(s string) string {
	return "#" + symregex.ReplaceAllString(s, "")
}

// Create creates a new status
func Create(config *fedipost.Config, status *CreateStatus) (*CreatedStatus, error) {
	var b bytes.Buffer

	err := json.NewEncoder(&b).Encode(status)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, config.StatusesEndpoint, &b)
	if err != nil {
		return nil, err
	}

	req.Header.Set("content-type", "application/json")

	resp, err := config.Exchange(req, true)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	created := &CreatedStatus{}

	err = json.NewDecoder(resp.Body).Decode(created)
	if err != nil {
		return nil, err
	}

	if created.Error != "" {
		return nil, errors.New(created.Error)
	}

	return created, nil
}
