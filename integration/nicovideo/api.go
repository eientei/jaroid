// Package nicovideo provides nicovideo search API client
package nicovideo

import (
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/eientei/jaroid/util/httputil/middleware"
)

const (
	baseVideoURI = "https://snapshot.search.nicovideo.jp/api/v2/snapshot/video/contents/search"
	thumbURI     = "https://ext.nicovideo.jp/api/getthumbinfo/"
	loginURI     = "https://account.nicovideo.jp/api/v1/login"
)

// Auth provides nicovideo credentials to log in with
type Auth struct {
	Username string
	Password string
	invalid  bool
}

// Config provides configuration for nicovideo api client
type Config struct {
	HTTPClient *http.Client
	Auth       *Auth
	BaseURI    string
	ThumbURI   string
	LoginURI   string
	Context    string
}

// New creates new API client
func New(cfg *Config) *Client {
	if cfg == nil {
		cfg = &Config{
			Context: "goclient-" + strconv.FormatUint(uint64(rand.Uint32()), 10),
		}
	}

	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Transport: &middleware.Transport{
			Middlewares: []middleware.Client{
				&middleware.ClientStaticHeaders{
					Set: map[string][]string{
						"user-agent": {"goclient/0.1 golang nicovideo api client"},
					},
				},
			},
			Transport: http.DefaultTransport,
		},
		}
	}

	if cfg.BaseURI == "" {
		cfg.BaseURI = baseVideoURI
	}

	if cfg.ThumbURI == "" {
		cfg.ThumbURI = thumbURI
	}

	if cfg.LoginURI == "" {
		cfg.LoginURI = loginURI
	}

	return &Client{
		Config: *cfg,
	}
}

// Client implements nicovideo API client
type Client struct {
	Config
}

func (client *Client) openCopyFile(f *os.File, outpath, fmtname string, reuse bool) error {
	if !reuse {
		return nil
	}

	parts := strings.SplitN(filepath.Base(outpath), "-", 2)

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(outpath), parts[0]+"-*-"+fmtname+"*"))
	if err != nil {
		return err
	}

	var src string

	for _, m := range matches {
		if m != outpath && m != outpath+".part" && strings.Contains(m, ".mp4") {
			src = m

			break
		}
	}

	if src == "" {
		return nil
	}

	sf, err := os.Open(src)
	if err != nil {
		return err
	}

	defer func() {
		_ = sf.Close()
	}()

	_, err = io.Copy(f, sf)
	if err != nil {
		return err
	}

	return nil
}
