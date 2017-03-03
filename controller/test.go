package controller

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"

	"github.com/deckarep/golang-set"
	"github.com/gorilla/schema"
	"github.com/kakao/cite/models"
	"github.com/labstack/echo"
)

func GetSession(c echo.Context) error {
	session := getSession(c)
	return c.String(http.StatusOK, fmt.Sprintf("%v", session.Values))
}

func PostSession(c echo.Context) error {
	session := getSession(c)
	session.Values["aa"] = 111
	session.Values["bb"] = "222"
	err := saveSession(session, c)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.String(http.StatusOK, "session stored")
}

func DeleteSession(c echo.Context) error {
	err := destroySession(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.String(http.StatusOK, "session destroyed")
}

func GetAce(c echo.Context) error {
	return c.Render(http.StatusOK, "test",
		map[string]interface{}{
			"aa": 11,
			"bb": "XXYY",
		})
}

func GetGithub(c echo.Context) error {
	owner := c.QueryParam("owner")
	if owner == "" {
		owner = "niko-bellic"
	}
	repo := c.QueryParam("repo")
	if repo == "" {
		repo = "helloworld2"
	}
	github := models.NewCommonGitHub()
	resp, err := github.CheckDockerfile(owner, repo)
	if err != nil {
		return err
	}

	return c.String(http.StatusOK, strconv.FormatBool(resp))
}

func GetGithubCommit(c echo.Context) error {
	owner := "niko-bellic"
	repo := "helloworld"
	sha := "e52440a4e8b511a2d9a998e1048f56191911e322"
	//sha := "f3243d881a01ca096550ab8b9cbe98b21b02b891"

	gh := models.NewCommonGitHub()
	commit, err := gh.GetCommit(owner, repo, sha)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, commit)
}

func GetGithubHook(c echo.Context) error {
	owner := c.QueryParam("owner")
	if owner == "" {
		owner = "niko-bellic"
	}
	repo := c.QueryParam("repo")
	if repo == "" {
		repo = "helloworld"
	}
	//	github := models.NewCommonGitHub()
	githubClient := models.NewGitHub("e5d957deccc20599413729b71ba9bdff040df71f")
	// githubClient := models.NewCommonGitHub()
	out, err := githubClient.ListHooks(owner, repo)
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, out)
}

func PostGithubHook(c echo.Context) error {
	owner := c.QueryParam("owner")
	if owner == "" {
		owner = "niko-bellic"
	}
	repo := c.QueryParam("repo")
	if repo == "" {
		repo = "helloworld"
	}
	//	github := models.NewCommonGitHub()
	githubClient := models.NewGitHub("9888a649fc5423e5c407643a177ba53b42450403")
	err := githubClient.UpsertHook(owner, repo)
	if err != nil {
		return err
	}

	return c.String(http.StatusOK, "OK")
}

func GetMetadata(c echo.Context) error {
	srcStr := "{\"source\":\"github\",\"namespace\":\"\",\"service\":\"docker-test\",\"github_org\":\"niko-bellic\",\"github_repo\":\"test\",\"git_branch\":\"docker\",\"image_name\":\"\",\"image_tag\":\"\",\"auto_deploy\":true,\"http_port\":\"80, 8080, 4560, 4561\",\"tcp_port\":\"\",\"probe_path\":\"\",\"replicas\":2,\"watchcenter\":1047,\"environment\":\"## this is comment\\r\\n## usage : KEY=VALUE\\r\\nCITE_VERSION=DEV\"}"

	meta, err := models.UnmarshalMetadata(srcStr)
	if err != nil {
		logger.Errorf("error while getting services: %v", err)
	}
	logger.Infof("json: %s", meta.Marshal())
	return c.JSON(http.StatusOK, *meta)
}

func GetGithubHookPatch(c echo.Context) error {
	// nsName := api.NamespaceAll
	// nsName := "kemi"
	nsName := "niko-bellic"
	type Metadata struct {
		GithubOrg  string `json:"github_org" form:"github_org"`
		GithubRepo string `json:"github_repo" form:"github_repo"`
	}
	logger.Info(models.Conf.GitHub.AccessToken)

	svcs, err := k8s.GetAllServices(nsName)
	if err != nil {
		logger.Errorf("error while getting services: %v", err)
	}

	metas := make(map[Metadata]struct{})
	for _, svc := range svcs {
		var annotation string
		var ok bool
		if annotation, ok = svc.Annotations[models.CITE_K8S_ANNOTATION_KEY]; !ok {
			continue
		}
		meta := new(Metadata)
		err = json.Unmarshal([]byte(annotation), meta)
		if err != nil {
			logger.Errorf("error while unmarshalling metadata: %v", err)
		}
		metas[*meta] = struct{}{}
	}

	githubClient := models.NewCommonGitHub()
	for meta := range metas {
		logger.Infof("org: %s, repo: %s", meta.GithubOrg, meta.GithubRepo)
		err := githubClient.UpsertHook(meta.GithubOrg, meta.GithubRepo)
		if err != nil {
			return err
		}
	}

	return c.JSON(http.StatusOK, "OK")
}

