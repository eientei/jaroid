package main

import (
	"context"
	"fmt"
	"math"
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
	"github.com/jessevdk/go-flags"
)

var opts struct {
	URL          *string `short:"u" long:"url" description:"Fediverse instance URL"`
	Login        *string `short:"l" long:"login" description:"Fediverse instance login"`
	Dir          *string `long:"dir" description:"Download directory"`
	YoutubeDLBin *string `long:"youtube-dl-path" description:"Set youtube-dl path"`
	Listen       *string `long:"listen" description:"Listen for authorization code" optional:"true" optional-value:":0"`
	Acccount     struct {
		Code string `short:"c" long:"code" description:"OAuth2 code"`
	} `command:"account"`
	Quiet   bool `short:"q" long:"quiet" description:"Suppress extra output"`
	Default bool `long:"default" description:"Set specifid url/login/args as default"`
}

func extractExtraArgs() (args, extraargs []string, extraok bool) {
	for i, a := range os.Args {
		if a == "--" {
			return os.Args[1:i], os.Args[i+1:], true
		}
	}

	return os.Args[1:], nil, false
}

type resp struct {
	ch   chan error
	code string
}

type binconfig struct {
	command     string
	uri         string
	login       string
	videourl    string
	format      string
	subs        string
	redirect    string
	redirectch  chan *resp
	extraargs   []string
	target      float64
	extraargsok bool
	post        bool
	list        bool
	force       bool
}

