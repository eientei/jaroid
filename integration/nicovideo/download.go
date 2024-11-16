package nicovideo

import (
	"bufio"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eientei/jaroid/mediaservice"
)

var (
	dataAPIDataRegex = regexp.MustCompile(`name="server-response"\s+content="([^"]+)"`)
	otpRegexp        = regexp.MustCompile(`action="(/mfa[^"]*)"`)
)

// BoolYesNo formats boolean as "yes" and "no" strings during json un/marshalling
type BoolYesNo bool

// UnmarshalJSON implementation
func (d *BoolYesNo) UnmarshalJSON(bs []byte) error {
	v, err := strconv.ParseBool(string(bs))
	if err != nil {
		return err
	}

	*d = BoolYesNo(v)

	return nil
}

// MarshalJSON implementation
func (d BoolYesNo) MarshalJSON() ([]byte, error) {
	if d {
		return ([]byte)(`"yes"`), nil
	}

	return ([]byte)(`"no"`), nil
}

// DurationSeconds provides jso un/marhsalling of time.Duration as integer number of seconds
type DurationSeconds time.Duration

// UnmarshalJSON implementation
func (d *DurationSeconds) UnmarshalJSON(bs []byte) error {
	v, err := strconv.ParseInt(string(bs), 10, 64)
	if err != nil {
		return err
	}

	*d = DurationSeconds(time.Second * time.Duration(v))

	return nil
}

// MarshalJSON implementation
func (d *DurationSeconds) MarshalJSON() ([]byte, error) {
	v := int64(time.Duration(*d).Seconds())

	return ([]byte)(strconv.FormatInt(v, 10)), nil
}

// APIDataSessionURL json mapping
type APIDataSessionURL struct {
	URL             string `json:"url,omitempty"`
	IsWellKnownPort bool   `json:"isWellKnownPort,omitempty"`
	IsSSL           bool   `json:"isSsl,omitempty"`
}

// APIDataMovieSession json mapping
type APIDataMovieSession struct {
	AuthTypes         map[string]string    `json:"authTypes,omitempty"`
	RecipeID          string               `json:"recipeId,omitempty"`
	PlayerID          string               `json:"playerId,omitempty"`
	ServiceUserID     string               `json:"serviceUserId,omitempty"`
	Token             string               `json:"token,omitempty"`
	Signature         string               `json:"signature,omitempty"`
	ContentID         string               `json:"contentId,omitempty"`
	Videos            []string             `json:"videos,omitempty"`
	Audios            []string             `json:"audios,omitempty"`
	URLS              []*APIDataSessionURL `json:"urls,omitempty"`
	HeartBeatLifetime uint64               `json:"heartbeatLifetime,omitempty"`
	ContentKeyTimeout uint64               `json:"contentKeyTimeout,omitempty"`
	Priority          float64              `json:"priority,omitempty"`
}

// APIDataAudioMetadata json mapping
type APIDataAudioMetadata struct {
	Bitrate      uint64 `json:"bitrate,omitempty"`
	SamplingRate uint64 `json:"samplingRate,omitempty"`
}

// APIDataMovieAudio json mapping
type APIDataMovieAudio struct {
	ID           string               `json:"id,omitempty"`
	Metadata     APIDataAudioMetadata `json:"metadata,omitempty"`
	Bitrate      uint64               `json:"bitrate,omitempty"`
	SamplingRate uint64               `json:"samplingRate,omitempty"`
	IsAvailable  bool                 `json:"isAvailable,omitempty"`
}

// GetBitrate resolver
func (a *APIDataMovieAudio) GetBitrate() uint64 {
	if a.Bitrate != 0 {
		return a.Bitrate
	}

	return a.Metadata.Bitrate
}

// GetSamplingRate resolver
func (a *APIDataMovieAudio) GetSamplingRate() uint64 {
	if a.SamplingRate != 0 {
		return a.SamplingRate
	}

	return a.Metadata.SamplingRate
}

// APIDataVideoResolution json mapping
type APIDataVideoResolution struct {
	Width  uint64 `json:"width,omitempty"`
	Height uint64 `json:"height,omitempty"`
}

// APIDataVideoMetadata json mapping
type APIDataVideoMetadata struct {
	Label      string                 `json:"label,omitempty"`
	Bitrate    uint64                 `json:"bitrate,omitempty"`
	Resolution APIDataVideoResolution `json:"resolution,omitempty"`
}

// APIDataMovieVideo json mapping
type APIDataMovieVideo struct {
	ID          string               `json:"id,omitempty"`
	Metadata    APIDataVideoMetadata `json:"metadata,omitempty"`
	IsAvailable bool                 `json:"isAvailable,omitempty"`
	Bitrate     uint64               `json:"bitrate,omitempty"`
	Width       uint64               `json:"width,omitempty"`
	Height      uint64               `json:"height,omitempty"`
}

// GetBitrate resolver
func (a *APIDataMovieVideo) GetBitrate() uint64 {
	if a.Bitrate != 0 {
		return a.Bitrate
	}

	return a.Metadata.Bitrate
}

