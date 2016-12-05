package models

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"gopkg.in/olivere/elastic.v3"
)

type Elastic struct {
	client        *elastic.Client
	GMT           *time.Location
	AppLogTmpl    *template.Template
	DeployLogTmpl *template.Template
}

var (
	elasticOnce sync.Once
	elasticInst *Elastic
)

const (
	APP_LOG_TMPL    = `{{.KibanaURL}}#/discover?_g=(refreshInterval:(display:'5%20seconds',pause:!f,section:1,value:5000),time:(from:now-1h,mode:quick,to:now))&_a=(columns:!(kubernetes.labels.service,kubernetes.labels.branch,log),filters:!(('$state':(store:appState),meta:(alias:!n,disabled:!f,index:'{{.Index}}-*',key:kubernetes.labels.service,negate:!f,value:{{.Service}}),query:(match:(kubernetes.labels.service:(query:{{.Service}},type:phrase)))),('$state':(store:appState),meta:(alias:!n,disabled:!f,index:'{{.Index}}-*',key:kubernetes.labels.branch,negate:!f,value:{{.Branch}}),query:(match:(kubernetes.labels.branch:(query:{{.Branch}},type:phrase))))),index:'{{.Index}}-*',interval:auto,query:(query_string:(analyze_wildcard:!t,query:'*')),sort:!('@timestamp',desc),vis:(aggs:!((params:(field:kubernetes.labels.branch,orderBy:'2',size:20),schema:segment,type:terms),(id:'2',schema:metric,type:count)),type:histogram))&indexPattern={{.Index}}-*&type=histogram`
	DEPLOY_LOG_TMPL = `{{.KibanaURL}}#/discover?_g=(refreshInterval:(display:'5%20seconds',pause:!f,section:1,value:5000),time:({{.Time}}))&_a=(columns:!(service,msg),filters:!(('$state':(store:appState),meta:(alias:!n,disabled:!f,index:'cite-core.deploy-*',key:deploy_id,negate:!f,value:'{{.DeployID}}'),query:(match:(deploy_id:(query:{{.DeployID}},type:phrase))))),index:'cite-core.deploy-*',interval:auto,query:(query_string:(analyze_wildcard:!t,query:'*')),sort:!('@timestamp',desc),vis:(aggs:!((params:(field:namespace,orderBy:'2',size:20),schema:segment,type:terms),(id:'2',schema:metric,type:count)),type:histogram))&indexPattern=cite-core.deploy-*&type=histogram`
)

func NewElastic() *Elastic {
	elasticOnce.Do(func() {
		client, err := elastic.NewClient(
			elastic.SetURL(Conf.ElasticSearch.Host),
			elastic.SetMaxRetries(50),
		)
		if err != nil {
			logger.Error("error on elasticsearch connection:", err)
			panic(err)
		}

		GMT, err := time.LoadLocation("GMT")
		if err != nil {
			logger.Error("error on loadLocation:", err)
			panic(err)
		}

		elasticInst = &Elastic{
			client:        client,
			GMT:           GMT,
			AppLogTmpl:    template.Must(template.New("applog").Parse(APP_LOG_TMPL)),
			DeployLogTmpl: template.Must(template.New("deploylog").Parse(DEPLOY_LOG_TMPL)),
		}
	})
	return elasticInst
}

func (this *Elastic) UpsertKibanaIndexPattern(namespace string) error {
	// will use 'index pattern creation API on kibana 5.0'.
	// see https://github.com/elastic/kibana/issues/3709
	// until then, manually add index-pattern to .kibana
	indexPattern := fmt.Sprintf("%s-*", namespace)
	kibanaIndex := map[string]string{
		"title":         indexPattern,
		"timeFieldName": "@timestamp",
	}
	x, err := this.client.Index().
		Index(".kibana").
		Type("index-pattern").
		Id(indexPattern).
		BodyJson(kibanaIndex).
		Do()
	logger.Info(x)
	return err
}

func (this *Elastic) GetAppLogURL(nsName, svcName, branchName string) string {
	var urlBytes bytes.Buffer
	err := this.AppLogTmpl.Execute(&urlBytes, map[string]string{
		"KibanaURL": Conf.ElasticSearch.KibanaHost,
		"Index":     nsName,
		"Service":   svcName,
		"Branch":    strings.Replace(branchName, "/", ".", -1),
	})
	if err != nil {
		log.Printf("failed to generate applog url: %v", err)
		return Conf.ElasticSearch.KibanaHost
	}

	return urlBytes.String()
}

func (this *Elastic) GetDeployLogURL(deployID int, fromStr, toStr string) string {
	var timeStr string
	if strings.HasPrefix(fromStr, "now") || strings.HasPrefix(toStr, "now") {
		timeStr = fmt.Sprintf("from:%s,mode:quick,to:%s", fromStr, toStr)
	} else {
		fromTime, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			logger.Info("failed to parse from time %s: %v", fromStr, err)
			fromTime = time.Now().Add(-1 * time.Hour)
		}
		toTime, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			logger.Info("failed to parse to time %s: %v", toStr, err)
			toTime = time.Now()
		}
		timeStr = fmt.Sprintf("from:'%s',mode:absolute,to:'%s'",
			fromTime.In(this.GMT).Format(time.RFC3339),
			toTime.In(this.GMT).Format(time.RFC3339))
	}

	var urlBytes bytes.Buffer
	err := this.DeployLogTmpl.Execute(&urlBytes, map[string]string{
		"KibanaURL": Conf.ElasticSearch.KibanaHost,
		"Time":      timeStr,
		"DeployID":  strconv.Itoa(deployID),
	})
	if err != nil {
		logger.Error("error on elasticsearch connection:", err)
		panic(err)
	}

	return urlBytes.String()
}
