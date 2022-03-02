// Package config provides persistent configuration for fedipost instances
package config

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/eientei/jaroid/fedipost"
	"golang.org/x/oauth2"
	yaml "gopkg.in/yaml.v2"
)

// OAuth2OOBRedirectURI special out of band redirect URI for displaying authorization code to user instead of
// redirecting
const OAuth2OOBRedirectURI = "urn:ietf:wg:oauth:2.0:oob"

// DefaultScopes for oauth client/tokens
var DefaultScopes = []string{"write:statuses", "write:media"}

// DefaultTemplate is a default post template. See search.ThumbItem for .info fields
const DefaultTemplate = `<b>{{.info.Title}}</b>
{{- if .info.Tags.jp }}
{{ .info.Tags.jp | makeTags | join " " }}
{{- end }}
{{.url}}
`

// DefaultUseragent for config
const DefaultUseragent = "jaroid"

// Root represents root of configuration file
type Root struct {
	Instances    map[string]*Instance `yaml:"instances"`
	Global       Global               `yaml:"global"`
	Mediaservice Mediaservice         `yaml:"mediaservice"`
}

// MediaserviceAuth authentication detauls
type MediaserviceAuth struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// Mediaservice contains currently selected media service and configuration details for each
type Mediaservice struct {
	Auth      MediaserviceAuth `yaml:"auth"`
	SaveDir   string           `yaml:"save_dir"`
	CookieJar string           `yaml:"cookie_jar"`
	KeepFiles bool             `yaml:"keep_files"`
}

// Load loads config
func (r *Root) Load(reader io.Reader) error {
	return yaml.NewDecoder(reader).Decode(r)
}

// LoadFile loads config from file
func (r *Root) LoadFile(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}

	defer func() {
		_ = f.Close()
	}()

	return r.Load(f)
}

// Save persists config
func (r *Root) Save(writer io.Writer) error {
	if r.Global.Template == "" {
		r.Global.Template = DefaultTemplate
	}

	if r.Global.UserAgent == "" {
		r.Global.UserAgent = DefaultUseragent
	}

	if r.Mediaservice.CookieJar == "" {
		homedir, err := os.UserHomeDir()
		if err != nil {
			return err
		}

		r.Mediaservice.CookieJar = filepath.Join(homedir, ".config", "jaroid", "cookie.jar")
	}

	return yaml.NewEncoder(writer).Encode(r)
}

// SaveFile persists config to file
func (r *Root) SaveFile(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}

	defer func() {
		_ = f.Close()
	}()

	return r.Save(f)
}

// Account returns account by instance address and login
func (r *Root) Account(addr, login string) (*Account, error) {
	inst, err := r.Instance(addr)
	if err != nil {
		return nil, err
	}

	return inst.Account(login), nil
}

// RestConfig returns fedipost config and template for given instance and accout
func (r *Root) RestConfig(
	ctx context.Context,
	addr, login, redirect string,
	refresh TokenNotifyFunc,
) (conf *fedipost.Config, template string, err error) {
	template = r.Global.Template
	useragent := r.Global.UserAgent

	inst, err := r.Instance(addr)
	if err != nil {
		return nil, "", err
	}

	if inst.Template != "" {
		template = inst.Template
	}

	if inst.UserAgent != "" {
		useragent = inst.UserAgent
	}

	acc := inst.Account(login)

	if redirect == "" {
		redirect = acc.RedirectURI
	}

	if acc.Template != "" {
		template = acc.Template
	}

	if useragent == "" {
		useragent = DefaultUseragent
	}

	if template == "" {
		template = DefaultTemplate
	}

	oauthconf := inst.OAuth2Config(redirect)

	var client *http.Client

	if acc.AccessToken != "" {
		t := &oauth2.Token{
			AccessToken:  acc.AccessToken,
			TokenType:    acc.Type,
			RefreshToken: acc.RefreshToken,
			Expiry:       acc.Expire,
		}
		ts := oauthconf.TokenSource(ctx, t)

		nts := &NotifyRefreshTokenSource{
			new: ts,
			t:   t,
			f: func(token *oauth2.Token) error {
				acc.m.Lock()
				defer acc.m.Unlock()

				acc.RedirectURIs[oauthconf.RedirectURL] = struct{}{}
				acc.RedirectURI = oauthconf.RedirectURL
				acc.AccessToken = token.AccessToken
				acc.RefreshToken = token.RefreshToken
				acc.Type = token.Type()
				acc.Expire = token.Expiry

				if refresh == nil {
					return nil
				}

				return refresh(token)
			},
			mu: sync.Mutex{},
		}

		client = oauth2.NewClient(ctx, nts)
	} else {
		client = http.DefaultClient
	}

	return &fedipost.Config{
		HTTPClient:             client,
		Host:                   inst.URL,
		MediaEndpoint:          inst.Endpoints.Media,
		StatusesEndpoint:       inst.Endpoints.Statuses,
		AppsEndpoint:           inst.Endpoints.Apps,
		OauthTokenEndpoint:     inst.Endpoints.OauthToken,
		OauthAuthorizeEndpoint: inst.Endpoints.OauthAuthorize,
		UserAgent:              useragent,
	}, template, nil
}