// GetWidth resolver
func (a *APIDataMovieVideo) GetWidth() uint64 {
	if a.Width != 0 {
		return a.Width
	}

	return a.Metadata.Resolution.Width
}

// GetHeight resolver
func (a *APIDataMovieVideo) GetHeight() uint64 {
	if a.Height != 0 {
		return a.Height
	}

	return a.Metadata.Resolution.Height
}

// APIDataMovie json mapping
type APIDataMovie struct {
	Audios  []*APIDataMovieAudio `json:"audios,omitempty"`
	Videos  []*APIDataMovieVideo `json:"videos,omitempty"`
	Session APIDataMovieSession  `json:"session,omitempty"`
}

// APIDataDelivery json mapping
type APIDataDelivery struct {
	Movie APIDataMovie `json:"movie,omitempty"`
}

// APIDataDemand json mapping
type APIDataDemand struct {
	AccessRightKey string               `json:"accessRightKey,omitempty"`
	Audios         []*APIDataMovieAudio `json:"audios,omitempty"`
	Videos         []*APIDataMovieVideo `json:"videos,omitempty"`
}

// APIClient json mapping
type APIClient struct {
	WatchID      string `json:"watchId,omitempty"`
	WatchTrackID string `json:"watchTrackId,omitempty"`
}

// APIDataMedia json mapping
type APIDataMedia struct {
	Domand   APIDataDemand   `json:"domand,omitempty"`
	Delivery APIDataDelivery `json:"delivery,omitempty"`
}

// Audios resolver
func (media *APIDataMedia) Audios() []*APIDataMovieAudio {
	if len(media.Domand.Videos) > 0 {
		return media.Domand.Audios
	}

	return media.Delivery.Movie.Audios
}

// Videos resolver
func (media *APIDataMedia) Videos() []*APIDataMovieVideo {
	if len(media.Domand.Videos) > 0 {
		return media.Domand.Videos
	}

	return media.Delivery.Movie.Videos
}

// APIDataVideo json mapping
type APIDataVideo struct {
	ID           string          `json:"id,omitempty"`
	Title        string          `json:"title,omitempty"`
	Description  string          `json:"description,omitempty"`
	RegisteredAt string          `json:"registeredAt,omitempty"`
	Duration     DurationSeconds `json:"duration,omitempty"`
}

// APIData represents subset of data-api-data video stream information required to establish a download session
type APIData struct {
	Created time.Time    `json:"-"`
	Client  APIClient    `json:"client,omitempty"`
	Video   APIDataVideo `json:"video,omitempty"`
	Media   APIDataMedia `json:"media,omitempty"`
}

// SessionRequestClientInfo json mapping
type SessionRequestClientInfo struct {
	PlayerID string `json:"player_id"`
}

// SessionRequestContentAuth json mapping
type SessionRequestContentAuth struct {
	AuthType          string `json:"auth_type"`
	ServiceID         string `json:"service_id"`
	ServiceUserID     string `json:"service_user_id"`
	ContentKeyTimeout uint64 `json:"content_key_timeout"`
}

// SessionRequestSrcMux json mapping
type SessionRequestSrcMux struct {
	AudioSrcIDs []string `json:"audio_src_ids"`
	VideoSrcIDs []string `json:"video_src_ids"`
}

// SessionRequestSrcID json mapping
type SessionRequestSrcID struct {
	SrcIDToMux SessionRequestSrcMux `json:"src_id_to_mux"`
}

// SessionRequestSrcIDSet json mapping
type SessionRequestSrcIDSet struct {
	ContentSrcIDs []SessionRequestSrcID `json:"content_src_ids"`
}

// SessionRequestHeartbeat json mapping
type SessionRequestHeartbeat struct {
	Lifetime uint64 `json:"lifetime"`
}

// SessionRequestKeepMethod json mapping
type SessionRequestKeepMethod struct {
	Heartbeat SessionRequestHeartbeat `json:"heartbeat"`
}

// SessionRequestHTTPData json mapping
type SessionRequestHTTPData struct {
	UseSSL           BoolYesNo `json:"use_ssl"`
	UseWellKnownPort BoolYesNo `json:"use_well_known_port"`
}

// SessionRequestHTTPParameters json mapping
type SessionRequestHTTPParameters struct {
	HTTPOutputDownloadParameters SessionRequestHTTPData `json:"http_output_download_parameters"`
}

// SessionRequestHTTP json mapping
type SessionRequestHTTP struct {
	Parameters SessionRequestHTTPParameters `json:"parameters"`
}

// SessionRequestProtocolParameters json mapping
type SessionRequestProtocolParameters struct {
	HTTPParameters SessionRequestHTTP `json:"http_parameters"`
}

// SessionRequestProtocol json mapping
type SessionRequestProtocol struct {
	Name       string                           `json:"name"`
	Parameters SessionRequestProtocolParameters `json:"parameters"`
}

