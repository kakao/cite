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

func (n *Notifier) Send(nsName, svcName, msg string) error {
	_, meta, err := n.k8s.GetService(nsName, svcName)
	if err != nil {
		return err
	}

	// for backward compatibility
	if len(meta.Notification) == 0 {
		n.wc.SendGroupTalk(meta.Watchcenter, msg)
	}

	for _, nm := range meta.Notification {
		if !nm.Enable {
			continue
		}
		switch nm.Driver {
		case "watchcenter":
			ep, err := strconv.Atoi(nm.Endpoint)
			if err != nil {
				return fmt.Errorf("failed to convert watchcenter endpoint %s to int: %v", nm.Endpoint, err)
			}
			n.wc.SendGroupTalk(ep, msg)
		case "slack":
			payload := fmt.Sprintf(`{"text": "%s"}`, msg)
			resp, err := http.Post(nm.Endpoint, "application/json", strings.NewReader(payload))
			defer resp.Body.Close()
			if err != nil {
				respBody, _ := ioutil.ReadAll(resp.Body)
				return fmt.Errorf("failed to send slack message: %s", respBody)
			}
		default:
			logger.Errorf("unknown notification driver %s", nm.Driver)
		}
	}
	return nil
}
