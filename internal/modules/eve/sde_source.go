package eve

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const sdeBaseURL = "https://developers.eveonline.com/static-data/tranquility/"

type sdeSource interface {
	Latest(context.Context) (int64, error)
	Download(context.Context, int64) (string, error)
}

type officialSDESource struct {
	dir    string
	client *http.Client
}

func newSDESource(dir string) *officialSDESource {
	if dir == "" {
		dir = filepath.Join(".local", "sde")
	}
	return &officialSDESource{dir: dir, client: &http.Client{Timeout: 12 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || req.URL.Scheme != "https" || req.URL.Host != "developers.eveonline.com" {
			return errors.New("unexpected SDE redirect")
		}
		return nil
	}}}
}
func (s *officialSDESource) get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GloryNavy-Seat/SDE-type-names")
	response, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 {
		response.Body.Close()
		return nil, fmt.Errorf("official SDE HTTP %d", response.StatusCode)
	}
	return response, nil
}
func (s *officialSDESource) Latest(ctx context.Context) (int64, error) {
	response, err := s.get(ctx, sdeBaseURL+"latest.jsonl")
	if err != nil {
		return 0, err
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	response.Body.Close()
	if err != nil {
		return 0, err
	}
	if len(data) > 1<<20 {
		return 0, errors.New("SDE metadata too large")
	}
	var build int64
	matches := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var row struct {
			Key   string `json:"_key"`
			Build int64  `json:"buildNumber"`
		}
		if err = json.Unmarshal([]byte(line), &row); err != nil {
			return 0, err
		}
		if row.Key == "sde" {
			build = row.Build
			matches++
		}
	}
	if matches != 1 || build <= 0 {
		return 0, errors.New("invalid official SDE build metadata")
	}
	return build, nil
}
func (s *officialSDESource) Download(ctx context.Context, build int64) (string, error) {
	if build <= 0 {
		return "", errors.New("invalid SDE build")
	}
	name := fmt.Sprintf("eve-online-static-data-%d-jsonl.zip", build)
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return "", err
	}
	filename := filepath.Join(s.dir, name)
	if _, err := os.Stat(filename); err == nil {
		return filename, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	response, err := s.get(ctx, sdeBaseURL+name)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.ContentLength > 512<<20 {
		return "", errors.New("SDE ZIP exceeds 512 MiB")
	}
	output, err := os.CreateTemp(s.dir, "sde-download-*.part")
	if err != nil {
		return "", err
	}
	defer os.Remove(output.Name())
	size, copyErr := io.Copy(output, io.LimitReader(response.Body, (512<<20)+1))
	closeErr := output.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	if size > 512<<20 {
		return "", errors.New("SDE ZIP exceeds 512 MiB")
	}
	if err = os.Rename(output.Name(), filename); err != nil {
		// Another instance may have finished the same official build first. Import
		// validation still checks the existing archive before it can be published.
		if _, statErr := os.Stat(filename); statErr != nil {
			return "", err
		}
	}
	return filename, nil
}
