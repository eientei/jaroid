package nico

import (
	"context"
	"path/filepath"
	"time"

	"github.com/eientei/jaroid/fedipost"
	"github.com/eientei/jaroid/fedipost/statuses"
	"github.com/eientei/jaroid/nicopost"
	"golang.org/x/oauth2"
)

func (mod *module) pleromaPostEnqueue(task *TaskDownload, fpath string) {
	s, ok := mod.servers[task.GuildID]

	if !ok || s.pleromaAuth == "" || s.pleromaHost == "" {
		return
	}

	_, _, err := mod.config.Repository.TaskEnqueue(&TaskPleromaPost{
		GuildID:     task.GuildID,
		ChannelID:   task.ChannelID,
		MessageID:   task.MessageID,
		VideoURL:    task.VideoURL,
		PleromaHost: s.pleromaHost,
		PleromaAuth: s.pleromaAuth,
		FilePath:    fpath,
		Preview:     task.Preview,
	}, 0, 0)
	if err != nil {
		mod.config.Log.WithError(err).Error("Scheduling pleroma post", task.GuildID, task.ChannelID, task.MessageID)
	}
}

func (mod *module) startPleromaPost() {
	task := &TaskPleromaPost{}

	for {
		id, err := mod.config.Repository.TaskDequeue(task, time.Second)
		if err != nil {
			mod.config.Log.WithError(err).Error("Dequeuing")
		}

		if id == "" {
			continue
		}

		mod.ackTask(task, id, err)

		err = mod.pleromaPost(context.Background(), task)

		if err != nil {
			mod.config.Log.WithError(err).Error("Posting pleroma status")
			_ = mod.config.Discord.MessageReactionAdd(task.ChannelID, task.MessageID, emojiNegative)
		} else {
			_ = mod.config.Discord.MessageReactionAdd(task.ChannelID, task.MessageID, emojiArrowUp)
		}
	}
}

func (mod *module) pleromaPost(ctx context.Context, task *TaskPleromaPost) error {
	config := &fedipost.Config{
		HTTPClient: oauth2.NewClient(context.Background(), oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken:  task.PleromaAuth,
			TokenType:    "Bearer",
			RefreshToken: "",
			Expiry:       time.Time{},
		})),
		Host:                   task.PleromaHost,
		MediaEndpoint:          task.PleromaHost + "/api/v1/media",
		StatusesEndpoint:       task.PleromaHost + "/api/v1/statuses",
		AppsEndpoint:           task.PleromaHost + "/api/v1/apps",
		OauthTokenEndpoint:     task.PleromaHost + "/oauth/token",
		OauthAuthorizeEndpoint: task.PleromaHost + "/oauth/authorize",
	}

	status, err := nicopost.MakeNicovideoStatus(
		ctx,
		config,
		mod.config.Nicovideo,
		task.VideoURL,
		task.FilePath,
		"",
		task.Preview,
	)
	if err != nil {
		return err
	}

	if task.Preview {
		base := filepath.Base(task.FilePath)
		uri := mod.config.Config.Private.Nicovideo.Public + "/" + base

		mod.updateMessage(task.GuildID, task.ChannelID, task.MessageID, "```"+status.Status+"```\n"+uri)
	} else {
		_, err = statuses.Create(config, status)
	}

	return err
}
