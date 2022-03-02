package nicovideo

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"io/ioutil"
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
	dataAPIDataRegex = regexp.MustCompile(`data-api-data="([^"]+)"`)
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
	URL             string `json:"url"`
	IsWellKnownPort bool   `json:"isWellKnownPort"`
	IsSSL           bool   `json:"isSsl"`
}

// APIDataMovieSession json mapping
type APIDataMovieSession struct {
	AuthTypes         map[string]string    `json:"authTypes"`
	RecipeID          string               `json:"recipeId"`
	PlayerID          string               `json:"playerId"`
	ServiceUserID     string               `json:"serviceUserId"`
	Token             string               `json:"token"`
	Signature         string               `json:"signature"`
	ContentID         string               `json:"contentId"`
	Videos            []string             `json:"videos"`
	Audios            []string             `json:"audios"`
	URLS              []*APIDataSessionURL `json:"urls"`
	HeartBeatLifetime uint64               `json:"heartbeatLifetime"`
	ContentKeyTimeout uint64               `json:"contentKeyTimeout"`
	Priority          float64              `json:"priority"`
}

// APIDataAudioMetadata json mapping
type APIDataAudioMetadata struct {
	Bitrate      uint64 `json:"bitrate"`
	SamplingRate uint64 `json:"	samplingRate"`
}

// APIDataMovieAudio json mapping
type APIDataMovieAudio struct {
	ID          string               `json:"id"`
	IsAvailable bool                 `json:"isAvailable"`
	Metadata    APIDataAudioMetadata `json:"metadata"`
}

// APIDataVideoResolution json mapping
type APIDataVideoResolution struct {
	Width  uint64 `json:"width"`
	Height uint64 `json:"height"`
}

// APIDataVideoMetadata json mapping
type APIDataVideoMetadata struct {
	Label      string                 `json:"label"`
	Bitrate    uint64                 `json:"bitrate"`
	Resolution APIDataVideoResolution `json:"resolution"`
}

// APIDataMovieVideo json mapping
type APIDataMovieVideo struct {
	ID          string               `json:"id"`
	Metadata    APIDataVideoMetadata `json:"metadata"`
	IsAvailable bool                 `json:"isAvailable"`
}

// APIDataMovie json mapping
type APIDataMovie struct {
	Audios []*APIDataMovieAudio `json:"audios"`
	Videos []*APIDataMovieVideo `json:"videos"`
	Sesion APIDataMovieSession  `json:"session"`
}

// APIDataDelivery json mapping
type APIDataDelivery struct {
	Movie APIDataMovie `json:"movie"`
}

// APIDataMedia json mapping
type APIDataMedia struct {
	Delivery APIDataDelivery `json:"delivery"`
}

// APIDataVideo json mapping
type APIDataVideo struct {
	Duration DurationSeconds `json:"duration"`
}

// APIData represents subset of data-api-data video stream information required to establish a download session
type APIData struct {
	Media APIDataMedia `json:"media"`
	Video APIDataVideo `json:"video"`
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

	return client.HTTPClient.Do(req)
}

func (client *Client) getPage(ctx context.Context, url string) ([]byte, error) {
	resp, err := client.methodPage(ctx, url, http.MethodGet, nil, nil)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	return ioutil.ReadAll(resp.Body)
}