// SessionRequestAuthSignature json mapping
type SessionRequestAuthSignature struct {
	Signature string `json:"signature"`
	Token     string `json:"token"`
}

// SessionRequestOperationAuth json mapping
type SessionRequestOperationAuth struct {
	SessionOperationAuthBySignature SessionRequestAuthSignature `json:"session_operation_auth_by_signature"`
}

// SessionRequestSession json mapping
type SessionRequestSession struct {
	ContentSrcIDSets     []SessionRequestSrcIDSet    `json:"content_src_id_sets"`
	ContentID            string                      `json:"content_id"`
	ContentType          string                      `json:"content_type"`
	ContentURI           string                      `json:"content_uri"`
	RecipeID             string                      `json:"recipe_id"`
	TimingConstraint     string                      `json:"timing_constraint"`
	ClientInfo           SessionRequestClientInfo    `json:"client_info"`
	SessionOperationAuth SessionRequestOperationAuth `json:"session_operation_auth"`
	Protocol             SessionRequestProtocol      `json:"protocol"`
	ContentAuth          SessionRequestContentAuth   `json:"content_auth"`
	KeepMethod           SessionRequestKeepMethod    `json:"keep_method"`
	Priority             float64                     `json:"priority"`
}

// SessionRequest post data to initialize or maintain a session
type SessionRequest struct {
	Session SessionRequestSession `json:"session"`
}

// SessionResponseDataSession json mapping
type SessionResponseDataSession struct {
	ID         string `json:"id"`
	ContentURI string `json:"content_uri"`
}

// SessionResponseData mapping to required SessionResponse fields
type SessionResponseData struct {
	Session SessionResponseDataSession `json:"session"`
}

// SessionResponseMeta json mapping
type SessionResponseMeta struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
}

// SessionResponse API response from SessionRequest, contains next SessionRequst data in `data` field
type SessionResponse struct {
	Meta SessionResponseMeta `json:"meta"`
	Data json.RawMessage     `json:"data"`
}

