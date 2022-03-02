package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/eientei/jaroid/fedipost/app"
	"github.com/eientei/jaroid/fedipost/config"
	"github.com/eientei/jaroid/fedipost/statuses"
	"github.com/eientei/jaroid/mediaservice"
	"github.com/eientei/jaroid/nicopost"
	flags "github.com/jessevdk/go-flags"
)

var opts struct {
	URL       *string `short:"f" long:"fediverse" description:"Fediverse instance URL"`
	Login     *string `short:"l" long:"login" description:"Fediverse instance login"`
	Dir       *string `short:"d" long:"dir" description:"Download directory (.)"`
	Output    *string `short:"o" long:"output" description:"Output file"`
	Config    *string `short:"c" long:"config" description:"Config file location (~/.config/jaroid/fedipost.yml)"`
	CookieJar *string `short:"j" long:"cookie-jar" description:"Cookie jar file (~/.config/jaroid/cookie.jar)"`
	Listen    *string `long:"listen" optional:"true" optional-value:":0" description:"Listen for authorization code"`

	NicovideoUsername *string `short:"u" long:"username" description:"Nicovideo username"`
	NicovideoPassword *string `short:"p" long:"password" description:"Nicovideo password"`
	Acccount          struct {
		Code string `long:"code" description:"OAuth2 code"`
	} `command:"account"`
	Quiet   bool `short:"q" long:"quiet" description:"Suppress extra output"`
	Default bool `long:"default" description:"Set specifid url/login/args as default"`
}

type resp struct {
	ch   chan error
	code string
}

type binconfig struct {
	redirectch chan *resp
	command    string
	uri        string
	login      string
	videourl   string
	format     string
	subs       string
	redirect   string
	post       bool
	preview    bool
	list       bool
}

func parseRest(p *flags.Parser, rest []string, c *binconfig) error {
	for _, a := range rest {
		switch {
		case a == "account":
		case a == "list":
			c.list = true
		case a == "post":
			c.post = true
		case a == "preview":
			c.post = true
			c.preview = true
		case strings.HasPrefix(a, "sub"):
			c.subs = a
		case c.videourl == "":
			c.videourl = a
		case c.format == "":
			c.format = a
		}
	}

	if opts.Default || c.command != "" {
		return nil
	}

	if c.videourl == "" {
		p.WriteHelp(os.Stdout)

		return &flags.Error{
			Type: flags.ErrHelp,
		}
	}

	validateVideoURL(c.videourl)

	return nil
}

func validateVideoURL(videourl string) {
	pvurl, err := url.Parse(videourl)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err.Error(), "Invalid video URL:", videourl)

		os.Exit(1)
	}

	if !strings.HasPrefix(pvurl.Scheme, "http") ||
		!strings.HasSuffix(pvurl.Hostname(), "nicovideo.jp") {
		_, _ = fmt.Fprintln(os.Stderr, "Invalid video URL:", videourl)

		os.Exit(1)
	}
}

func parseConfig() (c binconfig) {
	preargs := os.Args[1:]

	p := flags.NewParser(&opts, flags.Default)
	p.SubcommandsOptional = true
	p.Usage = "https://www.nicovideo.jp/watch/sm0000000 <size[!]|formatid|max|list> [post]"

	rest, err := p.ParseArgs(preargs)

	if p.Active != nil {
		c.command = p.Active.Name
	}

	if err == nil {
		err = parseRest(p, rest, &c)
	}

	if opts.Listen != nil {
		c.redirect, c.redirectch, err = startLocalHTTP(*opts.Listen)
		if err != nil {
			panic(err)
		}
	}

	if err == nil && c.videourl == "" && !opts.Default && p.Active == nil {
		p.WriteHelp(os.Stdout)

		err = &flags.Error{
			Type: flags.ErrHelp,
		}
	}

	if err != nil {
		t, _ := err.(*flags.Error)
		if t.Type == flags.ErrHelp {
			_, _ = fmt.Printf(
				"To add an account\n"+
					"%s account -f your.instance.domain -l yourlogin [--listen]\n\n"+
					"To change default instace/account\n"+
					"%s -f your.instance.domain -l yourlogin --default\n\n"+
					"You can list available formats with\n"+
					"%s https://www.nicovideo.jp/watch/sm0000000 list\n\n"+
					"To download a video with selected format use\n"+
					"%s https://www.nicovideo.jp/watch/sm0000000 formatid\n\n"+
					"Alternatively preselect a format with estimated filesize less or equal to desired\n"+
					"%s https://www.nicovideo.jp/watch/sm0000000 50m\n\n"+
					"Or force smallest available, if there is no formats smaller or equal to selected\n"+
					"%s https://www.nicovideo.jp/watch/sm0000000 50m!\n\n"+
					"Alternatively preselect a maximum available format\n"+
					"%s https://www.nicovideo.jp/watch/sm0000000 max\n\n"+
					"To post a video, add 'post'\n"+
					"%s https://www.nicovideo.jp/watch/sm0000000 <size[!]|formatid|max> post\n\n"+
					"To provide authentication for nicoideo\n"+
					"%s https://www.nicovideo.jp/watch/sm0000000 -u nicovideologin -p nicovideopassword\n\n",
				os.Args[0],
				os.Args[0],
				os.Args[0],
				os.Args[0],
				os.Args[0],
				os.Args[0],
				os.Args[0],
				os.Args[0],
				os.Args[0],
			)
		}

		os.Exit(1)
	}

	if opts.URL != nil {
		c.uri = *opts.URL
	}

	if opts.Login != nil {
		c.login = *opts.Login
	}

	return
}

