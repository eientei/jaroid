// Package main discord bot entrypoint
package main

import (
	"flag"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/cookiejarx"
	"github.com/eientei/jaroid/discordbot/bot"
	botConfig "github.com/eientei/jaroid/discordbot/config"
	"github.com/eientei/jaroid/discordbot/modules/auth"
	"github.com/eientei/jaroid/discordbot/modules/cleanup"
	"github.com/eientei/jaroid/discordbot/modules/color"
	"github.com/eientei/jaroid/discordbot/modules/config"
	"github.com/eientei/jaroid/discordbot/modules/deletereact"
	"github.com/eientei/jaroid/discordbot/modules/help"
	"github.com/eientei/jaroid/discordbot/modules/join"
	"github.com/eientei/jaroid/discordbot/modules/logdb"
	"github.com/eientei/jaroid/discordbot/modules/nico"
	"github.com/eientei/jaroid/discordbot/modules/pin"
	"github.com/eientei/jaroid/discordbot/modules/reply"
	"github.com/eientei/jaroid/discordbot/modules/rolereact"
	"github.com/eientei/jaroid/integration/nicovideo"
	"github.com/eientei/jaroid/mediaservice"
	"github.com/eientei/jaroid/util/httputil/middleware"
	redis "github.com/go-redis/redis/v7"
	"github.com/sirupsen/logrus"
)

func readConfig(log *logrus.Logger, configPath string) *botConfig.Root {
	configFile, err := os.OpenFile(configPath, os.O_CREATE|os.O_RDONLY, 0644)
	if err != nil {
		flag.PrintDefaults()
		log.Fatal(err)
	}

	c, err := botConfig.Read(configFile)
	if err != nil {
		flag.PrintDefaults()
		log.Fatal(err)
	}

	err = configFile.Close()
	if err != nil {
		flag.PrintDefaults()
		log.Fatal(err)
	}

	return c
}

func main() {
	log := logrus.New()

	configPath := flag.String("c", "config.yml", "Configuration file")
	login := flag.Bool("l", false, "cache login cookies")

	flag.Parse()

	configRoot := readConfig(log, *configPath)

	if configRoot.Private.Token == "" {
		log.Fatal("Missing token in config")
	}

	dg, err := discordgo.New("Bot " + configRoot.Private.Token)
	if err != nil {
		log.Fatal(err)
	}

	dg.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsAll)

	client := redis.NewClient(&redis.Options{
		Addr:     configRoot.Private.Redis.Address,
		Password: configRoot.Private.Redis.Password,
		DB:       configRoot.Private.Redis.DB,
	})

	var nicovideoAuth *nicovideo.Auth

	if configRoot.Private.Nicovideo.Auth.Username != "" && configRoot.Private.Nicovideo.Auth.Password != "" {
		nicovideoAuth = &nicovideo.Auth{
			Username: configRoot.Private.Nicovideo.Auth.Username,
			Password: configRoot.Private.Nicovideo.Auth.Password,
		}
	}

	storage := cookiejarx.NewInMemoryStorage()

	jar, err := cookiejarx.New(&cookiejarx.Options{
		Storage: storage,
	})
	if err != nil {
		panic(err)
	}

	homedir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	fpath := filepath.Join(homedir, ".config", "jaroid", "cookie.jar")

	err = os.MkdirAll(filepath.Dir(fpath), 0777)
	if err != nil {
		panic(err)
	}

	cookiejarfile := &middleware.ClientCookieJarFile{
		Storage:  storage,
		Jar:      jar,
		FilePath: fpath,
	}

	nicovideoClient := &http.Client{
		Transport: &middleware.Transport{
			Transport:   http.DefaultTransport,
			Middlewares: []middleware.Client{cookiejarfile},
		},
	}

	b, err := bot.NewBot(bot.Options{
		Discord: dg,
		Client:  client,
		Config:  configRoot,
		Log:     log,
		Nicovideo: nicovideo.New(&nicovideo.Config{
			HTTPClient: nicovideoClient,
			Auth:       nicovideoAuth,
		}),
		Modules: []bot.Module{
			cleanup.New(),
			reply.New(),
			auth.New(),
			help.New(),
			config.New(),
			nico.New(),
			join.New(),
			color.New(),
			deletereact.New(),
			logdb.New(),
			pin.New(),
			rolereact.New(),
		},
	})

	if err != nil {
		log.Fatal(err)
	}

	if *login {
		reporter := mediaservice.NewReporter(0, 16, os.Stdin)

		err = b.Nicovideo.CacheAuth(reporter)
		if err != nil {
			panic(err)
		}

		os.Exit(0)
	}

	err = b.Serve()
	if err != nil {
		log.Fatal(err)
	}
}
