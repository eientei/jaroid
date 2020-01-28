package main

import (
	"context"
	"flag"
	"os"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/bot"
	"github.com/eientei/jaroid/internal/config"
	"github.com/go-redis/redis/v7"
	"github.com/sirupsen/logrus"
)

func readConfig(log *logrus.Logger, configPath string) *config.Root {
	configFile, err := os.OpenFile(configPath, os.O_CREATE|os.O_RDONLY, 0644)
	if err != nil {
		flag.PrintDefaults()
		log.Fatal(err)
	}

	c, err := config.Read(configFile)
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

	client := redis.NewClient(&redis.Options{
		Addr:     configRoot.Private.Redis.Address,
		Password: configRoot.Private.Redis.Password,
		DB:       configRoot.Private.Redis.DB,
	})

	b := bot.NewBot(bot.Options{
		Discord: dg,
		Client:  client,
		Config:  configRoot,
		Log:     log,
	})

	err = b.Serve(context.Background())
	if err != nil {
		log.Fatal(err)
	}
}
