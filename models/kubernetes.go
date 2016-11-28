package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"regexp"
	"sync"
	"time"

	gologging "github.com/op/go-logging"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/api/resource"
	"k8s.io/kubernetes/pkg/client/restclient"
	k8sClient "k8s.io/kubernetes/pkg/client/unversioned"
	"k8s.io/kubernetes/pkg/labels"
	"k8s.io/kubernetes/pkg/util/intstr"
	"k8s.io/kubernetes/pkg/util/sets"
	"k8s.io/kubernetes/pkg/util/wait"
)

type Kubernetes struct {
	client       *k8sClient.Client
	util         *Util
	nameRegex    *regexp.Regexp
	pollInterval time.Duration
	pollTimeout  time.Duration
}

var (
	k8sOnce sync.Once
	k8sInst *Kubernetes
)

func NewKubernetes() *Kubernetes {
	k8sOnce.Do(func() {
		cfg := &restclient.Config{
			Host:     Conf.Kubernetes.Master,
			Insecure: true,
		}
		client, err := k8sClient.New(cfg)
		if err != nil {
			logger.Error("error on k8s master connection:", err)
		}

		pollInterval := Conf.Kubernetes.PollInterval
		pollTimeout := Conf.Kubernetes.PollTimeout
		k8sInst = &Kubernetes{
			client:       client,
			util:         NewUtil(),
			pollInterval: time.Duration(pollInterval) * time.Second,
			pollTimeout:  time.Duration(pollTimeout) * time.Second,
			nameRegex:    regexp.MustCompile("[^-0-9a-zA-Z]+"),
		}
	})
	return k8sInst
}

func (this *Kubernetes) GetLabels(githubRepo, gitBranch string) map[string]string {
	return map[string]string{
		"service": this.util.NormalizeByHyphen("", githubRepo),
		"branch":  this.util.NormalizeGitBranch("", gitBranch),
	}
}

func (this *Kubernetes) GetAllNamespaces() ([]api.Namespace, error) {
	nl, err := this.client.Namespaces().List(api.ListOptions{})
	return nl.Items, err
}

func (this *Kubernetes) GetNamespaces(labelMap map[string]string) ([]api.Namespace, error) {
	sel := labels.Set(labelMap).AsSelector()
	nl, err := this.client.Namespaces().List(api.ListOptions{LabelSelector: sel})
	if err != nil {
		return nil, err
	}
	return nl.Items, err
}

func (this *Kubernetes) GetNamespace(name string) (*api.Namespace, error) {
	return this.client.Namespaces().Get(name)
}

func (this *Kubernetes) UpsertNamespace(namespace string) error {
	nsi := this.client.Namespaces()
	ns, err := nsi.Get(namespace)
	if err != nil {
		logger.Info("k8s ns get failed. maybe not exist yet?:", namespace, err)

		nsSpec := &api.Namespace{
			ObjectMeta: api.ObjectMeta{
				Name: namespace,
			},
		}

		ns, err = nsi.Create(nsSpec)
		if err != nil {
			logger.Error("error on k8s Namespace create:", err)
			return err
		}
	}
	logger.Info("k8s ns:", ns)

	return nil
}

func (this *Kubernetes) DeleteNamespace(nsName string) error {
	// delete namespace
	err := this.client.Namespaces().Delete(nsName)
	if err != nil {
		return err
	}

	// delete namespace svcs
	svcRequirement, _ := labels.NewRequirement("type", labels.DoesNotExistOperator, sets.NewString())
	svcs, err := this.GetAllServices(nsName, *svcRequirement)
	if err != nil {
		return err
	}

	for _, svc := range svcs {
		err := this.DeleteService(nsName, svc.Name)
		if err != nil {
			return err
		}
	}
	return nil
}

func (this *Kubernetes) GetAllServices(namespace string, requirements ...labels.Requirement) ([]api.Service, error) {
	sel := labels.NewSelector()
	for _, r := range requirements {
		sel = sel.Add(r)
	}
	sl, err := this.client.Services(namespace).List(api.ListOptions{
		LabelSelector: sel,
	})
	return sl.Items, err
}