func (client *Client) postPage(ctx context.Context, url string, body io.Reader, h http.Header) ([]byte, error) {
	resp, err := client.methodPage(ctx, url, http.MethodPost, body, h)
	if err != nil {
		return nil, err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	return ioutil.ReadAll(resp.Body)
}

func (client *Client) fetchInitPage(ctx context.Context, url string, reporter mediaservice.Reporter) ([]byte, error) {
	reporter.Submit("Downloading video metadata...", false)

	bs, err := client.getPage(ctx, url)

	anonymous := strings.Contains(string(bs), "'not_login'") || strings.Contains(string(bs), "NEED_LOGIN")

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

	bs, err := ioutil.ReadAll(resp.Body)
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

	data := &APIData{}

	err = json.Unmarshal(([]byte)(html.UnescapeString(parts[0][1])), data)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (data *APIData) listFormats() (formats []*mediaservice.Format) {
	audios := make(map[string]mediaservice.AudioFormat)

	var audioIDs []string

	for _, a := range data.Media.Delivery.Movie.Audios {
		if !a.IsAvailable {
			continue
		}

		audios[a.ID] = mediaservice.AudioFormat{
			ID:         a.ID,
			Codec:      mediaservice.AudioCodecAAC,
			Bitrate:    a.Metadata.Bitrate,
			Samplerate: a.Metadata.SamplingRate,
		}

		audioIDs = append(audioIDs, a.ID)
	}

	videos := make(map[string]mediaservice.VideoFormat)

	var videoIDs []string

	for _, v := range data.Media.Delivery.Movie.Videos {
		if !v.IsAvailable {
			continue
		}

		videos[v.ID] = mediaservice.VideoFormat{
			ID:      v.ID,
			Codec:   mediaservice.VideoCodecH264,
			Bitrate: v.Metadata.Bitrate,
			Width:   v.Metadata.Resolution.Width,
			Height:  v.Metadata.Resolution.Height,
		}

		videoIDs = append(videoIDs, v.ID)
	}

	for _, v := range videoIDs {
		for _, a := range audioIDs {
			formats = append(formats, &mediaservice.Format{
				ID:        strings.TrimPrefix(v, "archive_") + "-" + strings.TrimPrefix(a, "archive_"),
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

	return data.listFormats(), nil
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

// SaveFormat mediaserivce.Downloader implementation
func (client *Client) SaveFormat(
	ctx context.Context,
	urls, formatID, outpath string,
	reuse bool,
	opts *mediaservice.SaveOptions,
) (fname string, err error) {
	reporter := opts.GetReporter()

	data, err := client.fetchAPIData(ctx, urls, reporter)
	if err != nil {
		return "", err
	}

	aformatid, vformatid, err := mediaservice.SelectFormat(data.listFormats(), formatID)
	if err != nil {
		return "", err
	}

	session := data.Media.Delivery.Movie.Sesion

	sess, sessdata, err := client.createSession(ctx, &session, aformatid, vformatid)
	if err != nil {
		return "", err
	}

	cctx, cancel := context.WithCancel(ctx)

	defer cancel()

	fmtname := strings.TrimPrefix(vformatid, "archive_") + "-" + strings.TrimPrefix(aformatid, "archive_")

	outpath = strings.ReplaceAll(outpath, "${fmt}", fmtname)

	tempname := outpath
	if reuse {
		tempname = outpath + ".part"
	}

	f, err := os.OpenFile(tempname, os.O_RDWR|os.O_CREATE, 0666)
	if err != nil {
		return "", err
	}

	defer func() {
		_ = f.Close()

		if err != nil || !reuse {
			return
		}

		_ = os.Rename(tempname, outpath)
	}()

	err = client.openCopyFile(f, outpath, fmtname, reuse)
	if err != nil {
		return "", err
	}

	finfo, err := f.Stat()
	if err != nil {
		return "", err
	}

	_, err = f.Seek(0, io.SeekEnd)
	if err != nil {
		return "", err
	}

	resp, err := client.methodPage(ctx, sessdata.Session.ContentURI, http.MethodGet, nil, http.Header{
		"range": []string{fmt.Sprintf("bytes=%d-", finfo.Size())},
	})
	if err != nil {
		return "", err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	cl, err := extractContentsRange(resp)
	if err != nil {
		return "", err
	}

	go client.keepalive(cctx, reporter, &session, &sess, &sessdata)

	if cl == finfo.Size() {
		return outpath, nil
	}

	go client.reportProgress(cctx, reporter, f, cl)

	_, err = io.Copy(f, resp.Body)
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
