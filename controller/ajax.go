package controller

import (
	"fmt"
	"github.com/google/go-github/github"
	"github.com/labstack/echo"
	"github.com/kakao/cite/models"
	"net/http"
)

func GetGithubRepos(c echo.Context) error {
	session := getSession(c)
	token := session.Values["token"].(string)
	orgName := c.QueryParam("github_org")
	githubClient := models.NewGitHub(token)
	user, err := githubClient.GetUser()
	if err != nil {
		errMsg := fmt.Sprintf("error while getting user from github: %v", err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	var repos []github.Repository
	if *user.Login == orgName {
		repos, err = githubClient.ListOwnerRepos()
	} else {
		repos, err = githubClient.ListOrgRepos(orgName)
	}

	if err != nil {
		errMsg := fmt.Sprintf("error while listing repos from %s: %v", orgName, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	repoJSON := make([][]string, len(repos))
	for i, repo := range repos {
		repoJSON[i] = []string{*repo.Name, *repo.Name}
	}
	return c.JSON(http.StatusOK, repoJSON)
}

func GetGithubBranches(c echo.Context) error {
	session := getSession(c)
	token := session.Values["token"].(string)
	orgName := c.QueryParam("github_org")
	repoName := c.QueryParam("github_repo")

	if orgName == "" || repoName == "" {
		return c.JSON(http.StatusBadRequest, [][]string{})
	}

	githubClient := models.NewGitHub(token)
	branches, err := githubClient.ListBranches(orgName, repoName, &github.ListOptions{})
	if err != nil {
		errMsg := fmt.Sprintf("error while listing branches from %s/%s: %v", orgName, repoName, err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	branchesJSON := make([][]string, len(branches))
	for i, branch := range branches {
		branchesJSON[i] = []string{*branch.Name, *branch.Name}
	}
	return c.JSON(http.StatusOK, branchesJSON)
}

func GetDockerTags(c echo.Context) error {
	name := c.QueryParam("name")
	if name == "" {
		return c.JSON(http.StatusBadRequest, []string{})
	}

	tags, err := docker.ListImageTags(name)
	logger.Info(fmt.Sprintf("list tags. name:%v, tags:%v, err:%v", name, tags, err))
	return c.JSON(http.StatusOK, tags)
}