func (client *Client) methodPage(
	ctx context.Context,
	url, method string,
	body io.Reader,
	h http.Header,
) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	for k, vs := range h {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}

	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (client *Client) getPageBuf(ctx context.Context, url string, buf *[]byte) error {
	resp, err := client.methodPage(ctx, url, http.MethodGet, nil, nil)
	if err != nil {
		return err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	*buf = (*buf)[0:]

	if resp.ContentLength != 0 {
		if int64(len(*buf)) < resp.ContentLength {
			*buf = append(*buf, make([]byte, resp.ContentLength)...)
		}

		_, err = io.ReadFull(resp.Body, *buf)
		if err != nil {
			return err
		}

		*buf = (*buf)[:resp.ContentLength]

		return nil
	}

	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	*buf = append(*buf, bs...)

	return nil
}

func (client *Client) getPage(ctx context.Context, url string) ([]byte, error) {
	resp, err := client.methodPage(ctx, url, http.MethodGet, nil, nil)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	return io.ReadAll(resp.Body)
}

func (client *Client) postPage(ctx context.Context, url string, body io.Reader, h http.Header) ([]byte, error) {
	resp, err := client.methodPage(ctx, url, http.MethodPost, body, h)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	return io.ReadAll(resp.Body)
}

// CacheAuth forcibly caches authentication
func (client *Client) CacheAuth(reporter mediaservice.Reporter) error {
	_, err := client.auth(reporter)

	return err
}

func (client *Client) fetchInitPage(ctx context.Context, url string, reporter mediaservice.Reporter) ([]byte, error) {
	reporter.Submit("Downloading video metadata...", false)

	bs, err := client.getPage(ctx, url)

	anonymous := strings.Contains(string(bs), "'not_login'") ||
		strings.Contains(string(bs), "NEED_LOGIN")

	if anonymous && client.Auth != nil && !client.Auth.invalid {
		var succ bool

		succ, err = client.auth(reporter)
		if err != nil {
			return nil, err
		}

		if succ {
			bs, err = client.getPage(ctx, url)
			if err != nil {
				bs = nil
			}
		}
	}

	return bs, err
}

func (client *Client) auth(reporter mediaservice.Reporter) (bool, error) {
	reporter.Submit("Logging in...", false)

	resp, err := client.HTTPClient.PostForm(client.LoginURI, url.Values{
		"mail_tel": []string{client.Auth.Username},
		"password": []string{client.Auth.Password},
	})
	if err != nil {
		client.Auth.invalid = true

		reporter.Submit("Invalid credentials", true)

		return false, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	succ := resp.Request.URL.Query().Get("message") == ""

	if !succ {
		client.Auth.invalid = true

		reporter.Submit("Invalid credentials", true)

		return false, fmt.Errorf("invalid credentials")
	}

	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}

	if strings.Contains(string(bs), `name="otp"`) {
		return client.otpAction(string(bs), reporter)
	}

	return succ, nil
}

func (client *Client) otpAction(s string, reporter mediaservice.Reporter) (bool, error) {
	if !reporter.CanRead() {
		return false, nil
	}

	matches := otpRegexp.FindAllStringSubmatch(s, -1)

	if len(matches) == 0 {
		return false, nil
	}

	u, err := url.Parse(client.LoginURI)
	if err != nil {
		return false, nil
	}

	reporter.Submit("Nicovideo has requested one-time password to perform login.", true)
	reporter.Submit("Please check your EMail and input 6-digit code on next line:", true)

	var otp string

	for {
		otp, err = bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil {
			return false, err
		}

		otp = strings.TrimSpace(otp)

		if len(otp) == 0 {
			return false, nil
		}

		if len(otp) == 6 {
			if _, err = strconv.ParseInt(otp, 10, 64); err == nil {
				break
			}
		}

		fmt.Println("Did not recognized input, enter 6-digit code or empty string to skip")
	}

	u.Path = ""
	u.RawQuery = ""

	targetURL := u.String() + matches[0][1]

	resp, err := client.HTTPClient.PostForm(targetURL, url.Values{
		"otp":                   []string{otp},
		"loginBtn":              []string{"Login"},
		"is_mfa_trusted_device": []string{"true"},
		"device_name":           []string{"jaroid"},
	})
	if err != nil {
		client.Auth.invalid = true

		reporter.Submit("Invalid credentials", true)

		return false, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode/100 == 2 {
		return true, nil
	}

	return false, nil
}

func (client *Client) fetchAPIData(ctx context.Context, url string, reporter mediaservice.Reporter) (*APIData, error) {
	initpage, err := client.fetchInitPage(ctx, url, reporter)
	if err != nil {
		return nil, err
	}

	parts := dataAPIDataRegex.FindAllStringSubmatch(string(initpage), -1)
	if len(parts) == 0 || len(parts[0]) < 2 {
		return nil, fmt.Errorf("no api data")
	}

	var resdata struct {
		Data struct {
			Response APIData `json:"response"`
		} `json:"data"`
	}

	err = json.Unmarshal(([]byte)(html.UnescapeString(parts[0][1])), &resdata)
	if err != nil {
		return nil, err
	}

	resdata.Data.Response.Created = time.Now()

	return &resdata.Data.Response, nil
}

// ListFormats returns available media format list for api response
func (data *APIData) ListFormats() (formats []*mediaservice.Format) {
	audios := make(map[string]mediaservice.AudioFormat)

	var audioIDs []string

	for _, a := range data.Media.Audios() {
		if !a.IsAvailable {
			continue
		}

		audios[a.ID] = mediaservice.AudioFormat{
			ID:         a.ID,
			Codec:      mediaservice.AudioCodecAAC,
			Bitrate:    a.GetBitrate(),
			Samplerate: a.GetSamplingRate(),
		}

		audioIDs = append(audioIDs, a.ID)
	}

	videos := make(map[string]mediaservice.VideoFormat)

	var videoIDs []string

	for _, v := range data.Media.Videos() {
		if !v.IsAvailable {
			continue
		}

		videos[v.ID] = mediaservice.VideoFormat{
			ID:      v.ID,
			Codec:   mediaservice.VideoCodecH264,
			Bitrate: v.GetBitrate(),
			Width:   v.GetWidth(),
			Height:  v.GetHeight(),
		}

		videoIDs = append(videoIDs, v.ID)
	}

	for _, v := range videoIDs {
		for _, a := range audioIDs {
			formats = append(formats, &mediaservice.Format{
				ID:        strings.TrimPrefix(v, "archive_") + "--" + strings.TrimPrefix(a, "archive_"),
				Container: mediaservice.ContainerMP4,
				Audio:     audios[a],
				Video:     videos[v],
				Duration:  time.Duration(data.Video.Duration),
			})
		}
	}

	sort.Slice(formats, func(i, j int) bool {
		fi := formats[i]
		fj := formats[j]

		return fi.Video.Bitrate+fi.Audio.Bitrate < fj.Video.Bitrate+fj.Audio.Bitrate
	})

	return
}

// ListFormats mediaserivce.Downloader implementation
func (client *Client) ListFormats(
	ctx context.Context,
	url string,
	opts *mediaservice.ListOptions,
) ([]*mediaservice.Format, error) {
	data, err := client.fetchAPIData(ctx, url, opts.GetReporter())
	if err != nil {
		return nil, err
	}

	return data.ListFormats(), nil
}

func (client *Client) createSession(
	ctx context.Context,
	session *APIDataMovieSession,
	aformatid, vformatid string,
) (sess SessionResponse, sessdata SessionResponseData, err error) {
	u, err := url.Parse(session.URLS[0].URL)
	if err != nil {
		return
	}

	q := u.Query()

	q.Set("_format", "json")

	u.RawQuery = q.Encode()

	sessiondata, err := json.Marshal(SessionRequest{
		Session: SessionRequestSession{
			ClientInfo: SessionRequestClientInfo{
				PlayerID: session.PlayerID,
			},
			ContentAuth: SessionRequestContentAuth{
				AuthType:          session.AuthTypes["http"],
				ContentKeyTimeout: session.ContentKeyTimeout,
				ServiceID:         "nicovideo",
				ServiceUserID:     session.ServiceUserID,
			},
			ContentID: session.ContentID,
			ContentSrcIDSets: []SessionRequestSrcIDSet{
				{
					ContentSrcIDs: []SessionRequestSrcID{
						{
							SrcIDToMux: SessionRequestSrcMux{
								AudioSrcIDs: []string{aformatid},
								VideoSrcIDs: []string{vformatid},
							},
						},
					},
				},
			},
			ContentType: "movie",
			ContentURI:  "",
			KeepMethod: SessionRequestKeepMethod{
				Heartbeat: SessionRequestHeartbeat{
					Lifetime: session.HeartBeatLifetime,
				},
			},
			Priority: session.Priority,
			Protocol: SessionRequestProtocol{
				Name: "http",
				Parameters: SessionRequestProtocolParameters{
					HTTPParameters: SessionRequestHTTP{
						Parameters: SessionRequestHTTPParameters{
							HTTPOutputDownloadParameters: SessionRequestHTTPData{
								UseSSL:           BoolYesNo(session.URLS[0].IsSSL),
								UseWellKnownPort: BoolYesNo(session.URLS[0].IsWellKnownPort),
							},
						},
					},
				},
			},
			RecipeID: session.RecipeID,
			SessionOperationAuth: SessionRequestOperationAuth{
				SessionOperationAuthBySignature: SessionRequestAuthSignature{
					Signature: session.Signature,
					Token:     session.Token,
				},
			},
			TimingConstraint: "unlimited",
		},
	})
	if err != nil {
		return
	}

	bs, err := client.postPage(ctx, u.String(), bytes.NewReader(sessiondata), http.Header{
		"content-type": []string{"application/json"},
	})
	if err != nil {
		return
	}

	err = json.Unmarshal(bs, &sess)
	if err != nil {
		return
	}

	if sess.Meta.Status/100 != 2 {
		err = fmt.Errorf("invalid session status(%d): %s", sess.Meta.Status, sess.Meta.Message)
		return
	}

	err = json.Unmarshal(sess.Data, &sessdata)
	if err != nil {
		return
	}

	return
}

func (client *Client) keepaliveError(reporter mediaservice.Reporter, err *error) {
	if err == nil || *err == nil || reporter == nil {
		return
	}

	reporter.Submit("keepalive error: "+(*err).Error(), true)
}

func (client *Client) keepalive(
	ctx context.Context,
	reporter mediaservice.Reporter,
	session *APIDataMovieSession,
	sess *SessionResponse,
	sessdata *SessionResponseData,
) {
	var err error

	defer client.keepaliveError(reporter, &err)

	t := time.NewTicker(time.Duration(session.HeartBeatLifetime/3000) * time.Second)

	defer t.Stop()

	var sessiondata, bs []byte

	u, err := url.Parse(session.URLS[0].URL)
	if err != nil {
		return
	}

	for {
		select {
		case <-t.C:
			sessurl := *u
			sessurl.Path += "/" + sessdata.Session.ID

			q := sessurl.Query()

			q.Set("_method", "PUT")
			q.Set("_format", "json")

			sessurl.RawQuery = q.Encode()

			sessiondata = sess.Data

			bs, err = client.postPage(ctx, sessurl.String(), bytes.NewReader(sessiondata), http.Header{
				"content-type": []string{"application/json"},
			})
			if err != nil {
				return
			}

			err = json.Unmarshal(bs, sess)
			if err != nil {
				return
			}

			if sess.Meta.Status/100 != 2 {
				return
			}

			err = json.Unmarshal(sess.Data, sessdata)
			if err != nil {
				return
			}
		case <-ctx.Done():
			sessurl := *u
			sessurl.Path += "/" + sessdata.Session.ID

			q := sessurl.Query()

			q.Set("_method", "DELETE")

			sessurl.RawQuery = q.Encode()

			sessiondata = sess.Data

			bg, cancel := context.WithTimeout(context.Background(), time.Minute)

			_, err = client.postPage(bg, sessurl.String(), bytes.NewReader(sessiondata), http.Header{
				"content-type": []string{"application/json"},
			})

			cancel()

			if err != nil {
				return
			}

			return
		}
	}
}

func (client *Client) reportProgress(ctx context.Context, reporter mediaservice.Reporter, f *os.File, total int64) {
	t := time.NewTicker(time.Second)

	defer t.Stop()

	finfo, err := f.Stat()
	if err != nil {
		return
	}

	totalsize := mediaservice.HumanSizeFormat(float64(total))

	for {
		select {
		case <-t.C:
			old := finfo.Size()

			finfo, err = f.Stat()
			if err != nil {
				return
			}

			diffsize := finfo.Size() - old

			if diffsize == 0 {
				continue
			}

			percent := float64(finfo.Size()) / float64(total) * 100

			if percent > 100 {
				percent = 100
			}

			speed := mediaservice.HumanSizeFormat(float64(diffsize))

			rem := total - finfo.Size()

			remain := rem / diffsize

			minutes, seconds := remain/60, remain%60

			msg := fmt.Sprintf(
				"%2.1f%% of %s at %s/s ETA %02d:%02d",
				percent,
				totalsize,
				speed,
				minutes,
				seconds,
			)

			reporter.Submit(msg, false)
		case <-ctx.Done():
			return
		}
	}
}

// QueryFormat returns API response with available media data formats
func (client *Client) QueryFormat(
	ctx context.Context,
	urls, _ string,
	reporter mediaservice.Reporter,
) (*APIData, error) {
	return client.fetchAPIData(ctx, urls, reporter)
}

type contentStream int

const (
	contentStreamAudio = 1
	contentStreamVideo = 2
)

type streamChunk struct {
	data   *[]byte
	err    error
	done   chan struct{}
	block  cipher.Block
	url    string
	iv     []byte
	stream contentStream
	idx    int
	init   bool
}

type contentChunk struct {
	audio *streamChunk
	video *streamChunk
}

func (chunk *contentChunk) download(work chan *streamChunk, audiodata, videodata *[]byte) (datas [][]byte, err error) {
	work <- chunk.audio
	work <- chunk.video

	<-chunk.audio.done
	<-chunk.video.done

	if chunk.audio.err != nil {
		return nil, chunk.audio.err
	}

	if chunk.video.err != nil {
		return nil, chunk.video.err
	}

	if len(*videodata) != 0 {
		datas = append(datas, *videodata)
	}

	if len(*audiodata) != 0 {
		datas = append(datas, *audiodata)
	}

	return
}

var (
	regexpURL              = regexp.MustCompile(`URI="([^"]*)`)
	regexpAverageBandwidth = regexp.MustCompile(`AVERAGE-BANDWIDTH=([^",]*)`)
	regexpIV               = regexp.MustCompile(`IV=0x([^",]*)`)
)

func (client *Client) parseM3U8AES(ctx context.Context, contents []*streamChunk, ivs, key string) (err error) {
	var iv []byte

	iv, err = hex.DecodeString(ivs)
	if err != nil {
		return
	}

	keybs, err := client.getPage(ctx, key)
	if err != nil {
		return
	}

	var block cipher.Block

	block, err = aes.NewCipher(keybs)
	if err != nil {
		return
	}

	for _, c := range contents {
		if !c.init {
			c.iv = iv
			c.block = block
		}
	}

	return
}

func matchM3U8(s string, regex *regexp.Regexp) (res string, ok bool) {
	parts := regex.FindAllStringSubmatch(s, -1)
	if len(parts) > 0 && len(parts[0]) == 2 {
		return parts[0][1], true
	}

	return "", false
}

func (client *Client) parseStreamM3U8(
	ctx context.Context,
	stream contentStream,
	page string,
) (contents []*streamChunk, err error) {
	data, err := client.getPage(ctx, page)
	if err != nil {
		return
	}

	reader := bufio.NewReader(bytes.NewReader(data))

	var line, key, ivs string

	for {
		line, err = reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
		}

		line = strings.TrimSpace(strings.TrimSuffix(line, "\n"))

		switch {
		case strings.HasPrefix(line, "#EXT-X-MAP"):
			if res, ok := matchM3U8(line, regexpURL); ok {
				contents = append(
					contents,
					&streamChunk{url: res, stream: stream, init: true, idx: len(contents)},
				)
			}
		case strings.HasPrefix(line, "#EXT-X-KEY"):
			if res, ok := matchM3U8(line, regexpURL); ok {
				key = res
			}

			if res, ok := matchM3U8(line, regexpIV); ok {
				ivs = res
			}
		case !strings.HasPrefix(line, "#") && line != "":
			contents = append(
				contents,
				&streamChunk{url: line, stream: stream, init: false, idx: len(contents)},
			)
		}
	}

	err = client.parseM3U8AES(ctx, contents, ivs, key)

	return
}

func (client *Client) downloadDMSExtractPlaylistsParseM3U8(
	respbs []byte,
) (audio, video string, bandwidth int64, err error) {
	var line string

	reader := bufio.NewReader(bytes.NewReader(respbs))

	for {
		line, err = reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				err = nil

				break
			}
		}

		line = strings.TrimSpace(strings.TrimSuffix(line, "\n"))

		switch {
		case strings.HasPrefix(line, "#EXT-X-MEDIA:TYPE=AUDIO"):
			if res, ok := matchM3U8(line, regexpURL); ok {
				audio = res
			}
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF"):
			if res, ok := matchM3U8(line, regexpAverageBandwidth); ok {
				bandwidth, err = strconv.ParseInt(res, 10, 64)
				if err != nil {
					return
				}
			}
		case !strings.HasPrefix(line, "#") && line != "":
			video = strings.TrimSuffix(line, "\n")
		}
	}

	return
}

