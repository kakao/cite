package controller

import (
	"fmt"

	"github.com/kakao/cite/models"
	"github.com/labstack/echo"
	"github.com/nlopes/slack"
)

func GetSlackCallback(c echo.Context) error {
	logger.Infof("client id:%v", models.Conf.Notification.Slack.ClientID)
	logger.Infof("client secret:%v", models.Conf.Notification.Slack.ClientSecret)
	logger.Infof("code:%v", c.QueryParam("code"))
	logger.Infof("redirect uri:%v", models.Conf.Cite.Host+models.Conf.Cite.ListenPort+models.Conf.Notification.Slack.RedirectURI)
	resp, err := slack.GetOAuthResponse(
		models.Conf.Notification.Slack.ClientID,
		models.Conf.Notification.Slack.ClientSecret,
		c.QueryParam("code"),
		models.Conf.Cite.Host+models.Conf.Cite.ListenPort+models.Conf.Notification.Slack.RedirectURI,
		false,
	)
	logger.Infof("resp:%v", resp)
	logger.Infof("err:%v", err)
	logger.Infof("token:%v", resp.AccessToken)
	logger.Infof("channel:%v", resp.IncomingWebhook.Channel)
	logger.Infof("channel_id:%v", resp.IncomingWebhook.ChannelID)

	return fmt.Errorf("not implemented yet.")
}
