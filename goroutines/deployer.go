package goroutines

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/kakao/cite/models"
)

type Deployer struct {
	docker *models.Docker
	github *models.GitHub
	k8s    *models.Kubernetes
	noti   *models.Notifier
	util   *models.Util
	wc     *models.WatchCenter
}

var (
	deployerOnce sync.Once
	deployerInst *Deployer
)

func NewDeployer() *Deployer {
	deployerOnce.Do(func() {
		deployerInst = &Deployer{
			docker: models.NewDocker(),
			github: models.NewCommonGitHub(),
			k8s:    models.NewKubernetes(),
			noti:   models.NewNotifier(),
			util:   models.NewUtil(),
			wc:     models.NewWatchCenter(),
		}
	})
	return deployerInst
}

func (this *Deployer) Deploy(meta *models.Metadata, sha string, imageName string, deployID int) {
	var (
		msg string
		err error
	)

	if deployID <= 0 {
		deployID, err = this.github.CreateDeployment(meta.GithubOrg, meta.GithubRepo, meta.GitBranch, "cite CI")
		if err != nil {
			errMsg := fmt.Sprintf(
				"error while create deployments to github:%s/%s/%s: %v",
				meta.GithubOrg, meta.GithubRepo, meta.GitBranch, err)
			logger.Error(errMsg)
			this.noti.SendWithFallback(meta.Notification, meta.Watchcenter, msg)
			return
		}
	}

	this.github.CreateDeploymentStatus(meta.GithubOrg, meta.GithubRepo, deployID, "pending")
	deploymentState := "failure"
	defer func() {
		this.github.CreateDeploymentStatus(meta.GithubOrg, meta.GithubRepo, deployID, deploymentState)
	}()

	nsName := this.util.NormalizeByHyphen("", meta.GithubOrg)
	fluentLogger := models.NewFluentLogger("cite-core.deploy", map[string]interface{}{
		"namespace": nsName,
		"service":   meta.Service,
		"sha":       sha,
		"imageName": imageName,
		"deploy_id": deployID,
	})

	if len(imageName) == 0 {
		msg = fmt.Sprintf(`invalid docker image name: "%s"`, imageName)
		this.noti.SendWithFallback(meta.Notification, meta.Watchcenter, msg)
		fluentLogger.Info(msg)
		return
	}
	logger.Debug("imageName:", imageName)

	msg = fmt.Sprintf("deploy started: %s/%s/%s:%s",
		meta.GithubOrg,
		meta.GithubRepo,
		meta.GitBranch,
		sha)
	this.noti.SendWithFallback(meta.Notification, meta.Watchcenter, msg)
	fluentLogger.Info(msg)

	baseLabels := this.k8s.GetLabels(meta.GithubRepo, meta.GitBranch)

	rcGenerateName := this.util.Normalize("-", meta.GithubRepo, meta.GitBranch, sha)
	if len(rcGenerateName) >= 58 {
		rcGenerateName = rcGenerateName[0:58]
	}

	rcLabels := make(map[string]string)
	for k, v := range baseLabels {
		rcLabels[k] = v
	}
	rcLabels["sha"] = sha
	rcLabels["deploy_id"] = strconv.Itoa(deployID)

	rcSelector := make(map[string]string)
	for k, v := range baseLabels {
		rcSelector[k] = v
	}
	rcSelector["sha"] = sha
	rcSelector["deploy_id"] = strconv.Itoa(deployID)

	// upsert k8s replication controller
	if err := this.k8s.UpsertReplicationController(
		nsName,
		rcGenerateName,
		imageName,
		rcLabels,
		rcSelector,
		meta.EnvironmentMap(),
		meta.Replicas,
		meta.ContainerPorts,
		meta.ProbePath,
		deployID,
		fluentLogger,
	); err != nil {
		logger.Error("error on upsert k8s ReplicationController :", err)
		msg = fmt.Sprintf("deploy failed: %v", err)
		this.noti.SendWithFallback(meta.Notification, meta.Watchcenter, msg)
		fluentLogger.Info(msg)
		return
	}

	svcLabels := make(map[string]string)
	for k, v := range baseLabels {
		svcLabels[k] = v
	}

	svcSelector := make(map[string]string)
	for k, v := range baseLabels {
		svcSelector[k] = v
	}
	svcSelector["sha"] = sha
	svcSelector["deploy_id"] = strconv.Itoa(deployID)

	// upsert k8s service
	_, err = this.k8s.UpsertService(
		nsName,
		meta.Service,
		svcLabels,
		svcSelector,
		meta.Marshal(),
		meta.ContainerPorts,
	)
	if err != nil {
		logger.Error("error on upsert k8s Service :", err)
		msg = fmt.Sprintf("deploy failed: %v", err)
		this.noti.SendWithFallback(meta.Notification, meta.Watchcenter, msg)
		fluentLogger.Info(msg)
		return
	}

	// lbMeta := make(map[string]interface{})
	// if lbMetaStr, ok := svc.Annotations["loadbalancer"]; ok {
	// 	if err := json.Unmarshal([]byte(lbMetaStr), &lbMeta); err != nil {
	// 		logger.Infof("failed to unmarshal service annotation 'loadbalancer' on %s/%s: %v", svc.Namespace, svc.Name, err)
	// 	}
	// }

	// msg = fmt.Sprintf(`deploy success: https://%s`, lbMeta["domain"])
	msg = fmt.Sprintf(`deploy success`)
	logger.Debug(msg)
	this.noti.SendWithFallback(meta.Notification, meta.Watchcenter, msg)
	fluentLogger.Info(msg)

	deploymentState = "success"
}