func (client *Client) downloadDMSExtractPlaylists(
	ctx context.Context,
	data *APIData,
	aformatid, vformatid string,
) (audio, video string, bandwidth int64, err error) {
	nvapi := fmt.Sprintf(
		"https://nvapi.nicovideo.jp/v1/watch/%s/access-rights/hls?actionTrackId=%s",
		data.Client.WatchID,
		data.Client.WatchTrackID,
	)

	var sessiondata []byte

	sessiondata, err = json.Marshal(map[string]interface{}{
		"outputs": []interface{}{
			[]string{vformatid, aformatid},
		},
	})
	if err != nil {
		return
	}

	var respbs []byte

	var resp struct {
		Data struct {
			ContentURL string `json:"contentUrl"`
		} `json:"data"`
	}

	respbs, err = client.postPage(ctx, nvapi, bytes.NewReader(sessiondata), http.Header{
		"accept-encoding":    []string{"br"},
		"content-type":       []string{"application/json"},
		"x-request-with":     []string{"https://www.nicovideo.jp"},
		"x-access-right-key": []string{data.Media.Domand.AccessRightKey},
		"x-frontend-id":      []string{"6"},
		"x-frontend-version": []string{"0"},
	})
	if err != nil {
		return
	}

	err = json.Unmarshal(respbs, &resp)
	if err != nil {
		return
	}

	respbs, err = client.getPage(ctx, resp.Data.ContentURL)
	if err != nil {
		return
	}

	return client.downloadDMSExtractPlaylistsParseM3U8(respbs)
}

