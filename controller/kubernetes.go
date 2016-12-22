package controller

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/go-github/github"
	"github.com/kakao/cite/goroutines"
	"github.com/kakao/cite/models"
	"github.com/labstack/echo"
	gologging "github.com/op/go-logging"
	k8sApi "k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/sets"
)

func GetNewService(c echo.Context) error {
	session := getSession(c)
	token := session.Values["token"].(string)
	form := new(models.Metadata)

	if len(c.QueryParam("base_ns")) > 0 &&
		len(c.QueryParam("base_svc")) > 0 &&
		len(c.QueryParam("branch")) > 0 {
		// copy base service metadata
		baseNsName := c.QueryParam("base_ns")
		baseSvcName := c.QueryParam("base_svc")
		branch := c.QueryParam("branch")

		_, meta, err := k8s.GetService(baseNsName, baseSvcName)
		if err != nil {
			errMsg := fmt.Sprintf("error while getting base service: %v", err)
			logger.Error(errMsg)
			return echo.NewHTTPError(http.StatusServiceUnavailable, errMsg)
		}
		form = meta
		form.GitBranch = branch
	} else {
		// set default values
		form.AutoDeploy = true
		form.ProbePath = "/"
		form.Replicas = 2
		form.Environment = fmt.Sprintf(`## this is comment
## usage : KEY=VALUE
CITE_VERSION=%s`, models.Conf.Cite.Version)
		form.Notification = []models.Notification{}
		if len(models.Conf.Notification.Watchcenter.API) > 0 {
			form.Notification = append(form.Notification, models.Notification{
				Driver: "watchcenter",
			})
		}
		if len(models.Conf.Notification.Slack.ClientID) > 0 && len(models.Conf.Notification.Slack.ClientSecret) > 0 {
			form.Notification = append(form.Notification, models.Notification{
				Driver: "slack",
			})
		}
	}

	if form.GithubOrg != "" && form.GithubRepo != "" && form.GitBranch != "" {
		nsName := util.NormalizeByHyphen("", form.GithubOrg)
		svcLabels := k8s.GetLabels(form.GithubRepo, form.GitBranch)
		svcs, err := k8s.GetServices(nsName, svcLabels)
		if err != nil {
			errMsg := fmt.Sprintf("failed to query services: %v", err)
			logger.Error(errMsg)
			return echo.NewHTTPError(http.StatusBadRequest, errMsg)
		}
		if len(svcs) == 1 {
			return c.Redirect(http.StatusFound,
				fmt.Sprintf("/namespaces/%s/services/%s", nsName, svcs[0].Name))
		}
	}

	githubClient := models.NewGitHub(token)
	orgs, err := githubClient.ListOrgs()
	if err != nil {
		errMsg := fmt.Sprintf("error while getting github organizations: %v", err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusServiceUnavailable, errMsg)
	}

	return c.Render(http.StatusOK, "new",
		map[string]interface{}{
			"form": form,
			"orgs": orgs,
		})
}

