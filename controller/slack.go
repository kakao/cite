package controller

import (
	"fmt"

	"github.com/kakao/cite/models"
	"github.com/labstack/echo"
	"github.com/nlopes/slack"
)

func GetSlackCallback(c echo.Context) error {
	logger.Infof("code:%v", c.QueryParam("code"))

	redirectURI := models.Conf.Cite.Host + models.Conf.Cite.ListenPort + models.Conf.Notification.Slack.RedirectURI
	resp, err := slack.GetOAuthResponse(
		models.Conf.Notification.Slack.ClientID,
		models.Conf.Notification.Slack.ClientSecret,
		c.QueryParam("code"),
		redirectURI,
		false,
	)
	logger.Infof("resp:%v", resp)
	logger.Infof("err:%v", err)
	logger.Infof("token:%v", resp.AccessToken)
	logger.Infof("bot token:%v", resp.Bot.BotAccessToken)
	logger.Infof("channel:%v", resp.IncomingWebhook.Channel)
	logger.Infof("channel_id:%v", resp.IncomingWebhook.ChannelID)
	logger.Infof("channel_id:%v", resp.IncomingWebhook.ConfigurationURL)
	logger.Infof("channel_id:%v", resp.IncomingWebhook.URL)

	return fmt.Errorf("not implemented yet.")
}