func startLocalHTTP(addr string) (string, chan *resp, error) {
	var deftry bool

	if addr == ":0" {
		addr = ":38080"
		deftry = true
	}

	l, err := net.Listen("tcp", addr)
	if err != nil {
		if deftry {
			addr = ":0"

			l, err = net.Listen("tcp", addr)
			if err != nil {
				return "", nil, err
			}
		} else {
			return "", nil, err
		}
	}

	ch := make(chan *resp)

	mux := http.NewServeMux()

	mux.Handle("/", http.NotFoundHandler())
	mux.HandleFunc("/callback", func(writer http.ResponseWriter, request *http.Request) {
		code := request.URL.Query().Get("code")
		if code == "" {
			http.Error(writer, "no query parameter 'code'", http.StatusBadRequest)

			return
		}

		r := &resp{
			code: code,
			ch:   make(chan error),
		}

		ch <- r

		err = <-r.ch
		if err != nil {
			http.Error(writer, "Error using code: "+err.Error(), http.StatusBadRequest)

			return
		}

		http.Error(writer, "Success! Check your terminal.", http.StatusOK)

		_ = l.Close()

		close(ch)
	})

	go func() {
		_ = http.Serve(l, mux)
	}()

	tcpaddr, _ := l.Addr().(*net.TCPAddr)
	host := tcpaddr.IP.String()

	if tcpaddr.IP.IsUnspecified() {
		host = "127.0.0.1"
	}

	cb := fmt.Sprintf("http://%s:%d/callback", host, tcpaddr.Port)

	return cb, ch, nil
}

func handleAccount(ctx context.Context, c *binconfig, fedipost *app.Fedipost) {
	if c.uri == "" || c.login == "" {
		fmt.Println("both -f your.instance.domain and -l yourlogin are requied")

		os.Exit(1)
	}

	if opts.Acccount.Code == "" {
		authurl, err := fedipost.MakeAccoutAuthorization(ctx, c.uri, c.login, c.redirect)
		if err != nil {
			panic(err)
		}

		fmt.Printf("Authorize jaroid application by followiing\n\n")
		fmt.Println(authurl)

		if c.redirect == "" {
			fmt.Printf("\nthen use received authorization code as\n\n")
			fmt.Printf("%s account -f %s -l %s --code yourcode\n", os.Args[0], c.uri, c.login)
			os.Exit(0)
		}
	}

	if c.redirect == "" {
		err := fedipost.ExchangeAuthorizeCode(ctx, c.uri, c.login, "", opts.Acccount.Code)
		if err != nil {
			panic(err)
		}
	} else {
		for res := range c.redirectch {
			err := fedipost.ExchangeAuthorizeCode(ctx, c.uri, c.login, c.redirect, res.code)

			res.ch <- err

			close(res.ch)

			if err == nil {
				break
			}

			_, _ = fmt.Fprintln(os.Stderr, err.Error())
		}

		<-c.redirectch
	}

	fmt.Println("\nSuccess! Got authentication token, now you can use posting API.")

	if c.videourl == "" {
		os.Exit(0)
	}
}

func handleDefaultInstanceLogin(c *binconfig, fedipost *app.Fedipost) {
	if opts.URL == nil && opts.Login == nil {
		return
	}

	inst, err := fedipost.Config.Instance(c.uri)
	if err != nil {
		panic(err)
	}

	if opts.URL != nil {
		fedipost.Config.Global.DefaultInstance = c.uri
	}

	if opts.Login != nil && inst != nil {
		inst.DefaultAccount = c.login
	}
}

func handleDefault(c *binconfig, fedipost *app.Fedipost) {
	handleDefaultInstanceLogin(c, fedipost)

	if opts.Dir != nil {
		fedipost.Config.Mediaservice.SaveDir = *opts.Dir
	}

	err := fedipost.Save()
	if err != nil {
		panic(err)
	}

	if c.videourl == "" && c.command == "" {
		os.Exit(0)
	}
}