func PostNewService(c echo.Context) error {
	session := getSession(c)
	token := session.Values["token"].(string)
	form := new(models.Metadata)

	onError := func(errMsg string) error {
		logger.Warning(errMsg)
		session.AddFlash(errMsg)
		saveSession(session, c)

		githubClient := models.NewGitHub(token)
		orgs, _ := githubClient.ListOrgs()
		return c.Render(http.StatusBadRequest, "new",
			map[string]interface{}{
				"form": form,
				"orgs": orgs,
			})
	}

	if err := c.Bind(form); err != nil {
		errMsg := fmt.Sprintf("error while parsing form %v, %v", form, err)
		return onError(errMsg)
	}

	// TODO: remove this. backward compatibility : fill watchcenter
	for _, noti := range form.Notification {
		if noti.Driver == "watchcenter" && len(noti.Endpoint) > 0 {
			form.Watchcenter, err = strconv.Atoi(noti.Endpoint)
			if err != nil {
				errMsg := fmt.Sprintf("error while decoding watchcenter id %v: %v", noti.Endpoint, err)
				return onError(errMsg)
			}
		}
	}

	// print form input for debug
	if logger.IsEnabledFor(gologging.DEBUG) {
		formJson, _ := json.MarshalIndent(form, "", "  ")
		logger.Debugf("form: %s", formJson)
	}

	// validate number of replicas
	if form.Replicas <= 0 || form.Replicas > models.Conf.Kubernetes.MaxPods {
		errMsg := fmt.Sprintf("invalid replicas : %d", form.Replicas)
		return onError(errMsg)
	}

	// calculate ports (TODO: make better ports UI)
	httpPorts, err := util.TCPPortsToList(form.HTTPPort)
	if err != nil {
		errMsg := fmt.Sprintf("invalid http port: %v", err)
		return onError(errMsg)
	}
	tcpPorts, err := util.TCPPortsToList(form.TCPPort)
	if err != nil {
		errMsg := fmt.Sprintf("invalid tcp port: %v", err)
		return onError(errMsg)
	}
	ports := append(httpPorts, tcpPorts...)
	if len(ports) < 1 {
		errMsg := "container port required"
		return onError(errMsg)
	}

	// validate service name
	if !unicode.IsLetter(rune(form.Service[0])) {
		errMsg := "invalid service name: service name starts with [a-z]"
		return onError(errMsg)
	}
	if len(form.Service) > 24 {
		errMsg := "invalid service name: service name too long (max. 24 chars)"
		return onError(errMsg)
	}

	nsName := util.NormalizeByHyphen("", form.GithubOrg)
	// check if service already exist
	if _, _, err := k8s.GetService(nsName, form.Service); err == nil {
		errMsg := fmt.Sprintf("service %s already exist", form.Service)
		return onError(errMsg)
	}

	// ensure namespace exist
	err = k8s.UpsertNamespace(nsName)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to create kubernetes namespace: %v", err)
		return onError(errMsg)
	}

	// ensure kibana index
	err = es.UpsertKibanaIndexPattern(nsName)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to create elasticsearch kibana index for namespace: %v", err)
		return onError(errMsg)
	}

	githubClient := models.NewGitHub(token)
	repo, err := githubClient.GetRepo(form.GithubOrg, form.GithubRepo)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to get repository metadata from github %s/%s: %v", form.GithubOrg, form.GithubRepo, err)
		return onError(errMsg)
	}

	// check if user has push permission on repository
	if perm, ok := (*repo.Permissions)["push"]; !ok || !perm {
		errMsg := fmt.Sprintf("You don't have push permission on %s/%s.", form.GithubOrg, form.GithubRepo)
		return onError(errMsg)
	}

	// check if Dockerfile exists in repository
	hasDockerfile, err := githubClient.CheckDockerfile(form.GithubOrg, form.GithubRepo)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to check repository Dockerfile: %v", err)
		return onError(errMsg)
	}
	if !hasDockerfile {
		errMsg := fmt.Sprintf("Your repository %s/%s doesn't have /Dockerfile on any branch. Please create one",
			form.GithubOrg, form.GithubRepo)
		return onError(errMsg)
	}

	// ensure github hook
	err = githubClient.UpsertHook(form.GithubOrg, form.GithubRepo)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to create github hook on %s/%s: %v", form.GithubOrg, form.GithubRepo, err)
		return onError(errMsg)
	}

	// ensure github collaborator
	err = githubClient.AddCollaborator(form.GithubOrg, form.GithubRepo, models.Conf.GitHub.Username)
	if err != nil {
		errMsg := fmt.Sprintf("Failed to add github collaborator %s on %s/%s: %v",
			models.Conf.GitHub.Username, form.GithubOrg, form.GithubRepo, err)
		return onError(errMsg)
	}

	// upsert k8s service
	svcLabels := k8s.GetLabels(form.GithubRepo, form.GitBranch)
	svcSelector := make(map[string]string)
	for k, v := range svcLabels {
		svcSelector[k] = v
	}
	svc, err := k8s.UpsertService(nsName, form.Service, svcLabels, svcSelector, form.Marshal(), ports)
	if err != nil {
		errMsg :=
			fmt.Sprintf("error while creating kubernetes service: %s/%s:%s, %v", nsName, form.GithubRepo, form.GitBranch, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusServiceUnavailable, errMsg)
	}

	return c.Redirect(http.StatusFound,
		fmt.Sprintf("/namespaces/%s/services/%s", nsName, svc.Name))
}