func (this *Kubernetes) GetServices(namespace string, labelMap map[string]string) ([]api.Service, error) {
	sel := labels.Set(labelMap).AsSelector()
	svcRequirement, _ := labels.NewRequirement("type", labels.DoesNotExistOperator, sets.NewString())
	sel = sel.Add(*svcRequirement)
	sl, err := this.client.Services(namespace).List(api.ListOptions{LabelSelector: sel})
	if err != nil {
		return nil, err
	}

	return sl.Items, nil
}

func (this *Kubernetes) GetService(nsName, svcName string) (*api.Service, *Metadata, error) {
	svc, err := this.client.Services(nsName).Get(svcName)
	if err != nil {
		return nil, nil, err
	}

	metaStr, ok := svc.Annotations[CITE_K8S_ANNOTATION_KEY]
	if !ok {
		errMsg := fmt.Sprintf("cite annotation not found. ns:%s, svc:%s",
			svc.Namespace, svc.Name)
		logger.Warning(errMsg)
		return nil, nil, fmt.Errorf(errMsg)
	}

	meta, err := UnmarshalMetadata(metaStr)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal annotation: %v", err)
	}

	return svc, meta, nil
}

func (this *Kubernetes) UpdateService(nsName string, svc *api.Service) (*api.Service, error) {
	return this.client.Services(nsName).Update(svc)
}

func (this *Kubernetes) UpsertService(nsName, svcName string,
	svcLabels, svcSelector map[string]string,
	annotations string, ports []int) (*api.Service, error) {
	logger.Debugf("service labels: %v, selector: %v, ports: %v", svcLabels, svcSelector, ports)

	svcLabels["loadbalancer"] = Conf.LoadBalancer.Driver

	var svc *api.Service
	svci := this.client.Services(nsName)

	// create service ports
	svcPorts := make([]api.ServicePort, len(ports))
	for i, port := range ports {
		svcPort := port
		if i == 0 {
			svcPort = 80
		}
		svcPorts[i] = api.ServicePort{
			Name:       fmt.Sprintf("port%d", svcPort),
			Protocol:   api.ProtocolTCP,
			Port:       int32(svcPort),
			TargetPort: intstr.FromInt(port),
		}
	}

	svcs, err := this.GetServices(nsName, svcLabels)

	if err != nil || len(svcs) < 1 {
		// create service if not exist

		svcAnnotations := make(map[string]string)
		if annotations != "" {
			svcAnnotations[CITE_K8S_ANNOTATION_KEY] = annotations
		}

		svcSpec := &api.Service{
			ObjectMeta: api.ObjectMeta{
				Name:        svcName,
				Labels:      svcLabels,
				Annotations: svcAnnotations,
			},
			Spec: api.ServiceSpec{
				Type:            api.ServiceTypeClusterIP,
				Ports:           svcPorts,
				Selector:        svcSelector,
				SessionAffinity: api.ServiceAffinityClientIP,
			},
		}
		svcSpecJSON, _ := json.MarshalIndent(svcSpec, "", "   ")
		logger.Debugf("service spec: %s", svcSpecJSON)

		svc, err = svci.Create(svcSpec)
		if err != nil {
			logger.Error("error on create k8s Service:", err)
			return svc, err
		}
	} else if len(svcs) > 1 {
		// return error if multiple services found
		svcNames := make([]string, len(svcs))
		for i, svc := range svcs {
			svcNames[i] = svc.Name
		}
		err = fmt.Errorf("multiple services found: %v", svcNames)
		logger.Error(err)
		return svc, err
	} else {
		// update service
		svc = &svcs[0]
		if annotations != "" {
			svc.Annotations[CITE_K8S_ANNOTATION_KEY] = annotations
		}
		svc.Spec.Ports = svcPorts
		svc.Spec.Selector = svcSelector
		svc, err = svci.Update(svc)
		if err != nil {
			logger.Error("error on update k8s Service:", err)
			return svc, err
		}
	}

	return svc, err
}

func (this *Kubernetes) GetEndpoints(nsName string, labelMap map[string]string) ([]api.Endpoints, error) {
	sel := labels.Set(labelMap).AsSelector()
	pl, err := this.client.Endpoints(nsName).List(api.ListOptions{LabelSelector: sel})
	if err != nil {
		return nil, err
	}

	return pl.Items, nil
}

