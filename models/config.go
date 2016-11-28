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
	Aggregator struct {
		Host string
	}
	Buildbot struct {
		WebHook string
	}
	Cite struct {
		Host                string
		ListenPort          string
		RCRetentionDuration string
		Version             string
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
		WatchcenterAPI string
	}
}

func init() {
	viper.SetConfigFile("conf/cite.yaml")
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
