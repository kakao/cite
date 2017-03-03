package models

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/spf13/viper"
)

var (
	Conf Config
)

type Config struct {
	Cite struct {
		Host                string
		ListenPort          string
		RCRetentionDuration string
		Version             string
	}
	Aggregator struct {
		Host string
	}
	Buildbot struct {
		Host    string
		WebHook string
	}
	ElasticSearch struct {
		Host       string
		KibanaHost string
	}
	LoadBalancer struct {
		Driver string
	}
	GitHub struct {
		AccessToken   string
		API           string
		ClientID      string
		ClientSecret  string
		Host          string
		OAuthAuthURL  string
		OAuthTokenURL string
		Scope         string
		Username      string
		WebhookURI    string
	}
	Grafana struct {
		Host string
	}
	Kubernetes struct {
		Master          string
		MaxPods         int
		MinInitialDelay int
		MaxInitialDelay int
		PollInterval    int
		PollTimeout     int
		DefaultCPU      string
		DefaultMemory   string
		MaxCPU          string
		MaxMemory       string
	}
	Notification struct {
		Watchcenter struct {
			API string
		}
		Slack struct {
			ClientID     string
			ClientSecret string
			RedirectURI  string
		}
		Default struct {
			Slack string
		}
	}
}

func init() {
	for _, path := range []string{
		"conf/cite.yaml",
		"/etc/conf/cite.yaml",
	} {
		if _, err := os.Stat(path); err == nil {
			viper.SetConfigFile(path)
			break
		}
	}

	err = viper.ReadInConfig()
	if err != nil {
		logger.Panic(err)
	}

	if phase, ok := os.LookupEnv("PHASE"); ok {
		viper.SetConfigFile(
			fmt.Sprintf("conf/%s.yaml", phase))
		err = viper.MergeInConfig()
		if err != nil {
			logger.Panic(err)
		}
	}

	err = viper.Unmarshal(&Conf)
	if err != nil {
		logger.Panic(err)
	}

	// try to parse duration
	_, err := time.ParseDuration(Conf.Cite.RCRetentionDuration)
	if err != nil {
		log.Panicf("failed to parse duration %v: %v", Conf.Cite.RCRetentionDuration, err)
	}
}