func (this *Kubernetes) DeleteService(nsName, svcName string) error {
	svc, err := this.client.Services(nsName).Get(svcName)
	if err != nil {
		return err
	}

	// clone svc spec selector before delete svc
	svcSelector := map[string]string{}
	for k, v := range svc.Spec.Selector {
		svcSelector[k] = v
	}

	// delete svc
	err = this.client.Services(nsName).Delete(svcName)
	if err != nil {
		return err
	}

	// delete svc rcs
	rcs, err := this.GetReplicationControllers(nsName, svcSelector)
	if err != nil {
		return err
	}
	for _, rc := range rcs {
		err := this.DeleteReplicationController(nsName, rc.Name)
		if err != nil {
			return err
		}
	}

	return nil
}

func (this *Kubernetes) GetAllReplicationControllers(nsName string) ([]api.ReplicationController, error) {
	rcl, err := this.client.ReplicationControllers(nsName).List(api.ListOptions{})
	return rcl.Items, err
}

func (this *Kubernetes) GetReplicationControllers(nsName string, labelMap map[string]string, requirements ...labels.Requirement) ([]api.ReplicationController, error) {
	sel := labels.Set(labelMap).AsSelector()
	for _, r := range requirements {
		sel = sel.Add(r)
	}
	if !sel.Empty() {
		logger.Debugf("rc selector: %v", sel)
	}
	rcl, err := this.client.ReplicationControllers(nsName).List(api.ListOptions{LabelSelector: sel})
	if err != nil {
		return nil, err
	}

	return rcl.Items, nil
}

func (this *Kubernetes) GetReplicationController(nsName, rcName string) (*api.ReplicationController, error) {
	return this.client.ReplicationControllers(nsName).Get(rcName)
}

func (this *Kubernetes) UpdateReplicationController(nsName string, rc *api.ReplicationController) (*api.ReplicationController, error) {
	return this.client.ReplicationControllers(nsName).Update(rc)
}