func DeleteService(c echo.Context) error {
	reqType := c.Param("type")
	nsName := c.Param("nsName")
	name := c.Param("name")
	logger.Info(fmt.Sprintf("delete request. type:%s namespace:%s name:%s", reqType, nsName, name))
	redirectURL := c.Request().Referer()

	switch reqType {
	case "ns":
		redirectURL = "/"
		err := k8s.DeleteNamespace(nsName)
		if err != nil {
			errMsg := fmt.Sprintf("failed to delete k8s namespace %s: %v", nsName, err)
			logger.Error(errMsg)
			return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
		}

	case "svc":
		redirectURL = "/namespaces/" + nsName
		err = k8s.DeleteService(nsName, name)
		if err != nil {
			errMsg := fmt.Sprintf("failed to delete k8s service %s/%s: %v", nsName, name, err)
			logger.Error(errMsg)
			return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
		}

	case "rc":
		err := k8s.DeleteReplicationController(nsName, name)
		if err != nil {
			errMsg := fmt.Sprintf("failed to delete k8s rc %s/%s: %v", nsName, name, err)
			logger.Error(errMsg)
			return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
		}

	case "po":
		err := k8s.DeletePod(nsName, name)
		if err != nil {
			errMsg := fmt.Sprintf("failed to delete k8s pod %s/%s: %v", nsName, name, err)
			logger.Error(errMsg)
			return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
		}
	}

	return c.Redirect(http.StatusFound, redirectURL)
}

