package nico

import (
	"context"
	"time"

	"github.com/eientei/jaroid/nicopost"

	"golang.org/x/oauth2"

	"github.com/eientei/jaroid/fedipost"
	"github.com/eientei/jaroid/fedipost/statuses"
)

func (mod *module) pleromaPostEnqueue(task *TaskDownload, fpath string) {
	s, ok := mod.servers[task.GuildID]

	if !ok || s.pleromaAuth == "" || s.pleromaHost == "" {
		return
	}

	_, err := mod.config.Repository.TaskEnqueue(&TaskPleromaPost{
		GuildID:     task.GuildID,
		ChannelID:   task.ChannelID,
		MessageID:   task.MessageID,
		VideoURL:    task.VideoURL,
		PleromaHost: s.pleromaHost,
		PleromaAuth: s.pleromaAuth,
		FilePath:    fpath,
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

		err = mod.pleromaPost(task)
		if err != nil {
			mod.config.Log.WithError(err).Error("Posting pleroma status")
			_ = mod.config.Discord.MessageReactionAdd(task.ChannelID, task.MessageID, emojiNegative)
		} else {
			_ = mod.config.Discord.MessageReactionAdd(task.ChannelID, task.MessageID, emojiArrowUp)
		}

		mod.ackTask(task, id, err)
	}
}

func (mod *module) pleromaPost(task *TaskPleromaPost) error {
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

	status, err := nicopost.MakeNicovideoStatus(config, mod.client, task.VideoURL, task.FilePath, "")
	if err != nil {
		return err
	}

	_, err = statuses.Create(config, status)

	return err
}