package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

type Docker struct{}

type DockerRepositoryAuth struct {
	Token string `json:"token"`
}

var (
	dockerOnce sync.Once
	dockerInst *Docker
)

const (
	OFFICIAL_DOCKER_REPOSITORY_URL = "registry.hub.docker.com"
	OFFICIAL_DOCKER_AUTH_URL       = "auth.docker.io/token"
	OFFICIAL_DOCKER_AUTH_SERVICE   = "registry.docker.io"
	OFFICIAL_DOCKER_AUTH_SCOPE     = "repository:%v:pull"
)

func NewDocker() *Docker {
	dockerOnce.Do(func() {
		dockerInst = &Docker{}
	})
	return dockerInst
}

func (this *Docker) CheckImage(imageName string) bool {
	i := strings.SplitN(imageName, "/", 2)
	repo := i[0]
	nameAndTag := i[1]
	j := strings.Split(nameAndTag, ":")
	name := j[0]
	tag := j[1]
	url := fmt.Sprintf("https://%v/v2/%v/manifests/%v", repo, name, tag)
	resp, err := http.Get(url)
	return err == nil && resp.StatusCode == http.StatusOK
}

func (this *Docker) DeleteImage(imageName, tag string) error {
	i := strings.SplitN(imageName, "/", 2)
	repo := i[0]
	name := i[1]
	url := fmt.Sprintf("https://%v/v2/%v/manifests/%v", repo, name, tag)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusAccepted {
		body, _ := ioutil.ReadAll(resp.Body)
		return errors.New(string(body))
	}
	return nil
}

func (this *Docker) GetOfficialRegistryAuthToken(name string) (string, error) {
	params := url.Values{}
	params.Add("service", OFFICIAL_DOCKER_AUTH_SERVICE)
	params.Add("scope", fmt.Sprintf(OFFICIAL_DOCKER_AUTH_SCOPE, name))
	url := fmt.Sprintf("https://%s?%s", OFFICIAL_DOCKER_AUTH_URL, params.Encode())

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	body, _ := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s", body)
	}

	auth := &DockerRepositoryAuth{}
	if err := json.Unmarshal(body, auth); err != nil {
		return "", err
	}
	return auth.Token, nil
}

func (this *Docker) IsRepositoryV2(repo string) (bool, error) {
	resp, err := http.Get(fmt.Sprintf("https://%v/v2/", repo))
	if err != nil || resp.StatusCode != http.StatusOK {
		return false, err
	}
	return true, nil
}

func (this *Docker) ListImageTags(fullname string) ([]string, error) {
	var (
		repo, name, token string
		err               error
		isV2              bool
	)

	if strings.Contains(fullname, ".") {
		logger.Info("private repository image:", fullname)
		i := strings.SplitN(fullname, "/", 2)
		repo = i[0]
		name = i[1]
		isV2, err = this.IsRepositoryV2(repo)
		if err != nil {
			return []string{}, fmt.Errorf("failed to check repository version:%v", err)
		}
	} else {
		logger.Info("official repository image:", fullname)
		isV2 = true
		repo = OFFICIAL_DOCKER_REPOSITORY_URL
		if strings.Index(fullname, "/") < 0 {
			name = "library/" + fullname
		} else {
			name = fullname
		}
		token, err = this.GetOfficialRegistryAuthToken(name)
		if err != nil {
			return []string{}, err
		}
	}

	if isV2 {
		return this.ListImageTagsV2(repo, name, token)
	} else {
		return this.ListImageTagsV1(repo, name, token)
	}
}

func (this *Docker) ListImageTagsV1(repo, name, token string) ([]string, error) {
	url := fmt.Sprintf("https://%v/v1/repositories/%v/tags", repo, name)
	logger.Info(fmt.Sprintf("listTagsURL: %v, token:%v", url, token))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return []string{}, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return []string{}, err
	}
	body, _ := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	logger.Info(fmt.Sprintf("body: %s", body))
	if resp.StatusCode != http.StatusOK {
		return []string{}, fmt.Errorf("%s", body)
	}

	tagsWithSHA := make(map[string]string)
	if err := json.Unmarshal(body, &tagsWithSHA); err != nil {
		return []string{}, err
	}
	logger.Info(fmt.Sprintf("tags: %v", tagsWithSHA))
	tags := make([]string, len(tagsWithSHA))
	i := 0
	for k, _ := range tagsWithSHA {
		tags[i] = k
		i += 1
	}
	return tags, nil
}

func (this *Docker) ListImageTagsV2(repo, name, token string) ([]string, error) {
	url := fmt.Sprintf("https://%v/v2/%v/tags/list", repo, name)
	logger.Info(fmt.Sprintf("listTagsURL: %v, token:%v", url, token))

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return []string{}, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return []string{}, err
	}
	body, _ := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	logger.Info(fmt.Sprintf("body: %s", body))
	if resp.StatusCode != http.StatusOK {
		return []string{}, fmt.Errorf("%s", body)
	}

	type DockerRepositoryTags struct {
		Name string   `json:"name"`
		Tags []string `json:"tags"`
	}
	tags := &DockerRepositoryTags{}
	if err := json.Unmarshal(body, tags); err != nil {
		return []string{}, err
	}
	return tags.Tags, nil
}