func parseRest(p *flags.Parser, rest []string, c *binconfig) error {
	for _, a := range rest {
		switch {
		case a == "account":
		case a == "list":
			c.list = true
		case a == "post":
			c.post = true
		case strings.HasPrefix(a, "sub"):
			c.subs = a
		case strings.ToLower(a) == "inf" || strings.ToLower(a) == "max":
			c.target = math.MaxFloat64
		case c.videourl == "":
			c.videourl = a
		case mediaservice.MatchesHumanSize(a):
			if strings.HasSuffix(a, "!") {
				c.force = true
				a = strings.TrimSuffix(a, "!")
			}

			c.target = float64(mediaservice.HumanSizeParse(a))
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
	var preargs []string

	preargs, c.extraargs, c.extraargsok = extractExtraArgs()

	p := flags.NewParser(&opts, flags.Default)
	p.SubcommandsOptional = true
	p.Usage = "https://www.nicovideo.jp/watch/sm0000000 <size[!]|formatid|max|list> [sub[:<jp|en|cn>]] [post]"

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

	c.target = math.MaxFloat64

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
					"%s account -u your.instance.domain -l yourlogin [--listen]\n\n"+
					"To change default instace/account\n"+
					"%s -u your.instance.domain -l yourlogin --default\n\n"+
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
					"To download also a danmaku subitles, add 'sub'\n"+
					"%s https://www.nicovideo.jp/watch/sm0000000 <size[!]|formatid|max> sub\n\n"+
					"To select danmaku subitles language 'sub:<en|jp|cn>'\n"+
					"%s https://www.nicovideo.jp/watch/sm0000000 <size[!]|formatid|max> sub:jp\n\n"+
					"To post a video, add 'post'\n"+
					"%s https://www.nicovideo.jp/watch/sm0000000 <size[!]|formatid|max> post\n\n"+
					"To pass extra options to youtube-dl\n"+
					"%s https://www.nicovideo.jp/watch/sm0000000 -- -u nicovideologin -p nicovideopassword\n\n",
				os.Args[0],
				os.Args[0],
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
		fmt.Println("both -u your.instance.domain and -l yourlogin are requied")

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
			fmt.Printf("%s account -u %s -l %s -c yourcode\n", os.Args[0], c.uri, c.login)
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

	if c.extraargsok {
		fedipost.Config.Mediaservice.YoutubeDL.CommonArgs = c.extraargs
	}

	if opts.YoutubeDLBin != nil {
		fedipost.Config.Mediaservice.YoutubeDL.ExecutablePath = *opts.YoutubeDLBin
	}

	if opts.Dir != nil {
		fedipost.Config.Mediaservice.YoutubeDL.SaveDir = *opts.Dir
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
	reporter := mediaservice.NewReporter(0, 10)

	go func() {
		for s := range reporter.Messages() {
			_, _ = os.Stderr.WriteString(s + "\n")
		}
	}()

	return reporter
}

func main() {
	c := parseConfig()

	fedipost, err := app.New("")
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

	if c.extraargsok {
		mediaservicecopy.YoutubeDL.CommonArgs = c.extraargs
	}

	downloader := mediaservicecopy.Instance()

	if c.format == "" {
		handleList(ctx, c, fedipost, downloader)
	}

	if c.format == "" {
		_, _ = fmt.Fprintln(os.Stderr, "No available formats found")

		os.Exit(3)
	}

	match := handleDownload(ctx, c, &mediaservicecopy, downloader)

	if !opts.Quiet {
		_, _ = fmt.Fprintln(os.Stderr, "Downloaded", c.format, "to", match)
	}

	if c.post {
		var status *statuses.CreatedStatus

		status, err = fedipost.MakeStatus(ctx, c.uri, c.login, c.videourl, match)
		if err != nil {
			panic(err)
		}

		fmt.Println(status.URL)
	}
}

func handleDownload(
	ctx context.Context,
	c binconfig,
	mediaservicecopy *config.Mediaservice,
	downloader mediaservice.Downloader,
) string {
	fid := nicopost.FormatFileID(path.Base(c.videourl), c.format)

	match, err := nicopost.GlobFind(mediaservicecopy.YoutubeDL.SaveDir, fid)
	if err != nil {
		panic(err)
	}

	if match != "" {
		return match
	}

	match = nicopost.SaveFilepath(mediaservicecopy.YoutubeDL.SaveDir, c.format)

	reporter := mediaservice.NewDummyReporter()

	if !opts.Quiet {
		_, _ = fmt.Fprintln(os.Stderr, "Downloading format", c.format)

		reporter = startReporter()
	}

	var downopts []mediaservice.SaveOption

	if c.subs != "" {
		c.subs = strings.TrimPrefix(c.subs, "sub")
		c.subs = strings.TrimPrefix(c.subs, ":")

		if c.subs == "" {
			c.subs = "jpn"
		}

		downopts = append(downopts, &mediaservice.SaveOptionSubs{
			Lang: c.subs,
		})
	}

	_, err = downloader.SaveFormat(ctx, c.videourl, c.format, match, reporter, downopts...)
	if err != nil {
		panic(err)
	}

	match, err = nicopost.GlobFind(mediaservicecopy.YoutubeDL.SaveDir, fid)
	if err != nil {
		panic(err)
	}

	return match
}

func handleList(ctx context.Context, c binconfig, fedipost *app.Fedipost, downloader mediaservice.Downloader) {
	thumb, err := fedipost.Client.ThumbInfo(path.Base(c.videourl))
	if err != nil {
		panic(err)
	}

	reporter := mediaservice.NewDummyReporter()

	if !opts.Quiet {
		reporter = startReporter()
	}

	formats, err := downloader.ListFormats(ctx, c.videourl, reporter)
	if err != nil {
		panic(err)
	}

	formatted, suggest, min := nicopost.ProcessFormats(
		formats,
		thumb.Length,
		c.target,
	)

	if suggest.Name != "" {
		c.format = suggest.Name
	} else if min.Name != "" {
		if !c.force {
			_, _ = fmt.Fprintf(
				os.Stderr,
				"Smallest available format is %s - %s\n"+
					"To proceed either select it, or force to preselect smallest available with '!'\n",
				min.Name,
				mediaservice.HumanSizeFormat(min.Size),
			)

			os.Exit(2)
		}

		c.format = min.Name
	}

	if c.list {
		fmt.Print(formatted)

		os.Exit(0)
	}
}