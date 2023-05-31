// Package app provides common implementation for fedipost apps
package app

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/eientei/cookiejarx"
	"github.com/eientei/jaroid/fedipost"
	"github.com/eientei/jaroid/fedipost/apps"
	"github.com/eientei/jaroid/fedipost/config"
	"github.com/eientei/jaroid/fedipost/statuses"
	"github.com/eientei/jaroid/integration/nicovideo"
	"github.com/eientei/jaroid/nicopost"
	"github.com/eientei/jaroid/util/httputil/middleware"
	"golang.org/x/oauth2"
)

// Fedipost contains fedipost app state
type Fedipost struct {
	Config         *config.Root
	Client         *nicovideo.Client
	FedipostConfig *fedipost.Config
	Template       string
	ConfigLocation string
}

// New returns new fedipost app with given config path
func New(configpath string, overrides func(fp *Fedipost)) (*Fedipost, error) {
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
	}

	err := f.Reload(overrides)
	if err != nil {
		return nil, err
	}

	return f, nil
}

// Reload fedipost from config
func (f *Fedipost) Reload(overrides func(fp *Fedipost)) error {
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

	overrides(f)

	err = os.MkdirAll(filepath.Dir(f.Config.Mediaservice.CookieJar), 0777)
	if err != nil {
		return err
	}

	var auth *nicovideo.Auth

	if f.Config.Mediaservice.Auth.Username != "" && f.Config.Mediaservice.Auth.Password != "" {
		auth = &nicovideo.Auth{
			Username: f.Config.Mediaservice.Auth.Username,
			Password: f.Config.Mediaservice.Auth.Password,
		}
	}

	storage := cookiejarx.NewInMemoryStorage()

	jar, err := cookiejarx.New(&cookiejarx.Options{
		Storage: storage,
	})
	if err != nil {
		panic(err)
	}

	cookiejarfile := &middleware.ClientCookieJarFile{
		Storage:  storage,
		Jar:      jar,
		FilePath: f.Config.Mediaservice.CookieJar,
	}

	nicovideoClient := &http.Client{
		Transport: &middleware.Transport{
			Transport:   http.DefaultTransport,
			Middlewares: []middleware.Client{cookiejarfile},
		},
	}

	f.Client = nicovideo.New(&nicovideo.Config{
		HTTPClient: nicovideoClient,
		Auth:       auth,
	})

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

// MakeAccountAuthorization returns authorization URL for user to follow
func (f *Fedipost) MakeAccountAuthorization(ctx context.Context, instance, login, redirect string) (string, error) {
	inst, err := f.Config.Instance(instance)
	if err != nil {
		return "", err
	}

	conf, _, err := f.login(ctx, instance, login, redirect)
	if err != nil {
		return "", err
	}

	client := inst.Client(redirect)

	err = f.ensureClient(ctx, inst, conf, client)
	if err != nil {
		return "", err
	}

	oauthconf := inst.OAuth2Config(redirect)

	return oauthconf.AuthCodeURL(""), nil
}

func (f *Fedipost) createClientApp(ctx context.Context, conf *fedipost.Config, client *config.Client) error {
	app, err := apps.Create(ctx, conf, &apps.AppConfig{
		ClientName:   "jaroid",
		RedirectURIs: []string{client.RedirectURI},
		Scopes:       client.Scopes,
	})
	if err != nil {
		return err
	}

	client.ClientID = app.ClientID
	client.ClientSecret = app.ClientSecret
	client.Scopes = app.Scopes

	return f.Save()
}

func (f *Fedipost) createClientToken(ctx context.Context, inst *config.Instance, client *config.Client) error {
	tok, err := inst.OAuth2ClientCredentialsConfig(client).Token(ctx)
	if err != nil {
		return err
	}

	client.ClientToken = tok.AccessToken

	return f.Save()
}

func (f *Fedipost) ensureClientToken(
	ctx context.Context,
	inst *config.Instance,
	conf *fedipost.Config,
	client *config.Client,
) error {
	if client.ClientToken != "" {
		return nil
	}

	err := f.createClientToken(ctx, inst, client)
	if err != nil {
		err = f.createClientApp(ctx, conf, client)
		if err != nil {
			return err
		}

		err = f.createClientToken(ctx, inst, client)
	}

	return err
}

func (f *Fedipost) ensureClient(
	ctx context.Context,
	inst *config.Instance,
	conf *fedipost.Config,
	client *config.Client,
) error {
	if client.ClientID == "" || client.ClientSecret == "" {
		err := f.createClientApp(ctx, conf, client)
		if err != nil {
			return err
		}
	}

	err := f.ensureClientToken(ctx, inst, conf, client)
	if err != nil {
		return err
	}

	err = apps.Verify(ctx, conf, client.ClientToken)
	if err != nil {
		client.ClientToken = ""

		err = f.ensureClientToken(ctx, inst, conf, client)
		if err != nil {
			return err
		}

		err = apps.Verify(ctx, conf, client.ClientToken)
	}

	return err
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
	preview bool,
) (*statuses.CreatedStatus, error) {
	conf, tmpl, err := f.login(ctx, uri, login, "")
	if err != nil {
		return nil, err
	}

	status, err := nicopost.MakeNicovideoStatus(ctx, conf, f.Client, videouri, videopath, tmpl, preview)
	if err != nil {
		return nil, err
	}

	if preview {
		return &statuses.CreatedStatus{
			Body:  status.Status,
			ID:    "",
			URL:   "",
			Error: "",
		}, nil
	}

	return statuses.Create(conf, status)
}
