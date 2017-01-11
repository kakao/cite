package controller

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/google/go-github/github"
	"github.com/kakao/cite/models"
	"github.com/labstack/echo"
	"github.com/yosssi/ace"
	k8sApi "k8s.io/kubernetes/pkg/api"
	k8sApiUnversioned "k8s.io/kubernetes/pkg/api/unversioned"
)

type AceRenderer struct{}

var aceFuncMap = template.FuncMap{
	"deref":                    deref,
	"getDomain":                getDomain,
	"getEndpoints":             getEndpoints,
	"getImageName":             getImageName,
	"getVIP":                   getVIP,
	"getPods":                  getPods,
	"githubDeploymentStatuses": githubDeploymentStatuses,
	"githubStatuses":           githubStatuses,
	"groupByRepoName":          groupByRepoName,
	"incrRC":                   incrRC,
	"decrRC":                   decrRC,
	"kibanaAppLogURL":          kibanaAppLogURL,
	"listWatchcenterGroups":    listWatchcenterGroups,
	"normalizeByHyphen":        normalizeByHyphen,
	"printTime":                printTime,
}

func (a AceRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	tpl, err := ace.Load("_layout", name, &ace.Options{
		DynamicReload: models.Conf.Cite.Version == "DEV",
		BaseDir:       "views",
		FuncMap:       aceFuncMap,
	})
	if err != nil {
		return err
	}

	switch dataMap := data.(type) {
	case map[string]interface{}:
		dataMap["conf"] = models.Conf
		if session := getSession(c); session != nil {
			if userName, ok := session.Values["userName"]; ok {
				dataMap["userName"] = userName
			}
			if userLogin, ok := session.Values["userLogin"]; ok {
				dataMap["userLogin"] = userLogin
			}
			if userEmail, ok := session.Values["userEmail"]; ok {
				dataMap["userEmail"] = userEmail
			}
			if flashes := session.Flashes(); len(flashes) > 0 {
				dataMap["flashes"] = flashes
				saveSession(session, c)
			}
		}
		return tpl.Execute(w, dataMap)
	default:
		return tpl.Execute(w, data)
	}
}

func deref(in *string) string {
	return *in
}

func getEndpoints(nsName string, epSelector map[string]string) []k8sApi.Endpoints {
	eps, err := k8s.GetEndpoints(nsName, epSelector)
	if err != nil {
		logger.Error(err)
		return []k8sApi.Endpoints{}
	}
	return eps
}

func getDomain(svc k8sApi.Service) string {
	var lbMeta ServiceAnnotation
	if lbMetaStr, ok := svc.Annotations["loadbalancer"]; ok {
		if err := json.Unmarshal([]byte(lbMetaStr), &lbMeta); err != nil {
			logger.Infof("failed to unmarshal service annotation 'loadbalancer' on %s/%s: %v", svc.Namespace, svc.Name, err)
			return ""
		}
	}
	return lbMeta.Domain
}

func getVIP(domain string) []string {
	addrs, err := net.LookupHost(domain)
	if err != nil {
		logger.Errorf("failed to lookup domain %s: %v", domain, err)
	}
	return addrs
}

func getImageName(description string) string {
	imageName, err := buildbotClient.GetImageName(description)
	if err != nil {
		logger.Error(err)
		return ""
	}
	return imageName
}

func getPods(nsName string, podSelector map[string]string) []k8sApi.Pod {
	pods, err := k8s.GetPods(nsName, podSelector)
	if err != nil {
		logger.Error(err)
		return []k8sApi.Pod{}
	}
	return pods
}

func githubStatuses(owner, repo, ref string) []github.RepoStatus {
	allStatuses, err := commonGitHub.ListStatuses(owner, repo, ref)
	if err != nil {
		errMsg := fmt.Sprintf("error while getting statuses from github %s/%s/%s, err:%v", owner, repo, ref, err)
		logger.Error(errMsg)
	}
	// filter out non-cite build statuses
	var statuses []github.RepoStatus
	for _, s := range allStatuses {
		if *s.Context == models.CITE_BUILDBOT_GITHUB_CONTEXT {
			statuses = append(statuses, s)
		}
	}
	return statuses
}

func githubDeploymentStatuses(owner, repo string, deployment int) []github.DeploymentStatus {
	ds, err := commonGitHub.ListDeploymentStatuses(owner, repo, deployment)
	if err != nil {
		errMsg := fmt.Sprintf("error while getting deployment statuses from github %s/%s, id:%v, err:%v", owner, repo, deployment, err)
		logger.Error(errMsg)
	}
	return ds
}

func groupByRepoName(in []k8sApi.Service) map[string][]k8sApi.Service {
	out := make(map[string][]k8sApi.Service)
	for _, svc := range in {
		githubRepo := svc.Labels["service"]
		out[githubRepo] = append(out[githubRepo], svc)
	}
	return out
}

func incrRC(in int32) int32 {
	in = in + 1
	if in > int32(models.Conf.Kubernetes.MaxPods) {
		return int32(models.Conf.Kubernetes.MaxPods)
	}
	return in
}

func decrRC(in int32) int32 {
	in = in - 1
	if in < 0 {
		return 0
	}
	return in
}

func kibanaAppLogURL(nsName, svcName, branchName string) string {
	return es.GetAppLogURL(nsName, svcName, branchName)
}

func listWatchcenterGroups(username, email string) []models.WatchCenterGroup {
	gs, err := watchcenter.ListGroups(username, email)
	if err != nil {
		logger.Info(err)
		return []models.WatchCenterGroup{}
	}
	return gs
}

func normalizeByHyphen(in string) string {
	return util.NormalizeByHyphen("", in)
}

func printTime(in interface{}) string {
	switch t := in.(type) {
	case k8sApiUnversioned.Time:
		return fmt.Sprintf("%s (%s)", t.Local().Format(time.RFC1123), humanize.Time(t.Time))
	case *k8sApiUnversioned.Time:
		if t == nil {
			return ""
		}
		return fmt.Sprintf("%s (%s)", t.Local().Format(time.RFC1123), humanize.Time(t.Time))
	case *time.Time:
		if t == nil {
			return ""
		}
		return fmt.Sprintf("%s (%s)", t.Local().Format(time.RFC1123), humanize.Time(*t))
	case *github.Timestamp:
		if t == nil {
			return ""
		}
		return fmt.Sprintf("%s (%s)", t.Local().Format(time.RFC1123), humanize.Time(t.Time))
	default:
		return fmt.Sprintf("unknown time type %T", in)
	}
}