// Instance returns instance address
func (r *Root) Instance(s string) (*Instance, error) {
	if s == "" {
		s = r.Global.DefaultInstance
	}

	if s == "" {
		return nil, errors.New("instance does not exist: " + s)
	}

	if r.Instances == nil {
		r.Instances = make(map[string]*Instance)
	}

	if !strings.Contains(s, "://") {
		s = "https://" + s
	}

	parsed, err := url.Parse(s)
	if err != nil {
		return nil, err
	}

	inst, ok := r.Instances[parsed.Hostname()]
	if !ok {
		inst = &Instance{
			Accounts:  nil,
			URL:       s,
			Template:  "",
			UserAgent: "",
			Endpoints: Endpoints{
				Media:          parsed.String() + "/api/v1/media",
				Statuses:       parsed.String() + "/api/v1/statuses",
				Apps:           parsed.String() + "/api/v1/apps",
				OauthToken:     parsed.String() + "/oauth/token",
				OauthAuthorize: parsed.String() + "/oauth/authorize",
			},
			Clients: nil,
		}

		r.Instances[parsed.Hostname()] = inst
	}

	return inst, nil
}

// Global contains globally-applied settings to every instance as defaults
type Global struct {
	Template        string `yaml:"template,omitempty"`
	UserAgent       string `yaml:"user_agent,omitempty"`
	DefaultInstance string `yaml:"default_instance"`
}

// Endpoints contains resolved endpoint paths (or URLs) for API sections
type Endpoints struct {
	Media          string `yaml:"media,omitempty"`
	Statuses       string `yaml:"statuses,omitempty"`
	Apps           string `yaml:"apps,omitempty"`
	OauthToken     string `yaml:"oauth_token,omitempty"`
	OauthAuthorize string `yaml:"oauth_authorize,omitempty"`
}

// Instance contains specific fediverse instance details
type Instance struct {
	Accounts       map[string]*Account `yaml:"accounts,omitempty"`
	Clients        map[string]*Client  `yaml:"clients,omitempty"`
	URL            string              `yaml:"url,omitempty"`
	Template       string              `yaml:"template,omitempty"`
	UserAgent      string              `yaml:"user_agent,omitempty"`
	DefaultAccount string              `yaml:"default_account,omitempty"`
	Endpoints      Endpoints           `yaml:"endpoints,omitempty"`
}

// OAuth2Config returns oauth config for instance
func (inst *Instance) OAuth2Config(redirect string) *oauth2.Config {
	client := inst.Client(redirect)

	return &oauth2.Config{
		ClientID:     client.ClientID,
		ClientSecret: client.ClientSecret,
		Endpoint: oauth2.Endpoint{
			AuthURL:  inst.Endpoints.OauthAuthorize,
			TokenURL: inst.Endpoints.OauthToken,
		},
		RedirectURL: client.RedirectURI,
		Scopes:      client.Scopes,
	}
}

// Client contains oauth client details
type Client struct {
	ClientID     string   `yaml:"client_id,omitempty"`
	ClientSecret string   `yaml:"client_secret,omitempty"`
	RedirectURI  string   `yaml:"redirect_uri,omitempty"`
	Scopes       []string `yaml:"scopes,omitempty"`
}

// Account returns account by login
func (inst *Instance) Account(login string) *Account {
	if login == "" {
		login = inst.DefaultAccount
	}

	if inst.Accounts == nil {
		inst.Accounts = make(map[string]*Account)
	}

	acc, ok := inst.Accounts[login]
	if !ok {
		acc = &Account{}

		inst.Accounts[login] = acc
	}

	if acc.RedirectURIs == nil {
		acc.RedirectURIs = make(map[string]struct{})
	}

	return acc
}

// Client resolves oauth client by redirect url
func (inst *Instance) Client(redirect string) *Client {
	if redirect == "" {
		redirect = OAuth2OOBRedirectURI
	}

	if inst.Clients == nil {
		inst.Clients = make(map[string]*Client)
	}

	c, ok := inst.Clients[redirect]
	if !ok {
		c = &Client{
			ClientID:     "",
			ClientSecret: "",
			RedirectURI:  redirect,
			Scopes:       DefaultScopes,
		}

		inst.Clients[redirect] = c
	}

	return c
}

// Account contains specific fediverse instance account details
type Account struct {
	Expire       time.Time           `yaml:"expire,omitempty"`
	AccessToken  string              `yaml:"access_token,omitempty"`
	RefreshToken string              `yaml:"refresh_token,omitempty"`
	Template     string              `yaml:"template,omitempty"`
	Type         string              `yaml:"type,omitempty"`
	RedirectURI  string              `yaml:"redirect_uri"`
	RedirectURIs map[string]struct{} `yaml:"redirect_uris,omitempty"`
	Scopes       []string            `yaml:"scopes,omitempty"`
	m            sync.Mutex          `yaml:"-"`
}

// TokenNotifyFunc is a function that accepts an oauth2 Token upon refresh, and
// returns an error if it should not be used.
type TokenNotifyFunc func(*oauth2.Token) error

// NotifyRefreshTokenSource is essentially `oauth2.ResuseTokenSource` with `TokenNotifyFunc` added.
type NotifyRefreshTokenSource struct {
	new oauth2.TokenSource
	t   *oauth2.Token
	f   TokenNotifyFunc
	mu  sync.Mutex
}

// Token returns the current token if it's still valid, else will
// refresh the current token (using r.Context for HTTP client
// information) and return the new one.
func (s *NotifyRefreshTokenSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.t.Valid() {
		return s.t, nil
	}

	t, err := s.new.Token()
	if err != nil {
		return nil, err
	}

	s.t = t

	if s.f == nil {
		return t, nil
	}

	return t, s.f(t)
}
