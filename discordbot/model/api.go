// Package model provides model and task repositories
package model

import (
	"github.com/go-redis/redis/v7"
)

// Task provides interface for persistable tasks
type Task interface {
	Scope() string
	Name() string
}

// NewRepository provides Repository instance
func NewRepository(client *redis.Client) *Repository {
	return &Repository{
		Client: client,
		Groups: make(map[string]bool),
	}
}
