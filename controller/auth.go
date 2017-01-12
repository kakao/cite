package controller

import (
	"fmt"
	"net/http"

	"github.com/kakao/cite/models"
	"github.com/labstack/echo"
)

func GetLogin(c echo.Context) error {
	return c.Redirect(http.StatusTemporaryRedirect, commonGitHub.GetLoginURL())
}

func GetLogout(c echo.Context) error {
	destroySession(c)
	return c.Render(http.StatusOK, "logout",
		map[string]interface{}{
			"github_host": models.Conf.GitHub.Host,
		})
}

func GetGithubCallback(c echo.Context) error {
	session := getSession(c)

	state := c.QueryParam("state")
	code := c.QueryParam("code")
	token, err := commonGitHub.GetOAuth2Token(state, code)
	if err != nil {
		errMsg := fmt.Sprintf("error while getting oauth2 token from github: %v", err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}

	session.Values["token"] = token

	githubClient := models.NewGitHub(token)
	user, err := githubClient.GetUser()
	if err != nil || user == nil {
		errMsg := fmt.Sprintf("error while getting user from github: %v", err)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusInternalServerError, errMsg)
	}
	session.Values["userLogin"] = *user.Login
	if user.Email != nil {
		session.Values["userEmail"] = *user.Email
	}
	if user.Name != nil {
		session.Values["userName"] = *user.Name
	} else {
		session.Values["userName"] = *user.Login
	}

	redirectPath, ok := session.Values["redirectPath"]
	if !ok || redirectPath == "" {
		redirectPath = "/"
	}

	saveSession(session, c)

	return c.Redirect(http.StatusTemporaryRedirect, redirectPath.(string))
}
