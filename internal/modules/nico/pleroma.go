package nico

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"regexp"
	"strings"
	"time"
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

func (mod *module) pleromaUploadFile(task *TaskPleromaPost) (string, error) {
	f, err := os.Open(task.FilePath)
	if err != nil {
		return "", err
	}

	defer func() {
		_ = f.Close()
	}()

	var b bytes.Buffer

	w := multipart.NewWriter(&b)

	fw, err := w.CreateFormFile("file", path.Base(f.Name()))
	if err != nil {
		return "", err
	}

	_, err = io.Copy(fw, f)
	if err != nil {
		return "", err
	}

	err = w.Close()
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest(http.MethodPost, task.PleromaHost+"/api/v1/media", &b)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+task.PleromaAuth)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	var media struct {
		ID string `json:"id"`
	}

	err = json.NewDecoder(resp.Body).Decode(&media)
	if err != nil {
		return "", err
	}

	return media.ID, nil
}

type pleromaStatus struct {
	Status      string   `json:"status,omitempty"`
	ContentType string   `json:"content_type,omitempty"`
	MediaIDs    []string `json:"media_ids,omitempty"`
}

func (mod *module) pleromaCreateStatus(task *TaskPleromaPost, status *pleromaStatus) error {
	var b bytes.Buffer

	err := json.NewEncoder(&b).Encode(status)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, task.PleromaHost+"/api/v1/statuses", &b)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+task.PleromaAuth)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	_ = mod.config.Discord.MessageReactionAdd(task.ChannelID, task.MessageID, emojiArrowUp)

	return nil
}

var symregex = regexp.MustCompile(`[^\pL\pN_]`)

func (mod *module) pleromaPost(task *TaskPleromaPost) error {
	name := path.Base(task.VideoURL)

	res, err := mod.client.ThumbInfo(name)
	if err != nil {
		mod.config.Log.WithError(err).Error("searching for " + name)

		return err
	}

	var tags []string

	for _, td := range res.Tags {
		if td.Domain == "jp" {
			for _, t := range td.Tag {
				tags = append(tags, "#"+symregex.ReplaceAllString(t, ""))
			}
		}
	}

	body := "**" + res.Title + "**\n" + res.WatchURL + "\n" + strings.Join(tags, " ")

	mediaID, err := mod.pleromaUploadFile(task)
	if err != nil {
		return err
	}

	return mod.pleromaCreateStatus(task, &pleromaStatus{
		Status:      body,
		MediaIDs:    []string{mediaID},
		ContentType: "text/markdown",
	})
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

		err = mod.tryPerform(func() error {
			return mod.pleromaPost(task)
		})
		if err != nil {
			mod.config.Log.WithError(err).Error("Posting pleroma status")
		}

		mod.ackTask(task, id, err)
	}
}
