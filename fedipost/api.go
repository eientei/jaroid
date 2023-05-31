// Package fedipost provides interface pleroma/mastodon-style API fediverse instance
package fedipost

import (
	"net/http"
)

const version = "1.0"

// Config for instance
type Config struct {
	HTTPClient             *http.Client
	Host                   string
	MediaEndpoint          string
	StatusesEndpoint       string
	AppsEndpoint           string
	AppsVerifyEndpoint     string
	OauthTokenEndpoint     string
	OauthAuthorizeEndpoint string
	UserAgent              string
}

// UserAgentValue returns useragent header value
func (c *Config) UserAgentValue() string {
	if c.UserAgent == "" || c.UserAgent == "jaroid" {
		return "jaroid/" + version
	}

	return c.UserAgent
}

// Exchange http request for http response
func (c *Config) Exchange(r *http.Request, auth bool) (*http.Response, error) {
	client := c.HTTPClient

	if client == nil {
		client = http.DefaultClient
	}

	r.Header.Set("user-agent", c.UserAgentValue())

	return client.Do(r)
}
