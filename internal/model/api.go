// Package model provides model and task repositories
package model

import (
	"time"

	"github.com/go-redis/redis/v7"
)

// Task can be serialized into byte form
type Task struct {
	ID      string
	GuildID string
	Data    map[string]interface{}
}

// NewRepository provides repository instance
func NewRepository(client *redis.Client) Repository {
	return &repository{
		client: client,
		groups: make(map[string]bool),
	}
}

// Repository interface provides methods to get and set configuration, enqueue and dequeue tasks
type Repository interface {
	ConfigSet(guildID, scope, key, value string) error
	ConfigGet(guildID, scope, key string) (string, error)
	TaskEnqueue(scope, name string, delay, timeout time.Duration, task *Task) error
	TaskDequeue(scope, name string, block time.Duration) (*Task, error)
	TaskAck(scope, name string, task *Task) error
}