func (client *Client) downloadDMSWorker(ctx context.Context, work chan *streamChunk) {
	for {
		select {
		case stream, ok := <-work:
			if !ok {
				return
			}

			err := client.getPageBuf(ctx, stream.url, stream.data)
			if err != nil {
				stream.err = err

				close(stream.done)

				continue
			}

			if stream.block != nil {
				cbc := cipher.NewCBCDecrypter(stream.block, stream.iv)
				cbc.CryptBlocks(*stream.data, *stream.data)

				length := len(*stream.data)
				unpadding := int((*stream.data)[length-1])
				*stream.data = (*stream.data)[:(length - unpadding)]
			}

			close(stream.done)
		case <-ctx.Done():
			return
		}
	}
}

func (client *Client) downloadDMSResumeIndex(f *os.File, idxbs []byte, magic uint32) (idx int) {
	siz, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return
	}

	if int(siz) > len(idxbs) {
		_, err = f.Seek(-int64(len(idxbs)), io.SeekEnd)
		if err != nil {
			return
		}

		_, err = io.ReadFull(f, idxbs)
		if err != nil {
			return
		}

		if binary.BigEndian.Uint32(idxbs[0:4]) != uint32(len(idxbs)) ||
			binary.BigEndian.Uint32(idxbs[4:8]) != magic {
			_, err = f.Seek(0, io.SeekStart)
			if err != nil {
				return
			}
		} else {
			idx = int(binary.BigEndian.Uint64(idxbs[8:16])) + 1
		}
	}

	return
}