func PostGithubHookProxy(c echo.Context) error {
	buildbot := models.NewBuildBot()
	reqBody, _ := ioutil.ReadAll(c.Request().Body)
	return buildbot.Proxy(c.Request().Method, c.Request().Header, reqBody)
}

func PostGithubCollaborator(c echo.Context) error {
	owner := c.QueryParam("owner")
	if owner == "" {
		owner = "niko-bellic"
	}
	repo := c.QueryParam("repo")
	if repo == "" {
		repo = "helloworld"
	}
	isOrgParam := c.QueryParam("is_org")
	if isOrgParam == "" {
		isOrgParam = "false"
	}
	isOrg, err := strconv.ParseBool(isOrgParam)
	if err != nil {
		return err
	}

	logger.Info("owner", owner)
	logger.Info("repo", repo)
	logger.Info("is_org", isOrg)
	githubClient := models.NewGitHub("9888a649fc5423e5c407643a177ba53b42450403")

	err = githubClient.AddCollaborator(owner, repo, "infra")
	if err != nil {
		return err
	}

	return c.String(http.StatusOK, "OK")
}

func GetRoute(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{
		"a":           c.Param("a"),
		"_*":          c.Param("_*"),
		"ParamNames":  fmt.Sprintf("%v", c.ParamNames()),
		"ParamValues": fmt.Sprintf("%v", c.ParamValues()),
	})
}

func GetRouteA(c echo.Context) error {
	return c.String(http.StatusOK, "Route A")
}

func GetRouteAB(c echo.Context) error {
	return c.String(http.StatusOK, "Route A B")
}

func GetRouteABC(c echo.Context) error {
	return c.String(http.StatusOK, "Route A B C")
}

func GetConfig(c echo.Context) error {
	return c.JSON(http.StatusOK, models.Conf)
}

func GetLogging(c echo.Context) error {
	logger.Debug("hello")
	logger.Info("world")
	return c.String(http.StatusOK, "OK")
}

func GetBuildbot(c echo.Context) error {
	logID := c.QueryParam("logid")
	if logID == "" {
		logID = "344"
	}
	bb := models.NewBuildBot()
	log, err := bb.GetLogContent(
		fmt.Sprintf(models.Conf.Buildbot.Host+"/api/v2/logs/%s/contents", logID))
	if err != nil {
		return err
	}

	wt := models.NewWatchCenter()
	wt.SendGroupTalk(1047, log)
	return c.String(http.StatusOK, log)
}

func GetNotifier(c echo.Context) error {
	noti := models.NewNotifier()
	_, meta, err := k8s.GetService("code0x9", "develop-helloworld")
	if err != nil {
		return err
	}

	err = noti.Send(meta.Notification, "testing....")
	if err != nil {
		return err
	}
	return c.String(http.StatusOK, "SENT!")
}

func GetSystemNotifier(c echo.Context) error {
	noti := models.NewNotifier()
	err = noti.SendSystem("testing system message....")
	if err != nil {
		return err
	}
	return c.String(http.StatusOK, "System Message SENT!")
}

func PostBuildbot(c echo.Context) error {
	owner := "code0x9"
	repo := "helloworld"
	branch := "develop"
	sha := "b2c295f7ea3c5cf79de51c5f57b9eeb71a493e25"

	bb := models.NewBuildBot()
	err = bb.Build(owner, repo, branch, sha)
	if err != nil {
		return err
	}

	return c.String(http.StatusOK, "OK")
}

func GetEnvironment(c echo.Context) error {
	return c.JSON(http.StatusOK, os.Environ())
}

func GetDocker(c echo.Context) error {
	owner := c.QueryParam("owner")
	if owner == "" {
		owner = "niko-bellic"
	}
	repo := c.QueryParam("repo")
	if repo == "" {
		repo = "helloworld"
	}
	github := models.NewCommonGitHub()

	resp, err := github.CheckDockerfile(owner, repo)
	if err != nil {
		return err
	}

	return c.String(http.StatusOK, strconv.FormatBool(resp))
}

func PostKibana(c echo.Context) error {
	namespace := c.QueryParam("namespace")
	if namespace == "" {
		namespace = "niko-bellic"
	}
	es := models.NewElastic()
	err = es.UpsertKibanaIndexPattern(namespace)
	if err != nil {
		return err
	}

	return c.String(http.StatusOK, "OK")
}

