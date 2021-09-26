package main

import (
	"flag"
	"os"

	"github.com/eientei/jaroid/mediaservice/youtubedl"

	"github.com/eientei/jaroid/discordbot/modules/rolereact"

	"github.com/eientei/jaroid/discordbot/modules/pin"

	"github.com/eientei/jaroid/discordbot/modules/logdb"

	"github.com/eientei/jaroid/discordbot/modules/deletereact"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/discordbot/bot"
	yamlConfig "github.com/eientei/jaroid/discordbot/config"
	"github.com/eientei/jaroid/discordbot/modules/auth"
	"github.com/eientei/jaroid/discordbot/modules/cleanup"
	"github.com/eientei/jaroid/discordbot/modules/color"
	"github.com/eientei/jaroid/discordbot/modules/config"
	"github.com/eientei/jaroid/discordbot/modules/help"
	"github.com/eientei/jaroid/discordbot/modules/join"
	"github.com/eientei/jaroid/discordbot/modules/nico"
	"github.com/eientei/jaroid/discordbot/modules/reply"
	"github.com/go-redis/redis/v7"
	"github.com/sirupsen/logrus"
)

func readConfig(log *logrus.Logger, configPath string) *yamlConfig.Root {
	configFile, err := os.OpenFile(configPath, os.O_CREATE|os.O_RDONLY, 0644)
	if err != nil {
		flag.PrintDefaults()
		log.Fatal(err)
	}

	c, err := yamlConfig.Read(configFile)
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

	b, err := bot.NewBot(bot.Options{
		Discord: dg,
		Client:  client,
		Config:  configRoot,
		Log:     log,
		Downloader: &youtubedl.Downloader{
			ExecutablePath: "youtube-dl",
			FormatRegexp:   "",
			CommonArgs:     configRoot.Private.Nicovideo.Opts,
			SaveArgs:       nil,
			ListArgs:       nil,
		},
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

	err = b.Serve()
	if err != nil {
		log.Fatal(err)
	}
}