func (client *Client) downloadDMS(
	ctx context.Context,
	f *os.File,
	data *APIData,
	aformatid, vformatid, outpath string,
	dur time.Duration,
	reporter mediaservice.Reporter,
) (err error) {
	defer func() {
		_ = f.Close()

		if err == nil {
			_ = os.Remove(f.Name())
		}
	}()

	var contentChunks []*contentChunk

	var audioChunks, videoChunks []*streamChunk

	audio, video, bandwidth, err := client.downloadDMSExtractPlaylists(ctx, data, aformatid, vformatid)
	if err != nil {
		return err
	}

	audioChunks, err = client.parseStreamM3U8(ctx, contentStreamAudio, audio)
	if err != nil {
		return err
	}

	videoChunks, err = client.parseStreamM3U8(ctx, contentStreamVideo, video)
	if err != nil {
		return err
	}

	if len(audioChunks) != len(videoChunks) {
		return fmt.Errorf(
			"uneven audi/video streams: %d != %d",
			len(audioChunks),
			len(videoChunks),
		)
	}

	for i := 0; i < len(audioChunks); i++ {
		contentChunks = append(contentChunks, &contentChunk{
			audio: audioChunks[i],
			video: videoChunks[i],
		})
	}

	work := make(chan *streamChunk, 2)

	defer close(work)

	for i := 0; i < 2; i++ {
		go client.downloadDMSWorker(ctx, work)
	}

	go client.reportProgress(ctx, reporter, f, bandwidth*int64(dur.Seconds())/8)

	idxbs := make([]byte, 16)

	off := int64(len(idxbs))

	const jaroid = 0x31393139

	idx := client.downloadDMSResumeIndex(f, idxbs, jaroid)

	binary.BigEndian.PutUint32(idxbs[0:4], uint32(len(idxbs)))
	binary.BigEndian.PutUint32(idxbs[4:8], uint32(jaroid))

	audiodata, videodata := make([]byte, 0), make([]byte, 0)

	for ; idx < len(contentChunks); idx++ {
		chunk := contentChunks[idx]

		if idx > 0 {
			_, err = f.Seek(-off, io.SeekEnd)
			if err != nil {
				return
			}
		}

		chunk.audio.done = make(chan struct{})
		chunk.video.done = make(chan struct{})

		chunk.audio.data, chunk.video.data = &audiodata, &videodata

		var datas [][]byte

		datas, err = chunk.download(work, &audiodata, &videodata)
		if err != nil {
			return err
		}

		if chunk.audio.init {
			err = combineInitSegments(datas, f)
		} else {
			err = combineMediaSegments(datas, f)
		}

		audiodata = audiodata[:0]
		videodata = videodata[:0]

		if err != nil {
			return err
		}

		binary.BigEndian.PutUint64(idxbs[off-8:], uint64(idx))

		_, err = f.Write(idxbs)
		if err != nil {
			return err
		}
	}

	_, err = f.Seek(0, io.SeekStart)
	if err != nil {
		return err
	}

	of, err := os.OpenFile(outpath, os.O_CREATE|os.O_TRUNC|os.O_RDWR, 0644)
	if err != nil {
		return
	}

	defer func() {
		_ = of.Close()
	}()

	metadata := map[string]string{
		"cprt":    "https://www.nicovideo.jp/watch/" + data.Video.ID,
		"\xA9nam": data.Video.Title,
		"\xA9cmt": data.Video.Description,
		"\xA9day": data.Video.RegisteredAt,
	}

	err = DefragmentMP4(f, of, metadata)
	if err != nil {
		return
	}

	return nil
}