func (this *Kubernetes) UpsertReplicationController(nsName, rcGenerateName, imageName string, rcLabels, rcSelector map[string]string, environment map[string]string, replicas int, ports []int, probePath string, deployID int, fluentLogger *gologging.Logger) error {
	logger.Info(fmt.Sprintf("upsert replication controller. ns:%s, rc:%s, env:%v", nsName, rcGenerateName, environment))

	var rc *api.ReplicationController
	rci := this.client.ReplicationControllers(nsName)

	// create container ports
	containerPorts := make([]api.ContainerPort, len(ports))
	for i, port := range ports {
		containerPorts[i] = api.ContainerPort{
			Name:          fmt.Sprintf("port%d", port),
			ContainerPort: int32(port),
		}
	}

	// define ReplicationController spec
	if probePath == "" {
		probePath = "/"
	}

	containerImage := imageName
	var containerEnvVars []api.EnvVar
	for k, v := range environment {
		containerEnvVars = append(containerEnvVars, api.EnvVar{
			Name:  k,
			Value: v,
		})
	}

	resourceRequests := make(api.ResourceList)
	resourceRequests[api.ResourceCPU] = resource.MustParse(Conf.Kubernetes.DefaultCPU)
	resourceRequests[api.ResourceMemory] = resource.MustParse(Conf.Kubernetes.DefaultMemory)

	resourceLimits := make(api.ResourceList)
	resourceLimits[api.ResourceCPU] = resource.MustParse(Conf.Kubernetes.MaxCPU)
	resourceLimits[api.ResourceMemory] = resource.MustParse(Conf.Kubernetes.MaxMemory)

	rcSpec := &api.ReplicationController{
		ObjectMeta: api.ObjectMeta{
			GenerateName: rcGenerateName,
			Labels:       rcLabels,
		},
		Spec: api.ReplicationControllerSpec{
			Replicas: int32(replicas),
			Selector: rcSelector,
			Template: &api.PodTemplateSpec{
				ObjectMeta: api.ObjectMeta{
					Name:   rcGenerateName,
					Labels: rcLabels,
				},
				Spec: api.PodSpec{
					Containers: []api.Container{
						api.Container{
							Name:  rcGenerateName,
							Env:   containerEnvVars,
							Image: containerImage,
							Ports: containerPorts,
							LivenessProbe: &api.Probe{
								Handler: api.Handler{
									TCPSocket: &api.TCPSocketAction{
										Port: intstr.FromInt(ports[0]),
									},
								},
								InitialDelaySeconds: int32(this.pollTimeout.Seconds()),
								TimeoutSeconds:      int32(this.pollTimeout.Seconds()),
							},
							Resources: api.ResourceRequirements{
								Requests: resourceRequests,
								Limits:   resourceLimits,
							},
						},
					},
				},
			},
		},
	}

	// create ReplicationController
	rc, err := rci.Create(rcSpec)
	if err != nil {
		logger.Error("error on k8s ReplicationController create:", err)
		return err
	}

	// wait for Pods
	logMsg := fmt.Sprintf("wait for pod ready status: trying to connect port %d", ports[0])
	if fluentLogger != nil {
		fluentLogger.Info(logMsg)
	} else {
		logger.Info(logMsg)
	}
	startTime := time.Now()
	initialDelay, _ := time.ParseDuration("0")

	logger.Info("start new pods...")
	if err := wait.Poll(this.pollInterval, this.pollTimeout, this.allPodsReady(rc, fluentLogger, &startTime, &initialDelay)); err != nil {
		logger.Error("error while waiting for pods:", err)
		deleteErr := rci.Delete(rc.Name)
		if deleteErr != nil {
			return fmt.Errorf("error while deleting RC:%v", deleteErr)
		}
		return err
	}
	initialDelaySeconds := int(initialDelay.Seconds())
	// tune initial delay
	if initialDelaySeconds < Conf.Kubernetes.MinInitialDelay {
		initialDelaySeconds = Conf.Kubernetes.MinInitialDelay
	} else if initialDelaySeconds > Conf.Kubernetes.MaxInitialDelay {
		initialDelaySeconds = Conf.Kubernetes.MaxInitialDelay
	}
	logger.Info("initial delay:", initialDelaySeconds)

	// wait for ReplicationController
	if err := wait.Poll(this.pollInterval, this.pollTimeout, k8sClient.ControllerHasDesiredReplicas(this.client, rc)); err != nil {
		logger.Error("error while waiting for ControllerHasDesiredReplicas:", err)
		deleteErr := rci.Delete(rc.Name)
		if deleteErr != nil {
			return fmt.Errorf("error while deleting RC:%v", deleteErr)
		}
		return err
	}

	// patch deployed ReplicationController
	rc, err = rci.Get(rc.Name)
	if err != nil {
		return fmt.Errorf("failed to get RC: %v", err)
	}

	containers := make([]api.Container, len(rcSpec.Spec.Template.Spec.Containers))
	for i, c := range rcSpec.Spec.Template.Spec.Containers {
		c.ReadinessProbe = &api.Probe{
			Handler: api.Handler{
				TCPSocket: &api.TCPSocketAction{
					Port: intstr.FromInt(ports[0]),
				},
			},
			InitialDelaySeconds: int32(initialDelaySeconds),
			TimeoutSeconds:      int32(this.pollTimeout.Seconds()),
		}
		containers[i] = c
	}

	rc.Spec.Template.Spec.Containers = containers

	rc, err = rci.Update(rc)
	if err != nil {
		return fmt.Errorf("failed to update RC: %v", err)
	}

	return nil
}

func (this *Kubernetes) DeleteReplicationController(nsName, rcName string) error {
	rc, err := this.client.ReplicationControllers(nsName).Get(rcName)
	if err != nil {
		return err
	}

	// copy rc spec selector before delete rc
	rcSelector := map[string]string{}
	for k, v := range rc.Spec.Selector {
		rcSelector[k] = v
	}

	// delete rc
	err = this.client.ReplicationControllers(nsName).Delete(rcName)
	if err != nil {
		return err
	}

	// delete rc pods
	pods, err := this.GetPods(nsName, rcSelector)
	if err != nil {
		return err
	}
	for _, pod := range pods {
		err := this.DeletePod(nsName, pod.Name)
		if err != nil {
			return err
		}
	}
	return nil
}

func (this *Kubernetes) ScaleReplicationController(nsName, rcName string, replicas int) (*api.ReplicationController, error) {
	rci := this.client.ReplicationControllers(nsName)
	rc, err := rci.Get(rcName)
	if err != nil {
		return nil, err
	}
	rc.Spec.Replicas = int32(replicas)
	return rci.Update(rc)
}

func (this *Kubernetes) GetAllPods(nsName string) ([]api.Pod, error) {
	pl, err := this.client.Pods(nsName).List(api.ListOptions{})
	return pl.Items, err
}

