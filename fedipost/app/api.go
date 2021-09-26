// Package app provides common implementation for fedipost apps
package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/eientei/jaroid/fedipost"

	"github.com/eientei/jaroid/fedipost/apps"
	"github.com/eientei/jaroid/fedipost/config"
	"github.com/eientei/jaroid/fedipost/statuses"
	"github.com/eientei/jaroid/nicopost"
	"github.com/eientei/jaroid/nicovideo/search"
	"golang.org/x/oauth2"
)

// Fedipost contains fedipost app state
type Fedipost struct {
	Config         *config.Root
	Client         *search.Client
	FedipostConfig *fedipost.Config
	Template       string
	ConfigLocation string
}

// New returns new fedipost app with given config path
func New(configpath string) (*Fedipost, error) {
	if configpath == "" {
		homedir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}

		configpath = filepath.Join(homedir, ".config", "jaroid", "fedipost.yml")
	}

	f := &Fedipost{
		ConfigLocation: configpath,
		Config:         &config.Root{},
		Client:         search.New(),
	}

	err := f.Reload()
	if err != nil {
		return nil, err
	}

	return f, nil
}

// Reload fedipost from config
func (f *Fedipost) Reload() error {
	err := f.Config.LoadFile(f.ConfigLocation)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}

	if err != nil {
		return err
	}

	err = f.Save()
	if err != nil {
		return err
	}

	return nil
}

// Save fedipost config
func (f *Fedipost) Save() error {
	err := os.MkdirAll(filepath.Dir(f.ConfigLocation), 0755)
	if err != nil {
		return err
	}

	err = f.Config.SaveFile(f.ConfigLocation)
	if err != nil {
		return err
	}

	return nil
}

// MakeAccoutAuthorization returns authorization URL for user to follow
func (f *Fedipost) MakeAccoutAuthorization(ctx context.Context, instance, login, redirect string) (string, error) {
	inst, err := f.Config.Instance(instance)
	if err != nil {
		return "", err
	}

	conf, _, err := f.login(ctx, instance, login, redirect)
	if err != nil {
		return "", err
	}

	client := inst.Client(redirect)

	if client.ClientID == "" || client.ClientSecret == "" {
		var app *apps.App

		app, err = apps.Create(conf, &apps.AppConfig{
			ClientName:   "jaroid",
			RedirectURIs: []string{client.RedirectURI},
			Scopes:       client.Scopes,
		})
		if err != nil {
			return "", err
		}

		client.ClientID = app.ClientID
		client.ClientSecret = app.ClientSecret
		client.Scopes = app.Scopes

		err = f.Save()
		if err != nil {
			return "", err
		}
	}

	oauthconf := inst.OAuth2Config(redirect)

	return oauthconf.AuthCodeURL(""), nil
}

func (f *Fedipost) login(ctx context.Context, uri, login, redirect string) (*fedipost.Config, string, error) {
	return f.Config.RestConfig(ctx, uri, login, redirect, func(token *oauth2.Token) error {
		return f.Save()
	})
}

// ExchangeAuthorizeCode exchanges authorization code for access/refresh tokens
func (f *Fedipost) ExchangeAuthorizeCode(ctx context.Context, instance, login, redirect, code string) error {
	inst, err := f.Config.Instance(instance)
	if err != nil {
		return nil
	}

	oauthconf := inst.OAuth2Config(redirect)

	token, err := oauthconf.Exchange(ctx, code)
	if err != nil {
		return err
	}

	acc := inst.Account(login)

	acc.RedirectURIs[oauthconf.RedirectURL] = struct{}{}
	acc.RedirectURI = oauthconf.RedirectURL
	acc.AccessToken = token.AccessToken
	acc.RefreshToken = token.RefreshToken
	acc.Scopes = oauthconf.Scopes
	acc.Expire = token.Expiry
	acc.Type = token.Type()

	if f.Config.Global.DefaultInstance == "" {
		f.Config.Global.DefaultInstance = inst.URL
	}

	if inst.DefaultAccount == "" {
		inst.DefaultAccount = login
	}

	return f.Save()
}

// MakeStatus creates new fediverse status
func (f *Fedipost) MakeStatus(
	ctx context.Context,
	uri, login, videouri, videopath string,
) (*statuses.CreatedStatus, error) {
	conf, tmpl, err := f.login(ctx, uri, login, "")
	if err != nil {
		return nil, err
	}

	status, err := nicopost.MakeNicovideoStatus(conf, f.Client, videouri, videopath, tmpl)
	if err != nil {
		return nil, err
	}

	return statuses.Create(conf, status)
}
