package model

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	redis "github.com/go-redis/redis/v7"
)

// Repository provides methods to get and set configuration, enqueue and dequeue tasks
type Repository struct {
	Client *redis.Client
	Groups map[string]bool
}

// ConfigSet sets config value for given guild
func (repo *Repository) ConfigSet(guildID, scope, key, value string) error {
	fullkey := fmt.Sprintf("%s.%s.%s", guildID, scope, key)
	cmd := repo.Client.Set(fullkey, value, 0)

	return cmd.Err()
}

// ConfigGet returns config value for given guild
func (repo *Repository) ConfigGet(guildID, scope, key string) (s string, err error) {
	fullkey := fmt.Sprintf("%s.%s.%s", guildID, scope, key)
	s, err = repo.Client.Get(fullkey).Result()

	if err == redis.Nil {
		err = nil
	}

	return
}

// TaskEnqueue schedules task for execution
func (repo *Repository) TaskEnqueue(task Task, delay, timeout time.Duration) (id string, pending int64, err error) {
	fkey := fmt.Sprintf("task.%s.%s", task.Scope(), task.Name())

	gs, err := repo.Client.XRange(fkey, "-", "+").Result()
	if err != nil {
		return "", 0, err
	}

	pending = int64(len(gs))

	bs, err := json.Marshal(task)
	if err != nil {
		return "", 0, err
	}

	id, err = repo.Client.XAdd(&redis.XAddArgs{
		Stream: fkey,
		Values: map[string]interface{}{
			"created": time.Now().Unix(),
			"delay":   int64(delay),
			"timeout": int64(timeout),
			"data":    bs,
		},
	}).Result()
	if err != nil {
		return "", 0, err
	}

	return
}

func (repo *Repository) processMessage(
	fkey string,
	m *redis.XMessage,
	task Task,
	inwait int64,
) (minwait int64, id string, err error) {
	minwait = inwait

	var created, delay, timeout int64
	if raw, ok := m.Values["created"].(string); ok {
		created, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return minwait, "", nil
		}
	}

	if raw, ok := m.Values["delay"].(string); ok {
		delay, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return minwait, "", nil
		}
	}

	if raw, ok := m.Values["timeout"].(string); ok {
		timeout, err = strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return minwait, "", nil
		}
	}

	passed := int64(time.Since(time.Unix(created, 0)))
	if passed < delay {
		if delay-passed < minwait {
			minwait = delay - passed
		}

		return minwait, "", nil
	}

	passed -= delay
	if timeout > 0 && passed > timeout {
		tx := repo.Client.TxPipeline()
		tx.XAck(fkey, "tasks", m.ID)
		tx.XDel(fkey, m.ID)

		_, err = tx.Exec()
		if err != nil {
			return 0, "", err
		}

		return minwait, "", nil
	}

	bs, ok := m.Values["data"].(string)
	if !ok {
		return minwait, "", nil
	}

	err = json.Unmarshal([]byte(bs), &task)
	if err != nil {
		return minwait, "", nil
	}

	return minwait, m.ID, nil
}

func (repo *Repository) processMessages(
	fkey string,
	res []redis.XStream,
	task Task,
) (minwait int64, id string, err error) {
	minwait = int64(math.MaxInt64)

	for _, s := range res {
		for _, m := range s.Messages {
			minwait, id, err = repo.processMessage(fkey, &m, task, minwait)
			if err != nil || id != "" {
				return
			}
		}
	}

	return minwait, "", nil
}

func (repo *Repository) readMessages(
	fkey, start string,
	task Task,
	block time.Duration,
) (minwait int64, id string, err error) {
	res, err := repo.Client.XReadGroup(&redis.XReadGroupArgs{
		Group:    "tasks",
		Consumer: "dequeue",
		Streams:  []string{fkey, start},
		Block:    block,
	}).Result()
	if err == redis.Nil {
		err = nil
	}

	if err != nil {
		return minwait, "", err
	}

	return repo.processMessages(fkey, res, task)
}

func (repo *Repository) ensureGroup(fkey string) {
	if _, ok := repo.Groups[fkey]; !ok {
		repo.Client.XGroupCreateMkStream(fkey, "tasks", "0")
		repo.Groups[fkey] = true
	}
}

// TaskDequeue retreives next task
func (repo *Repository) TaskDequeue(task Task, block time.Duration) (id string, err error) {
	fkey := fmt.Sprintf("task.%s.%s", task.Scope(), task.Name())

	repo.ensureGroup(fkey)

	var minwait int64

	minwait, id, err = repo.readMessages(fkey, "0", task, block)
	if err != nil || id != "" {
		return
	}

	pending := false

	if minwait != math.MaxInt64 && (minwait < int64(block) || block == 0) {
		block = time.Duration(minwait)
		pending = true
	}

	minwait, id, err = repo.readMessages(fkey, ">", task, block)
	if err != nil || id != "" {
		return
	}

	if !pending && minwait != math.MaxInt64 && minwait < int64(block) {
		time.Sleep(time.Duration(minwait))

		pending = true
	}

	if pending {
		_, id, err = repo.readMessages(fkey, "0", task, 0)
	}

	return
}

// TaskAck confirms task as successfully executed
func (repo *Repository) TaskAck(task Task, id string) (err error) {
	fkey := fmt.Sprintf("task.%s.%s", task.Scope(), task.Name())

	tx := repo.Client.TxPipeline()
	tx.XAck(fkey, "tasks", id)
	tx.XDel(fkey, id)
	_, err = tx.Exec()

	return
}

// TaskGet retrieves task by id
func (repo *Repository) TaskGet(task Task, id string) (err error) {
	fkey := fmt.Sprintf("task.%s.%s", task.Scope(), task.Name())

	ms, err := repo.Client.XRange(fkey, id, id).Result()
	if err != nil {
		return
	}

	for _, m := range ms {
		bs, ok := m.Values["data"].(string)
		if !ok {
			return nil
		}

		err = json.Unmarshal([]byte(bs), &task)

		return
	}

	return
}