func PutScale(c echo.Context) error {
	nsName := c.Param("nsName")
	svcName := c.Param("svcName")
	rcName := c.Param("rcName")
	replicas := 2
	if c.Param("replicas") != "" {
		replicas, err = strconv.Atoi(c.Param("replicas"))
		if err != nil {
			errMsg := fmt.Sprintf("failed to parse replicas: %v", err)
			logger.Error(errMsg)
			return echo.NewHTTPError(http.StatusBadRequest, errMsg)
		}
	}

	logger.Info(fmt.Sprintf("scale request. ns:%s, svc:%s, rc:%s, replicas:%d", nsName, svcName, rcName, replicas))

	_, err = k8s.ScaleReplicationController(nsName, rcName, replicas)
	if err != nil {
		errMsg := fmt.Sprintf("failed to scale k8s replication controller %s/%s: %v", nsName, rcName, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	svc, meta, err := k8s.GetService(nsName, svcName)
	if err != nil {
		errMsg := fmt.Sprintf("error while getting service from kubernetes %s/%s: %v", nsName, svcName, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}
	meta.Replicas = replicas
	svc.Annotations[models.CITE_K8S_ANNOTATION_KEY] = meta.Marshal()
	svc, err = k8s.UpdateService(nsName, svc)
	if err != nil {
		errMsg := fmt.Sprintf("failed to update service metadata %s/%s: %v", nsName, svcName, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	return c.Redirect(http.StatusFound, c.Request().Referer())
}

func GetNamespaces(c echo.Context) error {
	nss, err := k8s.GetAllNamespaces()
	if err != nil {
		errMsg := fmt.Sprintf("error while getting all namespaces: %v", err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	return c.Render(http.StatusOK, "namespaces",
		map[string]interface{}{
			"nss": nss,
		})
}

func GetNamespace(c echo.Context) error {
	nsName := c.Param("namespace")

	svcRequirement, _ := labels.NewRequirement("type", labels.DoesNotExistOperator, sets.NewString())
	svcs, err := k8s.GetAllServices(nsName, *svcRequirement)
	if err != nil {
		errMsg := fmt.Sprintf("error while getting all services at %s: %v", nsName, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	return c.Render(http.StatusOK, "services",
		map[string]interface{}{
			"nsName": nsName,
			"svcs":   svcs,
		})
}

func GetService(c echo.Context) error {
	session := getSession(c)
	token := session.Values["token"].(string)
	nsName := c.Param("namespace")
	svcName := c.Param("service")

	svc, meta, err := k8s.GetService(nsName, svcName)
	if err != nil {
		errMsg := fmt.Sprintf("error while getting service from kubernetes %s/%s: %v", nsName, svcName, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	rcSelector := make(map[string]string)
	for k, v := range svc.Spec.Selector {
		if k != "sha" && k != "deploy_id" && k != "loadbalancer" {
			rcSelector[k] = strings.TrimSpace(strings.ToLower(v))
		}
	}

	rcRequirement, _ := labels.NewRequirement("type", labels.DoesNotExistOperator, sets.NewString())
	rcs, err := k8s.GetReplicationControllers(nsName, rcSelector, *rcRequirement)
	if err != nil {
		errMsg := fmt.Sprintf("error while getting replication controllers from kubernetes %s/%v: %v", nsName, rcSelector, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	var (
		activeRC    k8sApi.ReplicationController
		inactiveRCs []k8sApi.ReplicationController
	)

	deployID, _ := svc.Spec.Selector["deploy_id"]
	for _, rc := range rcs {
		if di, ok := rc.Labels["deploy_id"]; ok && di == deployID {
			activeRC = rc
		} else {
			inactiveRCs = append(inactiveRCs, rc)
		}
	}

	githubOrg := meta.GithubOrg
	githubRepo := meta.GithubRepo
	gitBranch := meta.GitBranch

	githubClient := models.NewGitHub(token)
	branches, err := githubClient.ListBranches(githubOrg, githubRepo, &github.ListOptions{
		PerPage: 100,
	})
	if err != nil {
		errMsg := fmt.Sprintf("error while listing branch from github %s/%s: %v", githubOrg, githubRepo, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	commits, err := githubClient.ListCommits(githubOrg, githubRepo, &github.CommitsListOptions{
		SHA: gitBranch,
		ListOptions: github.ListOptions{
			PerPage: 4,
		},
	})
	if err != nil {
		errMsg := fmt.Sprintf(
			"error while getting commits from github. k8s: %s/%s, github:%s/%s/%s: %v",
			nsName, svcName, githubOrg, githubRepo, gitBranch, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	deployments, err := githubClient.ListDeployments(githubOrg, githubRepo, &github.DeploymentsListOptions{
		Ref: gitBranch,
		ListOptions: github.ListOptions{
			PerPage: 4,
		},
	})
	if err != nil {
		errMsg := fmt.Sprintf(
			"error while getting deployments from github. k8s: %s/%s, github:%s/%s/%s: %v",
			nsName, svcName, githubOrg, githubRepo, gitBranch, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	data := make(map[string]interface{})
	data["nsName"] = nsName
	data["svcName"] = svcName
	data["meta"] = meta

	data["githubOrg"] = githubOrg
	data["githubRepo"] = githubRepo
	data["gitBranch"] = gitBranch
	data["branches"] = branches
	data["commits"] = commits
	data["deployments"] = deployments
	data["sha"] = svc.Spec.Selector["sha"]

	data["svc"] = svc
	if activeRC.Name != "" {
		data["rc"] = activeRC
	}
	data["inactiveRCs"] = inactiveRCs

	return c.Render(http.StatusOK, "service", data)
}

func GetGitHubCommits(c echo.Context) error {
	session := getSession(c)
	token := session.Values["token"].(string)
	nsName := c.Param("namespace")
	svcName := c.Param("service")

	svc, meta, err := k8s.GetService(nsName, svcName)
	if err != nil {
		errMsg := fmt.Sprintf("error while getting service from kubernetes %s/%s: %v", nsName, svcName, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	githubClient := models.NewGitHub(token)
	sha := svc.Spec.Selector["sha"]

	commits, err := githubClient.ListCommits(
		meta.GithubOrg,
		meta.GithubRepo,
		&github.CommitsListOptions{SHA: meta.GitBranch})
	if err != nil {
		errMsg := fmt.Sprintf(
			"error while getting commits from github. k8s: %s/%s, github:%s/%s/%s: %v",
			nsName, svcName, meta.GithubOrg, meta.GithubRepo, meta.GitBranch, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	return c.Render(http.StatusOK, "github_commit",
		map[string]interface{}{
			"nsName":     nsName,
			"svcName":    svcName,
			"githubOrg":  meta.GithubOrg,
			"githubRepo": meta.GithubRepo,
			"gitBranch":  meta.GitBranch,
			"sha":        sha,
			"commits":    commits,
		})
}

func GetGitHubDeployments(c echo.Context) error {
	session := getSession(c)
	token := session.Values["token"].(string)
	nsName := c.Param("namespace")
	svcName := c.Param("service")

	svc, meta, err := k8s.GetService(nsName, svcName)
	if err != nil {
		errMsg := fmt.Sprintf("error while getting service from kubernetes %s/%s: %v", nsName, svcName, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	githubClient := models.NewGitHub(token)
	sha := svc.Spec.Selector["sha"]

	deployments, err := githubClient.ListDeployments(
		meta.GithubOrg,
		meta.GithubRepo,
		&github.DeploymentsListOptions{Ref: meta.GitBranch})
	if err != nil {
		errMsg := fmt.Sprintf(
			"error while getting deployments from github. k8s: %s/%s, github:%s/%s/%s: %v",
			nsName, svcName, meta.GithubOrg, meta.GithubRepo, meta.GitBranch, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	return c.Render(http.StatusOK, "github_deployment",
		map[string]interface{}{
			"nsName":      nsName,
			"svcName":     svcName,
			"githubOrg":   meta.GithubOrg,
			"githubRepo":  meta.GithubRepo,
			"gitBranch":   meta.GitBranch,
			"sha":         sha,
			"deployments": deployments,
		})
}

func GetServiceSettings(c echo.Context) error {
	nsName := c.Param("namespace")
	svcName := c.Param("service")
	form := new(models.Metadata)

	_, meta, err := k8s.GetService(nsName, svcName)
	if err != nil {
		errMsg := fmt.Sprintf("error while getting service from kubernetes %s/%s: %v", nsName, svcName, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	form = meta
	form.Namespace = nsName
	form.Service = svcName

	return c.Render(http.StatusOK, "settings",
		map[string]interface{}{
			"form":    form,
			"nsName":  nsName,
			"svcName": svcName,
		})
}

func PostServiceSettings(c echo.Context) error {
	nsName := c.Param("namespace")
	svcName := c.Param("service")
	form := new(models.Metadata)

	onError := func(errMsg string) error {
		logger.Warning(errMsg)
		session := getSession(c)
		session.AddFlash(errMsg)
		saveSession(session, c)
		return c.Render(http.StatusBadRequest, "new",
			map[string]interface{}{
				"form":    form,
				"nsName":  nsName,
				"svcName": svcName,
			})
	}

	if err := c.Bind(form); err != nil {
		errMsg := fmt.Sprintf("error while parsing form %v, %v", form, err)
		return onError(errMsg)
	}

	// TODO: remove this. backward compatibility : fill watchcenter
	for _, noti := range form.Notification {
		if noti.Driver == "watchcenter" && len(noti.Endpoint) > 0 {
			form.Watchcenter, err = strconv.Atoi(noti.Endpoint)
			if err != nil {
				errMsg := fmt.Sprintf("error while decoding watchcenter id %v: %v", noti.Endpoint, err)
				return onError(errMsg)
			}
		}
	}

	// print form input for debug
	if logger.IsEnabledFor(gologging.DEBUG) {
		formJSON, _ := json.MarshalIndent(form, "", "  ")
		logger.Debugf("form: %s", formJSON)
	}

	// validate number of replicas
	if form.Replicas <= 0 || form.Replicas > models.Conf.Kubernetes.MaxPods {
		errMsg := fmt.Sprintf("invalid replicas : %d", form.Replicas)
		return onError(errMsg)
	}

	svc, _, err := k8s.GetService(nsName, svcName)
	if err != nil {
		errMsg := fmt.Sprintf("error while getting service from kubernetes %s/%s: %v", nsName, svcName, err)
		return onError(errMsg)
	}

	svc.Annotations[models.CITE_K8S_ANNOTATION_KEY] = form.Marshal()

	if _, err := k8s.UpdateService(nsName, svc); err != nil {
		errMsg := fmt.Sprintf("error while update service metadata %v, %v", form.Marshal(), err)
		return onError(errMsg)
	}

	return c.Redirect(http.StatusFound,
		fmt.Sprintf("/namespaces/%s/services/%s", nsName, svcName))
}

func PostBuild(c echo.Context) error {
	nsName := c.Param("namespace")
	svcName := c.Param("service")
	sha := c.Param("sha")

	_, meta, err := k8s.GetService(nsName, svcName)
	if err != nil {
		errMsg := fmt.Sprintf("error while getting service from kubernetes %s/%s: %v", nsName, svcName, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	err = buildbotClient.Build(nsName, meta.GithubRepo, meta.GitBranch, sha)
	if err != nil {
		errMsg := fmt.Sprintf("failed to send event to buildbot: %v", err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	session := getSession(c)
	session.AddFlash("build started")
	saveSession(session, c)
	return c.Redirect(http.StatusFound, c.Request().Referer())
}

func PostDeploy(c echo.Context) error {
	session := getSession(c)
	token := session.Values["token"].(string)
	nsName := c.Param("namespace")
	svcName := c.Param("service")
	sha := c.Param("sha")
	imageName := c.QueryParam("imageName")
	if imageName == "" {
		errMsg := "imageName required."
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	_, meta, err := k8s.GetService(nsName, svcName)
	if err != nil {
		errMsg := fmt.Sprintf("error while getting service from kubernetes %s/%s: %v", nsName, svcName, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	githubClient := models.NewGitHub(token)
	deployID, err := githubClient.CreateDeployment(
		meta.GithubOrg,
		meta.GithubRepo,
		meta.GitBranch,
		"manual deploy")
	if err != nil {
		errMsg := fmt.Sprintf(
			"error while create deployments to github:%s/%s/%s: %v",
			meta.GithubOrg, meta.GithubRepo, meta.GitBranch, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	deployer := goroutines.NewDeployer()
	go deployer.Deploy(meta, sha, imageName, deployID)

	return c.Redirect(http.StatusFound, es.GetDeployLogURL(deployID, "now-1h", "now"))
}

func PutActivate(c echo.Context) error {
	nsName := c.Param("namespace")
	svcName := c.Param("service")
	sha := c.Param("sha")
	deployID := c.Param("deploy_id")

	svc, _, err := k8s.GetService(nsName, svcName)
	if err != nil {
		errMsg := fmt.Sprintf("failed to get kubernetes service %s/%s: %v", nsName, svcName, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}
	svc.Spec.Selector["sha"] = sha
	svc.Spec.Selector["deploy_id"] = deployID
	svc, err = k8s.UpdateService(nsName, svc)
	if err != nil {
		errMsg := fmt.Sprintf("failed to update service metadata %s/%s: %v", nsName, svcName, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	return c.Redirect(http.StatusFound, c.Request().Referer())
}
