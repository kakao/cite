package controller

import (
	"fmt"
	"net/http"

	"github.com/kakao/cite/models"
	"github.com/labstack/echo"
	"github.com/nlopes/slack"
)

func GetSlackOAuthToken(c echo.Context) error {
	code := c.QueryParam("code")
	if len(code) == 0 {
		// get oauth code
		url := fmt.Sprintf("https://slack.com/oauth/authorize?scope=incoming-webhook,bot&client_id=%s&redirect_uri=%s",
			models.Conf.Notification.Slack.ClientID,
			c.Request().URL.String())
		return c.Redirect(http.StatusFound, url)
	}

	redirectURI := models.Conf.Cite.Host + models.Conf.Cite.ListenPort + models.Conf.Notification.Slack.RedirectURI
	resp, err := slack.GetOAuthResponse(
		models.Conf.Notification.Slack.ClientID,
		models.Conf.Notification.Slack.ClientSecret,
		code,
		redirectURI,
		false,
	)
	if err != nil {
		return err
	}

	return c.Render(http.StatusOK, "notification/slack_popup",
		map[string]interface{}{
			"team":       resp.TeamName,
			"channel":    resp.IncomingWebhook.Channel,
			"webhookURL": resp.IncomingWebhook.URL,
		})
}

func GetSlackSend(c echo.Context) error {
	token := "xoxb-110811105782-mrTss4CmHSyvffXehyZPg22D"
	api := slack.New(token)

	// channel, err := api.GetChannelInfo("#general")
	// logger.Infof("err: %v", err)

	// logger.Infof("channel: %v", channel)
	// logger.Infof("channel id: %v", channel.ID)
	params := slack.PostMessageParameters{}
	// channel, ts, err := api.PostMessage("C37K6B0LD", "hello, there! 한글 메시지다!!", params)
	channel, ts, err := api.PostMessage("C0HFSMUFQ", "hello, there! 한글 메시지다!!", params)
	logger.Infof("err: %v", err)
	logger.Infof("channel: %v", channel)
	logger.Infof("timestamp: %v", ts)

	return fmt.Errorf("not implemented yet")
}
