package controller

import (
	"fmt"
	"github.com/labstack/echo"
	"github.com/kakao/cite/models"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/sets"
	"net/http"
	"sort"
	"strconv"
	"time"
)

func GetIndex(c echo.Context) error {
	session := getSession(c)
	token := session.Values["token"].(string)
	userLogin := session.Values["userLogin"].(string)

	githubClient := models.NewGitHub(token)
	orgs, err := githubClient.ListOrgs()
	if err != nil {
		errMsg := fmt.Sprintf("error while getting github organizations: %v", err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	nss, err := k8s.GetAllNamespaces()
	if err != nil {
		errMsg := fmt.Sprintf("error while getting all namespaces: %v", err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	svcRequirement, _ := labels.NewRequirement("type", labels.DoesNotExistOperator, sets.NewString())
	svcs, err := k8s.GetAllServices(util.NormalizeByHyphen("", userLogin), *svcRequirement)
	if err != nil {
		errMsg := fmt.Sprintf("error while getting all services: %v", err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	return c.Render(http.StatusOK, "index",
		map[string]interface{}{
			"orgs": orgs,
			"nss":  nss,
			"svcs": svcs,
		})
}

func GetDeployLog(c echo.Context) error {
	session := getSession(c)
	token := session.Values["token"].(string)

	githubOrg := c.Param("github_org")
	githubRepo := c.Param("github_repo")
	deployID, err := strconv.Atoi(c.Param("deploy_id"))
	if deployID <= 0 || err != nil {
		errMsg := fmt.Sprintf("invalid deploy id %v: %v", deployID, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	githubClient := models.NewGitHub(token)
	statuses, err := githubClient.ListDeploymentStatuses(githubOrg, githubRepo, deployID)
	if err != nil {
		errMsg := fmt.Sprintf("failed to list deployment statuses from github %s/%s: %v", githubOrg, githubRepo, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}
	sort.Sort(SortGithubDeploymentStatusesByCreatedAt(statuses))
	from := statuses[0].CreatedAt.Time
	to := statuses[len(statuses)-1].CreatedAt.Time.Add(10 * time.Second)

	if len(statuses) <= 1 {
		to = from.Add(1 * time.Hour)
	}

	return c.Redirect(http.StatusFound, es.GetDeployLogURL(deployID, from.In(GMT).Format(time.RFC3339), to.In(GMT).Format(time.RFC3339)))
}

func GetProfileSettings(c echo.Context) error {
	session := getSession(c)
	token := session.Values["token"].(string)

	return c.Render(http.StatusOK, "profile_settings",
		map[string]interface{}{
			"token": token,
		})
}
