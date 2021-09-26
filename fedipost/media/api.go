// Package media provides methods for media API
package media

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"

	"github.com/eientei/jaroid/fedipost"
)

// UploadFile returns media id for provided file path
func UploadFile(config *fedipost.Config, filepath string) (string, error) {
	f, err := os.Open(filepath)
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

	req, err := http.NewRequest(http.MethodPost, config.MediaEndpoint, &b)
	if err != nil {
		return "", err
	}

	req.Header.Set("content-type", w.FormDataContentType())

	resp, err := config.Exchange(req, true)
	if err != nil {
		return "", err
	}

	defer func() {
		_ = resp.Body.Close()
	}()

	var media struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}

	err = json.NewDecoder(resp.Body).Decode(&media)
	if err != nil {
		return "", err
	}

	if media.Error != "" {
		return "", errors.New(media.Error)
	}

	return media.ID, nil
}
