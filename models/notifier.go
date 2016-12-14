package models

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

type Notifier struct {
	k8s *Kubernetes
	wc  *WatchCenter
	// slack Slack
}

var (
	notiOnce sync.Once
	notiInst *Notifier
)

func NewNotifier() *Notifier {
	notiOnce.Do(func() {
		notiInst = &Notifier{
			k8s: NewKubernetes(),
			wc:  NewWatchCenter(),
		}
	})
	return notiInst
}

func (n *Notifier) Send(nms []Notification, msg string) error {
	for _, nm := range nms {
		if !nm.Enable {
			continue
		}
		switch nm.Driver {
		case "slack":
			payload := fmt.Sprintf(`{"text": "%s"}`, msg)
			resp, err := http.Post(nm.Endpoint, "application/json", strings.NewReader(payload))
			defer resp.Body.Close()
			if err != nil {
				respBody, _ := ioutil.ReadAll(resp.Body)
				return fmt.Errorf("failed to send slack message: %s", respBody)
			}
		case "watchcenter":
			ep, err := strconv.Atoi(nm.Endpoint)
			if err != nil {
				return fmt.Errorf("failed to convert watchcenter endpoint %s to int: %v", nm.Endpoint, err)
			}
			n.wc.SendGroupTalk(ep, msg)
		default:
			logger.Errorf("unknown notification driver %s", nm.Driver)
		}
	}
	return nil
}

func (n *Notifier) SendSystem(msg string) error {
	nm := Notification{
		Driver:      "slack",
		Enable:      true,
		Endpoint:    Conf.Notification.Default.Slack,
		Description: "system message",
	}
	return n.Send([]Notification{nm}, msg)
}

func (n *Notifier) SendWithFallback(nms []Notification, wc int, msg string) error {
	// for backward compatibility
	if len(nms) == 0 {
		n.wc.SendGroupTalk(wc, msg)
		return nil
	}
	return n.Send(nms, msg)
}
