// Package apps provides methods for oauth apps API
package apps

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/eientei/jaroid/fedipost"
)

// App contains created app details
type App struct {
	ClientID     string   `json:"client_id,omitempty"`
	ClientSecret string   `json:"client_secret,omitempty"`
	RedirectURI  string   `json:"redirect_uri,omitempty"`
	Error        string   `json:"error"`
	Scopes       []string `json:"-"`
}

// AppConfig provides parameters fro new oauth client
type AppConfig struct {
	ClientName   string
	RedirectURIs []string
	Scopes       []string
}

// Create creates new app using app config
func Create(config *fedipost.Config, appconf *AppConfig) (*App, error) {
	values := url.Values{}

	values.Set("client_name", appconf.ClientName)
	values["redirect_uris"] = appconf.RedirectURIs
	values.Set("scopes", strings.Join(appconf.Scopes, " "))

	encoded := values.Encode()

	req, err := http.NewRequest(http.MethodPost, config.AppsEndpoint, strings.NewReader(encoded))
	if err != nil {
		return nil, err
	}

	req.Header.Set("content-type", "application/x-www-form-urlencoded")

	resp, err := config.Exchange(req, false)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	app := &App{}

	err = json.NewDecoder(resp.Body).Decode(app)
	if err != nil {
		return nil, err
	}

	if app.Error != "" {
		return nil, errors.New(app.Error)
	}

	app.Scopes = appconf.Scopes

	return app, nil
}