func startReporter() mediaservice.Reporter {
	reporter := mediaservice.NewReporter(0, 10, os.Stdin)

	go func() {
		for s := range reporter.Messages() {
			_, _ = os.Stderr.WriteString(s + "\n")
		}
	}()

	return reporter
}

func overrides(f *app.Fedipost) {
	if opts.CookieJar != nil {
		f.Config.Mediaservice.CookieJar = *opts.CookieJar
	}

	if opts.NicovideoUsername != nil {
		f.Config.Mediaservice.Auth.Username = *opts.NicovideoUsername
	}

	if opts.NicovideoPassword != nil {
		f.Config.Mediaservice.Auth.Password = *opts.NicovideoPassword
	}

	if opts.Dir != nil {
		f.Config.Mediaservice.SaveDir = *opts.Dir
	}
}

func main() {
	c := parseConfig()

	var configpath string

	if opts.Config != nil {
		configpath = *opts.Config
	}

	fedipost, err := app.New(configpath, overrides)
	if err != nil {
		panic(err)
	}

	ctx := context.Background()

	if opts.Default {
		handleDefault(&c, fedipost)
	}

	if c.command == "account" {
		handleAccount(ctx, &c, fedipost)
	}

	mediaservicecopy := fedipost.Config.Mediaservice

	if c.list {
		handleList(ctx, c, fedipost.Client)
	}

	var match string

	if c.preview {
		match = "path/to/file.mp4"
	} else {
		match = handleDownload(ctx, c, &mediaservicecopy, fedipost.Client)
	}

	if !opts.Quiet {
		_, _ = fmt.Fprintln(os.Stderr, "Downloaded", c.format, "to", match)
	}

	if c.post {
		var status *statuses.CreatedStatus

		status, err = fedipost.MakeStatus(ctx, c.uri, c.login, c.videourl, match, c.preview)
		if err != nil {
			panic(err)
		}

		if c.preview {
			fmt.Println(status.Body)
		} else {
			fmt.Println(status.URL)
		}
	}
}

func handleDownload(
	ctx context.Context,
	c binconfig,
	mediaservicecopy *config.Mediaservice,
	downloader mediaservice.Downloader,
) string {
	fid := nicopost.FormatFileID(path.Base(c.videourl), c.format)

	match, err := nicopost.GlobFind(mediaservicecopy.SaveDir, fid)
	if err != nil {
		panic(err)
	}

	if match != "" && opts.Output == nil {
		return match
	}

	savedir := mediaservicecopy.SaveDir

	if savedir == "" {
		savedir, err = os.Getwd()
		if err != nil {
			panic(err)
		}
	} else {
		err = os.MkdirAll(mediaservicecopy.SaveDir, 0777)
		if err != nil {
			panic(err)
		}
	}

	match = nicopost.SaveFilepath(savedir, c.videourl, c.format)

	reporter := mediaservice.NewDummyReporter()

	if !opts.Quiet {
		_, _ = fmt.Fprintln(os.Stderr, "Downloading format", c.format)

		reporter = startReporter()
	}

	downopts := &mediaservice.SaveOptions{
		Reporter: reporter,
	}

	if c.subs != "" {
		c.subs = strings.TrimPrefix(c.subs, "sub")
		c.subs = strings.TrimPrefix(c.subs, ":")

		if c.subs == "" {
			c.subs = "jpn"
		}

		downopts.Subtitles = append(downopts.Subtitles, c.subs)
	}

	reuse := true

	if opts.Output != nil {
		match = *opts.Output
		reuse = false
	}

	match, err = downloader.SaveFormat(ctx, c.videourl, c.format, match, reuse, downopts)
	if err != nil {
		panic(err)
	}

	return match
}

func handleList(ctx context.Context, c binconfig, downloader mediaservice.Downloader) {
	reporter := mediaservice.NewDummyReporter()

	if !opts.Quiet {
		reporter = startReporter()
	}

	formats, err := downloader.ListFormats(ctx, c.videourl, &mediaservice.ListOptions{
		Reporter: reporter,
	})
	if err != nil {
		sugg, ok := err.(*mediaservice.ErrFormatSuggest)
		if ok {
			_, _ = fmt.Fprintf(
				os.Stderr,
				"Smallest available format is %s - %s\n"+
					"To proceed either select it, or force to preselect smallest available with '!'\n",
				sugg.ID,
				mediaservice.HumanSizeFormat(float64(sugg.SizeEstimate())),
			)

			os.Exit(2)
		}

		panic(err)
	}

	formatted := nicopost.ProcessFormats(formats)

	if c.list {
		fmt.Print(formatted)

		os.Exit(0)
	}
}
