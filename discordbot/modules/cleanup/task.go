// Package cleanup provides bot module for automated bot replies removal
package cleanup

import (
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/eientei/jaroid/discordbot/model"
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

func (mod *module) ackTask(task model.Task, id string, err error) {
	if err != nil {
		mod.config.Log.WithError(err).Error("Dequeuing")

		if r, ok := err.(*discordgo.RESTError); ok && r.Response.StatusCode == 404 {
			err = nil
		}
	}

	if err == nil {
		err = mod.config.Repository.TaskAck(task, id)
		if err != nil {
			mod.config.Log.WithError(err).Error("Acking task", id)
		}
	}
}

func (mod *module) start() {
	task := &Task{}

	for {
		id, err := mod.config.Repository.TaskDequeue(task, time.Second)
		if err != nil {
			mod.config.Log.WithError(err).Error("Dequeuing")
		}

		if id == "" {
			continue
		}

		err = mod.config.Discord.ChannelMessageDelete(task.ChannelID, task.MessageID)
		mod.ackTask(task, id, err)
	}
}
