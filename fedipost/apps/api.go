// Package apps provides methods for oauth apps API
package apps

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/eientei/jaroid/fedipost"
)

// ErrInvalidToken is returned when supplied token is invalid
var ErrInvalidToken = errors.New("invalid token")

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
func Create(ctx context.Context, config *fedipost.Config, appconf *AppConfig) (*App, error) {
	values := url.Values{}

	values.Set("client_name", appconf.ClientName)
	values["redirect_uris"] = appconf.RedirectURIs
	values.Set("scopes", strings.Join(appconf.Scopes, " "))

	encoded := values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.AppsEndpoint, strings.NewReader(encoded))
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

// Verify performs client token verification
func Verify(ctx context.Context, config *fedipost.Config, clientToken string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, config.AppsVerifyEndpoint, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+clientToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode/100 != 2 {
		return ErrInvalidToken
	}

	return nil
}
