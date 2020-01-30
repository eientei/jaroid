// Package cleanup provides bot module for automated bot replies removal
package cleanup

import (
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/internal/bot"
	"github.com/eientei/jaroid/internal/model"
)

// Task provides message removal delayed task
type Task struct {
	GuildID   string `json:"guild_id"`
	ChannelID string `json:"channel_id"`
	MessageID string `json:"message_id"`
}

// Scope returns task scope
func (Task) Scope() string {
	return "cleanup"
}

// Name returns task name
func (Task) Name() string {
	return "message"
}

func ackTask(config *bot.Configuration, task model.Task, id string, err error) {
	if err != nil {
		config.Log.WithError(err).Error("Dequeuing")

		if r, ok := err.(*discordgo.RESTError); ok && r.Response.StatusCode == 404 {
			err = nil
		}
	}

	if err == nil {
		err = config.Repository.TaskAck(task, id)
		if err != nil {
			config.Log.WithError(err).Error("Acking task", id)
		}
	}
}

// Start starts cleaning up messages
func Start(config *bot.Configuration) {
	go func() {
		task := &Task{}

		for {
			id, err := config.Repository.TaskDequeue(task, time.Second)
			if err != nil {
				config.Log.WithError(err).Error("Dequeuing")
			}

			if id == "" {
				continue
			}

			err = config.Discord.ChannelMessageDelete(task.ChannelID, task.MessageID)
			ackTask(config, task, id, err)
		}
	}()
}