func (client *Client) downloadDMC(
	ctx context.Context,
	f *os.File,
	data *APIData,
	aformatid, vformatid, outpath string,
	reuse bool,
	reporter mediaservice.Reporter,
) (err error) {
	defer func() {
		_ = f.Close()

		if err != nil || !reuse {
			return
		}

		_ = os.Rename(f.Name(), outpath)
	}()

	var (
		session  APIDataMovieSession
		sess     SessionResponse
		sessdata SessionResponseData
	)

	session = data.Media.Delivery.Movie.Session

	sess, sessdata, err = client.createSession(ctx, &session, aformatid, vformatid)
	if err != nil {
		return err
	}

	siz, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}

	resp, err := client.methodPage(ctx, sessdata.Session.ContentURI, http.MethodGet, nil, http.Header{
		"range": []string{fmt.Sprintf("bytes=%d-", siz)},
	})
	if err != nil {
		return err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	cl, err := extractContentsRange(resp)
	if err != nil {
		return err
	}

	go client.keepalive(ctx, reporter, &session, &sess, &sessdata)

	if cl == siz {
		return nil
	}

	go client.reportProgress(ctx, reporter, f, cl)

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

// SaveFormat mediaserivce.Downloader implementation
func (client *Client) SaveFormat(
	ctx context.Context,
	urls, formatID, outpath string,
	reuse bool,
	dataraw []byte,
	opts *mediaservice.SaveOptions,
) (fname string, err error) {
	reporter := opts.GetReporter()

	data := &APIData{}

	_ = json.Unmarshal(dataraw, data)

	if time.Since(data.Created).Seconds() >= float64(data.Media.Delivery.Movie.Session.ContentKeyTimeout) {
		data, err = client.fetchAPIData(ctx, urls, reporter)
		if err != nil {
			return "", err
		}
	}

	aformatid, vformatid, _, _, dur, err := mediaservice.SelectFormat(data.ListFormats(), formatID)
	if err != nil {
		return "", err
	}

	fmtname := strings.TrimPrefix(vformatid, "archive_") + "--" + strings.TrimPrefix(aformatid, "archive_")

	outpath = strings.ReplaceAll(outpath, "${fmt}", fmtname)

	tempname := outpath
	if reuse {
		tempname = outpath + ".part"
	}

	f, err := os.OpenFile(tempname, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return "", err
	}

	err = client.openCopyFile(f, outpath, fmtname, reuse)
	if err != nil {
		return "", err
	}

	cctx, cancel := context.WithCancel(ctx)

	defer cancel()

	switch {
	case data.Media.Domand.AccessRightKey != "":
		err = client.downloadDMS(cctx, f, data, aformatid, vformatid, outpath, dur, reporter)
	case len(data.Media.Delivery.Movie.Session.URLS) > 0:
		err = client.downloadDMC(cctx, f, data, aformatid, vformatid, outpath, reuse, reporter)
	default:
		err = fmt.Errorf("unknown content delivery method")
	}

	if err != nil {
		return "", err
	}

	return outpath, nil
}

func extractContentsRange(resp *http.Response) (int64, error) {
	cr := resp.Header.Get("content-range")

	parts := strings.Split(cr, "/")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid session")
	}

	return strconv.ParseInt(parts[1], 10, 64)
}