func (this *Kubernetes) GetPods(nsName string, labelMap map[string]string) ([]api.Pod, error) {
	sel := labels.Set(labelMap).AsSelector()
	pl, err := this.client.Pods(nsName).List(api.ListOptions{LabelSelector: sel})
	if err != nil {
		return nil, err
	}

	return pl.Items, nil
}

func (this *Kubernetes) GetPodLogs(nsName, podID string, createdAt time.Time) (string, error) {
	logger.Info(fmt.Sprintf("get pod logs. ns:%v, pod:%v, createdAt:%v", nsName, podID, createdAt.Format(time.RFC3339)))
	var (
		readCloser io.ReadCloser
		err        error
	)

	for {
		req := this.client.RESTClient.Get().
			Namespace(nsName).
			Name(podID).
			Resource("pods").
			SubResource("log")
		req.Param("previous", "true")
		req.Param("tailLines", "100")

		readCloser, err = req.Stream()
		if err == nil {
			break
		} else {
			logger.Info(fmt.Sprintf("error while getting logs:%v", err))
			time.Sleep(1 * time.Second)
		}
	}

	var out bytes.Buffer
	defer readCloser.Close()
	_, err = io.Copy(&out, readCloser)
	return out.String(), err
}

func (this *Kubernetes) DeletePod(nsName, podName string) error {
	return this.client.Pods(nsName).Delete(podName, &api.DeleteOptions{})
}

func (this *Kubernetes) WaitForRC(rc *api.ReplicationController) error {
	if err := wait.Poll(this.pollInterval, this.pollTimeout, this.allPodsReady(rc, nil, nil, nil)); err != nil {
		return err
	}
	return nil
}

func (this *Kubernetes) allPodsReady(controller *api.ReplicationController, fluentLogger *gologging.Logger, startTime *time.Time, initialDelay *time.Duration) wait.ConditionFunc {
	sel := labels.Set(controller.Spec.Selector).AsSelector()
	logger.Info("label selector:", sel)

	return func() (bool, error) {
		pods, err := this.client.Pods(controller.Namespace).List(api.ListOptions{LabelSelector: sel})
		if err != nil {
			return false, err
		}

		readyPods := int32(0)
		for _, pod := range pods.Items {
			numOfContainers := len(pod.Spec.Containers)
			readyContainers := 0
			for _, c := range pod.Status.ContainerStatuses {
				if c.State.Waiting != nil {
					if c.State.Waiting.Reason == "CrashLoopBackOff" {
						msg, err := this.GetPodLogs(pod.Namespace, pod.Name, pod.CreationTimestamp.Time)
						if err != nil {
							msg = fmt.Sprintf("failed to get message: %v", err)
						}
						return false, fmt.Errorf("failed to start container. message:%v", msg)
					}
					//return false, fmt.Errorf("waiting. reason:%v, message:%v", c.State.Waiting.Reason, c.State.Waiting.Message)
				} else if c.State.Running != nil {
					readyContainers++
				}
			}

			if numOfContainers == readyContainers {
				for _, container := range pod.Spec.Containers {
					if len(container.Ports) <= 0 {
						readyPods++
					}
					// for _, port := range container.Ports {
					port := container.Ports[0]
					if port.Protocol == api.ProtocolTCP {
						_, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", pod.Status.PodIP, port.ContainerPort), 10*time.Second)
						if err != nil {
							logMsg := fmt.Sprintf("failed to connect : %s:%d", pod.Status.PodIP, port.ContainerPort)
							logger.Info(logMsg)
						} else {
							readyPods++
						}
					} else if port.Protocol == api.ProtocolUDP {
						readyPods++
					}
				}
			}
		}

		if readyPods > 0 && startTime != nil && initialDelay.Seconds() <= 0 {
			*initialDelay = time.Now().Sub(*startTime)
			logger.Info("initial delay set!", initialDelay)
		}
		logMsg := fmt.Sprintf("pod ready: %d/%d = %.2f%%",
			readyPods,
			controller.Spec.Replicas,
			float32(readyPods)/float32(controller.Spec.Replicas)*100)
		if fluentLogger != nil {
			fluentLogger.Info(logMsg)
		} else {
			logger.Info(logMsg)
		}
		return readyPods == controller.Spec.Replicas, nil
	}
}
