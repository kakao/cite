package models

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
)

type BuildBot struct {
	github         *GitHub
	imageNameRegex *regexp.Regexp
	logURLRegex    *regexp.Regexp
}

type BuildbotChangeHook struct {
	Project    string    `json:"project"`
	Revision   string    `json:"revision"`
	Branch     string    `json:"branch"`
	RevLink    string    `json:"revlink"`
	Repository string    `json:"repository"`
	CreatedAt  time.Time `json:"when_timestamp"`
	Author     string    `json:"author"`
	Comments   string    `json:"comments"`
}

var (
	buildbotOnce sync.Once
	buildbotInst *BuildBot
)

type LogChunk struct {
	Content   string `json:"content"`
	FirstLine int    `json:"firstline"`
	LogID     int    `json:"logid"`
}

type LogChunks struct {
	Meta      interface{} `json:"meta"`
	LogChunks []LogChunk  `json:"logchunks"`
}

func NewBuildBot() *BuildBot {
	buildbotOnce.Do(func() {
		buildbotInst = &BuildBot{
			github:         NewCommonGitHub(),
			imageNameRegex: regexp.MustCompile("imageName:(.+)"),
			logURLRegex:    regexp.MustCompile("logURL:(.+)"),
		}
	})
	return buildbotInst
}

func (this *BuildBot) GetLogContent(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("failed to get log contents from %s: %v", url, err)
	}

	var logChunks LogChunks
	if err := json.Unmarshal(body, &logChunks); err != nil {
		return "", fmt.Errorf("failed to read log contents from %s: %v", url, err)
	}

	logs := make([]string, len(logChunks.LogChunks))
	for i, lc := range logChunks.LogChunks {
		lcr := bufio.NewScanner(strings.NewReader(lc.Content))
		var lcs []string
		for lcr.Scan() {
			lc := lcr.Text()
			prefix := ""
			switch lc[0] {
			case 'h':
				prefix = "header"
			case 'o':
				prefix = "stdout"
			case 'e':
				prefix = "stderr"
			case 'i':
				prefix = "input"
			}
			lcs = append(lcs, fmt.Sprintf("%s: %s", prefix, lc[1:len(lc)]))
		}
		logs[i] = strings.Join(lcs, "\n")
	}
	return strings.Join(logs, "\n"), nil
}

func (b *BuildBot) GetImageName(description string) (string, error) {
	imageNameSubmatch := b.imageNameRegex.FindStringSubmatch(description)
	if len(imageNameSubmatch) < 2 {
		return "", fmt.Errorf("image name parse failed on description %s", description)
	}
	return strings.TrimSpace(imageNameSubmatch[1]), nil
}

func (b *BuildBot) GetLogURL(description string) (string, error) {
	logURLSubmatch := b.logURLRegex.FindStringSubmatch(description)
	if len(logURLSubmatch) < 2 {
		return "", fmt.Errorf("log URL parse failed on description %s", description)
	}
	return strings.TrimSpace(logURLSubmatch[1]), nil
}

func (b *BuildBot) Build(owner, repo, branch, sha string) error {
	c, err := b.github.GetCommit(owner, repo, sha)
	if err != nil {
		return err
	}

	bch := BuildbotChangeHook{
		Project:    fmt.Sprintf("%s/%s", owner, repo),
		Repository: strings.TrimSuffix(*c.HTMLURL, "/commit/"+sha),
		Branch:     branch,
		Author:     fmt.Sprintf("%s <%s>", *c.Commit.Committer.Name, *c.Commit.Committer.Email),
		CreatedAt:  *c.Commit.Committer.Date,
		RevLink:    *c.HTMLURL,
		Comments:   *c.Commit.Message,
		Revision:   sha,
	}

	client := &http.Client{}
	req, err := http.NewRequest(http.MethodPost, Conf.Buildbot.WebHook, nil)

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "cite")
	body, err := json.Marshal(bch)
	req.Body = ioutil.NopCloser(bytes.NewReader(body))
	resp, err := client.Do(req)
	if err != nil {
		respBody, _ := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()
		err := fmt.Errorf("failed to send 'cite' event to buildbot: %s", respBody)
		log.Printf(err.Error())
		return err
	}
	return nil
}

func (b *BuildBot) Proxy(method string, header http.Header, body []byte) error {
	req, err := http.NewRequest(method, Conf.Buildbot.WebHook, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create new proxy request: %v", err)
	}
	req.Header = header

	buildbotClient := http.DefaultClient
	resp, err := buildbotClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send proxy request: %v", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 399 {
		respBody, _ := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()
		return fmt.Errorf("buildbot error. status:%v, body:%s", resp.Status, respBody)
	}
	return nil
}
