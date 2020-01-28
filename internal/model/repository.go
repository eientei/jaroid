package model

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v7"
)

type repository struct {
	client *redis.Client
	groups map[string]bool
}

func (repo *repository) ConfigSet(guildID, scope, key, value string) error {
	fullkey := fmt.Sprintf("%s.%s.%s", guildID, scope, key)
	cmd := repo.client.Set(fullkey, value, 0)

	return cmd.Err()
}

func (repo *repository) ConfigGet(guildID, scope, key string) (s string, err error) {
	fullkey := fmt.Sprintf("%s.%s.%s", guildID, scope, key)
	s, err = repo.client.Get(fullkey).Result()

	if err == redis.Nil {
		err = nil
	}

	return
}

func (repo *repository) TaskEnqueue(scope, name string, delay, timeout time.Duration, task *Task) error {
	fkey := fmt.Sprintf("task.%s.%s", scope, name)

	bs, err := json.Marshal(task.Data)
	if err != nil {
		return err
	}

	return repo.client.XAdd(&redis.XAddArgs{
		Stream: fkey,
		Values: map[string]interface{}{
			"created": time.Now().Unix(),
			"delay":   int64(delay),
			"timeout": int64(timeout),
			"data":    bs,
		},
	}).Err()
}

func (repo *repository) processMessage(fkey string, m *redis.XMessage) (task *Task, err error) {
	var created, delay, timeout int64

	if raw, ok := m.Values["created"].(string); ok {
		created, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return
		}
	}

	if raw, ok := m.Values["delay"].(string); ok {
		delay, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return
		}
	}

	if raw, ok := m.Values["timeout"].(string); ok {
		timeout, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return
		}
	}

	passed := int64(time.Since(time.Unix(created, 0)))
	if passed < delay {
		return nil, nil
	}

	passed -= delay
	if timeout > 0 && passed > timeout {
		tx := repo.client.TxPipeline()
		tx.XAck(fkey, "tasks", m.ID)
		tx.XDel(fkey, m.ID)

		_, err = tx.Exec()
		if err != nil {
			return nil, err
		}

		return nil, nil
	}

	bs, ok := m.Values["data"].(string)
	if !ok {
		return nil, nil
	}

	task = &Task{
		ID:   m.ID,
		Data: make(map[string]interface{}),
	}

	err = json.Unmarshal([]byte(bs), &task.Data)
	if err != nil {
		return nil, err
	}

	return task, nil
}

func (repo *repository) TaskDequeue(scope, name string, block time.Duration) (task *Task, err error) {
	fkey := fmt.Sprintf("task.%s.%s", scope, name)

	if _, ok := repo.groups[fkey]; !ok {
		repo.client.XGroupCreateMkStream(fkey, "tasks", "0")
		repo.groups[fkey] = true
	}

	res, err := repo.client.XReadGroup(&redis.XReadGroupArgs{
		Group:    "tasks",
		Consumer: "dequeue",
		Streams:  []string{fkey, "0"},
		Block:    0,
		Count:    1,
	}).Result()
	if err == redis.Nil {
		err = nil
	}

	if err != nil {
		return nil, err
	}

	for _, s := range res {
		for _, m := range s.Messages {
			task, err = repo.processMessage(fkey, &m)
			if err != nil {
				return nil, err
			}

			if task != nil {
				return task, nil
			}
		}
	}

	res, err = repo.client.XReadGroup(&redis.XReadGroupArgs{
		Group:    "tasks",
		Consumer: "dequeue",
		Streams:  []string{fkey, ">"},
		Block:    block,
	}).Result()
	if err == redis.Nil {
		err = nil
	}

	if err != nil {
		return nil, err
	}

	for _, s := range res {
		for _, m := range s.Messages {
			task, err = repo.processMessage(fkey, &m)
			if err != nil {
				return nil, err
			}
		}
	}

	return
}

func (repo *repository) TaskAck(scope, name string, task *Task) (err error) {
	if task == nil {
		return nil
	}

	fkey := fmt.Sprintf("task.%s.%s", scope, name)

	tx := repo.client.TxPipeline()
	tx.XAck(fkey, "tasks", task.ID)
	tx.XDel(fkey, task.ID)
	_, err = tx.Exec()

	return
}
