package middleware

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/eientei/cookiejarx"
)

type entry struct {
	Expires    time.Time `json:"expires,omitempty"`
	Creation   time.Time `json:"creation,omitempty"`
	LastAccess time.Time `json:"last_access,omitempty"`
	Name       string    `json:"name,omitempty"`
	Value      string    `json:"value,omitempty"`
	Domain     string    `json:"domain,omitempty"`
	Path       string    `json:"path,omitempty"`
	SameSite   string    `json:"same_site,omitempty"`
	Key        string    `json:"key,omitempty"`
	ID         string    `json:"id,omitempty"`
	Secure     bool      `json:"secure,omitempty"`
	HTTPOnly   bool      `json:"http_only,omitempty"`
	Persistent bool      `json:"persistent,omitempty"`
	HostOnly   bool      `json:"host_only,omitempty"`
}

// ClientCookieJarFile implements persistent cookie jar file using cookiejarx.InMemoryStorage
type ClientCookieJarFile struct {
	Storage  *cookiejarx.InMemoryStorage
	Jar      *cookiejarx.Jar
	FilePath string
	mu       sync.Mutex
	loaded   bool
}

// Preprocess implementation, loads cookie jar file on first request
func (c *ClientCookieJarFile) Preprocess(req *http.Request) (*http.Request, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.loaded {
		err := c.load()
		if err != nil {
			return nil, err
		}

		c.loaded = true
	}

	for _, cookie := range c.Jar.Cookies(req.URL) {
		req.AddCookie(cookie)
	}

	return req, nil
}

// Postprocess implementation, dumps all cookies to file after response
func (c *ClientCookieJarFile) Postprocess(resp *http.Response) (*http.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Jar.SetCookies(resp.Request.URL, resp.Cookies())

	err := c.save()
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (c *ClientCookieJarFile) load() error {
	bs, err := ioutil.ReadFile(c.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	var entries []entry

	err = json.Unmarshal(bs, &entries)
	if err != nil {
		return err
	}

	var mementries []*cookiejarx.Entry

	for _, e := range entries {
		mementries = append(mementries, &cookiejarx.Entry{
			Name:       e.Name,
			Value:      e.Value,
			Domain:     e.Domain,
			Path:       e.Path,
			SameSite:   e.SameSite,
			Key:        e.Key,
			ID:         e.ID,
			Secure:     e.Secure,
			HttpOnly:   e.HTTPOnly,
			Persistent: e.Persistent,
			HostOnly:   e.HostOnly,
			Expires:    e.Expires,
			Creation:   e.Creation,
			LastAccess: e.LastAccess,
		})
	}

	c.Storage.EntriesClear()
	c.Storage.EntriesRestore(mementries)

	return nil
}

func (c *ClientCookieJarFile) save() error {
	entries := make([]entry, 0)

	for _, e := range c.Storage.EntriesDump() {
		if !e.Persistent {
			continue
		}

		entries = append(entries, entry{
			Name:       e.Name,
			Value:      e.Value,
			Domain:     e.Domain,
			Path:       e.Path,
			SameSite:   e.SameSite,
			Key:        e.Key,
			ID:         e.ID,
			Secure:     e.Secure,
			HTTPOnly:   e.HttpOnly,
			Persistent: e.Persistent,
			HostOnly:   e.HostOnly,
			Expires:    e.Expires,
			Creation:   e.Creation,
			LastAccess: e.LastAccess,
		})
	}

	bs, err := json.Marshal(entries)
	if err != nil {
		return err
	}

	return os.WriteFile(c.FilePath, bs, 0666)
}
