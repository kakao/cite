package controller

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	githubClient "github.com/google/go-github/github"
	"github.com/kakao/cite/goroutines"
	"github.com/kakao/cite/models"
	"github.com/labstack/echo"
)

func PostGithubCallback(c echo.Context) error {
	githubEvent, ok := c.Request().Header["X-GitHub-Event"]
	if !ok {
		githubEvent = []string{"unknown"}
	}
	logger.Info("received github event:", githubEvent)
	var body = clearJSONRepoOrgField(c.Request().Body)

	switch githubEvent[0] {
	case "push":
		// check if pushed repo/branch is registered to cite
		var event githubClient.PushEvent
		if err := json.Unmarshal(body, &event); err != nil {
			errMsg := fmt.Sprintf("error while unmarshalling event:%v, %v", body, err)
			logger.Error(errMsg)
			return echo.NewHTTPError(http.StatusBadRequest, errMsg)
		}
		nsName := util.NormalizeByHyphen("", *event.Repo.Owner.Name)
		refs := strings.Split(*event.Ref, "/")
		branch := refs[len(refs)-1]
		svcLabels := k8s.GetLabels(*event.Repo.Name, branch)
		svcs, err := k8s.GetServices(nsName, svcLabels)
		if err != nil || len(svcs) == 0 {
			return echo.NewHTTPError(http.StatusNotFound, "service not found. owner:%s, repo:%s, branch:%s", *event.Repo.Owner.Name, *event.Repo.Name, branch)
		}

		return buildbotClient.Proxy(c.Request().Method, c.Request().Header, body)

	case "status":
		var event githubClient.StatusEvent
		if err := json.Unmarshal(body, &event); err != nil {
			errMsg := fmt.Sprintf("error while unmarshalling event:%v, %v", body, err)
			logger.Error(errMsg)
			return echo.NewHTTPError(http.StatusBadRequest, errMsg)
		}
		logger.Debugf("event: %v", event)

		if event.Context == nil {
			errMsg := "event context is nil. skipping..."
			logger.Error(errMsg)
			return echo.NewHTTPError(http.StatusBadRequest, errMsg)
		}

		if *event.Context != models.CITE_BUILDBOT_GITHUB_CONTEXT {
			errMsg := fmt.Sprintf("not a cite build: %s. skipping...", *event.Context)
			logger.Error(errMsg)
			return echo.NewHTTPError(http.StatusBadRequest, errMsg)
		}

		ownerName := util.NormalizeByHyphen("", *event.Repo.Owner.Login)
		repoName := *event.Repo.Name
		branchName := ""
		for _, branch := range event.Branches {
			if *branch.Commit.SHA == *event.SHA {
				branchName = *branch.Name
				break
			}
		}

		if branchName == "" {
			branchNames := make([]string, len(event.Branches))
			for i, b := range event.Branches {
				branchNames[i] = *b.Name
			}
			errMsg := fmt.Sprintf("unknown branch:%v, owner:%s, repo:%s",
				branchNames, ownerName, repoName)
			logger.Info(errMsg)
			return echo.NewHTTPError(http.StatusNotFound, errMsg)
		}

		logger.Infof("received state:%s, owner:%s, repo:%s, branch:%s",
			*event.State, ownerName, repoName, branchName)

		svcLabels := k8s.GetLabels(repoName, branchName)
		svcs, err := k8s.GetServices(ownerName, svcLabels)
		if err != nil || len(svcs) < 1 {
			errMsg := fmt.Sprintf("service not found. owner:%s, repo:%s, branch:%s",
				ownerName, repoName, branchName)
			logger.Info(errMsg)
			return echo.NewHTTPError(http.StatusNotFound, errMsg)
		}
		svc := svcs[0]
		metaStr, ok := svc.Annotations[models.CITE_K8S_ANNOTATION_KEY]
		if !ok {
			errMsg := fmt.Sprintf("cite annotation not found. ns:%s, svc:%s",
				svc.Namespace, svc.Name)
			logger.Info(errMsg)
			return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
		}

		meta, err := models.UnmarshalMetadata(metaStr)
		if err != nil {
			logger.Panicf("failed to unmarshal cite annotation. ns:%s, svc:%s, err:%v",
				svc.Namespace, svc.Name, err)
		}

		switch *event.State {
		case "pending":
			msg := fmt.Sprintf(`build started: %s/%s/%s:%s
* buildbot url: %s`, ownerName, repoName, branchName, *event.SHA, *event.TargetURL)
			noti.SendWithFallback(meta.Notification, meta.Watchcenter, msg)
			return c.String(http.StatusOK, "status/pending event received")
		case "success":
			imageName, err := buildbotClient.GetImageName(*event.Description)
			if err != nil {
				logger.Info(err.Error())
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}

			msg := fmt.Sprintf("build success. image name: %s", imageName)
			noti.SendWithFallback(meta.Notification, meta.Watchcenter, msg)

			if meta.AutoDeploy {
				deployer := goroutines.NewDeployer()
				go deployer.Deploy(meta, *event.SHA, imageName, -1)
			}

			return c.String(http.StatusOK, "status/success event received")
		case "error", "failure":
			logURL, err := buildbotClient.GetLogURL(*event.Description)
			if err != nil {
				logger.Info(err.Error())
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}

			logContent, err := buildbotClient.GetLogContent(logURL)
			if err != nil {
				logger.Info(err.Error())
				return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
			}

			msg := fmt.Sprintf(`build failed.
* buildbot url: %s
* lastlog
%s`, *event.TargetURL, logContent)
			noti.SendWithFallback(meta.Notification, meta.Watchcenter, msg)
			return c.String(http.StatusOK, "status/failure event received")
		default:
			errMsg := fmt.Sprintf("unknown status: %v", event.State)
			logger.Warning(errMsg)
			return echo.NewHTTPError(http.StatusNotImplemented, errMsg)
		}

	case "deployment":
		var event githubClient.Event
		if err := json.Unmarshal(body, &event); err != nil {
			errMsg := fmt.Sprintf("error while unmarshalling event:%v, %v", body, err)
			logger.Error(errMsg)
			return echo.NewHTTPError(http.StatusBadRequest, errMsg)
		}
		logger.Debug(event)
		return c.String(http.StatusOK, "deployment event received")

	case "deployment_status":
		var event githubClient.Event
		if err := json.Unmarshal(body, &event); err != nil {
			errMsg := fmt.Sprintf("error while unmarshalling event:%v, %v", body, err)
			logger.Error(errMsg)
			return echo.NewHTTPError(http.StatusBadRequest, errMsg)
		}
		logger.Debug(event)
		return c.String(http.StatusOK, "deployment_status event received")

	case "ping":
		logger.Debug("ping event received")
		return c.String(http.StatusOK, "pong")

	default:
		errMsg := fmt.Sprintf("unknown github event:%v", githubEvent)
		logger.Warning(errMsg)
		return echo.NewHTTPError(http.StatusNotImplemented, errMsg)
	}
}

func clearJSONRepoOrgField(body io.Reader) []byte {
	// workaround for https://github.com/google/go-github/issues/131
	var o map[string]interface{}
	dec := json.NewDecoder(body)
	dec.UseNumber()
	dec.Decode(&o)
	if o != nil {
		repo := o["repository"]
		if repo != nil {
			if repo, ok := repo.(map[string]interface{}); ok {
				delete(repo, "organization")
			}
		}
	}
	b, _ := json.MarshalIndent(o, "", "  ")
	return b
}