func GetError(c echo.Context) error {
	err := fmt.Errorf("testing error!!!")
	return echo.NewHTTPError(http.StatusConflict, err.Error())
}

func GetPanic(c echo.Context) error {
	err := fmt.Errorf("testing panic!!!")
	panic(err)
	return err
}

func GetGoroutinePanic(c echo.Context) error {
	err := fmt.Errorf("testing panic!!!")
	go func() {
		panic(err)
	}()
	return err
}

func GetSet(c echo.Context) error {
	setA := mapset.NewSet()
	setA.Add("80")
	setA.Add("443")
	setB := mapset.NewSet()
	setB.Add("80")
	setB.Add("400")

	logger.Infof("old:%v", setA)
	logger.Infof("new:%v", setB)
	logger.Infof("old-new:%v", setA.Difference(setB))
	logger.Infof("new-old:%v", setB.Difference(setA))

	return c.JSON(http.StatusOK, map[string]interface{}{
		"old":     setA.ToSlice(),
		"new":     setB.ToSlice(),
		"old-new": setA.Difference(setB).ToSlice(),
		"new-old": setB.Difference(setA).ToSlice(),
	})
}

func GetAnnotation(c echo.Context) error {
	nss, err := k8s.GetAllNamespaces()
	if err != nil {
		return err
	}

	output := make(map[string]string)

	for _, ns := range nss {
		svcs, err := k8s.GetAllServices(ns.Name)
		if err != nil {
			return err
		}

		for _, svc := range svcs {
			svcName, ok := svc.Labels["service"]
			if !ok {
				logger.Infof("skip %s", svc.Name)
				continue
			}
			branchName, ok := svc.Labels["branch"]
			if !ok {
				logger.Infof("skip %s", svc.Name)
				continue
			}

			output[fmt.Sprintf("%s/%s/%s", ns.Name, svcName, branchName)] = svc.Annotations[models.CITE_K8S_ANNOTATION_KEY]
			logger.Infof("%s/%s/%s: %v", ns.Name, svcName, branchName, svc.Annotations[models.CITE_K8S_ANNOTATION_KEY])
		}
	}

	return c.JSON(http.StatusOK, output)
}

func GetNormalize(c echo.Context) error {
	src1 := "aaa/bbb"
	src2 := "aaa-bbb"
	src3 := "aaa한글 아아아아 bbb"
	return c.JSON(http.StatusOK, map[string]string{
		"src1":           src1,
		"src1 norm":      util.Normalize("", src1),
		"src1 norm hyph": util.NormalizeByHyphen("", src1),
		"src2":           src2,
		"src2 norm":      util.Normalize("", src2),
		"src2 norm hyph": util.NormalizeByHyphen("", src2),
		"src3":           src3,
		"src3 norm":      util.Normalize("", src3),
		"src3 norm hyph": util.NormalizeByHyphen("", src3),
	})
}

func GetKubernetesService(c echo.Context) error {
	owner := c.QueryParam("owner")
	if owner == "" {
		owner = "niko-bellic"
	}
	repo := c.QueryParam("repo")
	if repo == "" {
		repo = "he-ll-o-world"
	}
	branch := c.QueryParam("branch")
	if branch == "" {
		branch = "master"
	}

	svcLabels := k8s.GetLabels(repo, branch)
	svcs, err := k8s.GetServices(owner, svcLabels)
	if err != nil {
		return err
	}
	if len(svcs) < 1 {
		return fmt.Errorf("svc not found")
	}

	return c.JSON(http.StatusOK, svcs)
}

func GetFormSubmit(c echo.Context) error {
	return c.Render(http.StatusOK, "test", map[string]interface{}{})
}

func PostFormSubmit(c echo.Context) error {
	type SubmitTest struct {
		Aa string `schema:"aa" json:"aa"`
		Bb int    `schema:"bb" json:"bb"`
		Cc string `schema:"cc" json:"cc"`
	}

	req := c.Request()
	if err := req.ParseForm(); err != nil {
		logger.Error("failed to parse form: %v", err)
		return c.Render(http.StatusOK, "test", map[string]interface{}{})
	}

	form := new(SubmitTest)
	decoder := schema.NewDecoder()
	if err := decoder.Decode(form, req.PostForm); err != nil {
		logger.Error("failed to decode form: %v", err)
		return c.Render(http.StatusOK, "test", map[string]interface{}{
			"form": form,
		})
	}

	logger.Infof("form: %v", form)
	return c.JSON(http.StatusOK, form)
}
