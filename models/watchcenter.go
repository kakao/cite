package models

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

type WatchCenter struct {
	baseURL string
}

const (
	SendGroupTalk    = "/send/group/kakaotalk"
	SendPersonalTalk = "/send/personal/kakaotalk"
	ListGroups       = "/user/%s/groups"
)

var (
	wcOnce sync.Once
	wcInst *WatchCenter
)

func NewWatchCenter() *WatchCenter {
	wcOnce.Do(func() {
		wcInst = &WatchCenter{
			baseURL: Conf.Notification.Watchcenter.API,
		}
	})
	return wcInst
}

type WatchCenterResponse struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Success  bool   `json:"success,omitempty"`
	Response string `json:"response"`
}

type WatchCenterGroupResponse struct {
	Code     string             `json:"code"`
	Message  string             `json:"message"`
	Response []WatchCenterGroup `json:response`
}

type WatchCenterGroup struct {
	Id       int `json:"id"`
	Category struct {
		Type                string `json:"type"`
		UnconditionalInvite bool   `json:"unconditionalInvite"`
	} `json:"category"`
	Name                string `json:"name"`
	Description         string `json:"description"`
	ServiceCode         string `json:"serviceCode"`
	LegacyId            string `json:"legacyId,omitempty"`
	UnconditionalInvite bool   `json:"unconditionalInvite"`
}

func (this *WatchCenter) ListGroups(username, email string) ([]WatchCenterGroup, error) {
	username = strings.Replace(username, "-", ".", 1)
	if idx := strings.Index(email, "@"); idx > 0 {
		email = email[0:idx]
	}

	for _, u := range []string{username, email} {
		wcURL := fmt.Sprintf(this.baseURL+ListGroups, u)
		resp, err := http.Get(wcURL)
		if err != nil {
			return nil, err
		}
		respBody, _ := ioutil.ReadAll(resp.Body)
		defer resp.Body.Close()

		var wcResp WatchCenterGroupResponse
		err = json.Unmarshal(respBody, &wcResp)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal watchcenter response: %v", err)
		}
		if len(wcResp.Response) > 0 {
			return wcResp.Response, nil
		}
	}
	return nil, fmt.Errorf("watchcenter groups not found")
}

func (this *WatchCenter) SendGroupTalk(to int, msg string) {
	this.sendTalk(SendGroupTalk, strconv.Itoa(to), msg)
}

func (this *WatchCenter) SendPersonalTalk(to, msg string) {
	this.sendTalk(SendPersonalTalk, to, msg)
}

func (this *WatchCenter) sendTalk(api, to, msg string) {
	wcURL := this.baseURL + api
	values := url.Values{
		"to": []string{to},
		"msg": []string{
			fmt.Sprintf("[CITE-%s] %s", Conf.Cite.Version, msg)},
	}
	resp, _ := http.PostForm(wcURL, values)
	respBody, _ := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()

	var wcResp WatchCenterResponse
	json.Unmarshal(respBody, &wcResp)
	if wcResp.Success == false {
		logger.Warning("error while send message to watchcenter:", wcResp)
	}
}
