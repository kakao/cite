package models

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type Metadata struct {
	Namespace      string         `json:"namespace" form:"namespace" form:"namespace"`
	Service        string         `json:"service" form:"service" schema:"service"`
	GithubOrg      string         `json:"github_org" form:"github_org" schema:"github_org"`
	GithubRepo     string         `json:"github_repo" form:"github_repo" schema:"github_repo"`
	GitBranch      string         `json:"git_branch" form:"git_branch" schema:"git_branch"`
	AutoDeploy     bool           `json:"auto_deploy" form:"auto_deploy" schema:"auto_deploy"`
	ContainerPort  int            `json:"container_port" form:"container_port" schema:"container_port"`
	ContainerPorts []int          `json:"container_ports"`
	HTTPPort       string         `json:"http_port" form:"http_port" schema:"http_port"`
	TCPPort        string         `json:"tcp_port" form:"tcp_port" schema:"tcp_port"`
	ProbePath      string         `json:"probe_path" form:"probe_path" schema:"probe_path"`
	Replicas       int            `json:"replicas" form:"replicas" schema:"replicas"`
	Watchcenter    int            `json:"watchcenter" form:"watchcenter" schema:"watchcenter"`
	Environment    string         `json:"environment" form:"environment" schema:"environment"`
	Notification   []Notification `json:"notification" schema:"noti"`
	environmentMap map[string]string
}

type Notification struct {
	Driver      string `json:"driver" schema:"driver"`
	Enable      bool   `json:"enable" schema:"enable"`
	Endpoint    string `json:"endpoint" schema:"endpoint"`
	Description string `json:"description" schema:"description"`
}

func (this *Metadata) EnvironmentMap() map[string]string {
	if this.environmentMap != nil {
		return this.environmentMap
	}
	env := make(map[string]string)
	lines := strings.Split(this.Environment, "\n")
	for _, line := range lines {
		entries := strings.SplitN(line, "=", 2)
		if len(entries) != 2 || strings.HasPrefix(line, "#") {
			continue
		}
		env[entries[0]] = strings.TrimSpace(entries[1])
	}
	this.environmentMap = env
	return env
}

// not using reflection due to performance problem.
func (this *Metadata) Iter() map[string]string {
	m := make(map[string]string)
	m["namespace"] = fmt.Sprintf(`"%s"`, this.Namespace)
	m["service"] = fmt.Sprintf(`"%s"`, this.Service)
	m["github_org"] = fmt.Sprintf(`"%s"`, this.GithubOrg)
	m["github_repo"] = fmt.Sprintf(`"%s"`, this.GithubRepo)
	m["git_branch"] = fmt.Sprintf(`"%s"`, this.GitBranch)
	m["auto_deploy"] = strconv.FormatBool(this.AutoDeploy)
	m["container_port"] = strconv.Itoa(this.ContainerPort)
	m["http_port"] = fmt.Sprintf(`"%s"`, this.HTTPPort)
	m["tcp_port"] = fmt.Sprintf(`"%s"`, this.TCPPort)
	m["probe_path"] = fmt.Sprintf(`"%s"`, this.ProbePath)
	m["replicas"] = strconv.Itoa(this.Replicas)
	m["watchcenter"] = strconv.Itoa(this.Watchcenter)
	b, err := json.Marshal(this.EnvironmentMap())
	if err != nil {
		logger.Warning("error while marshaling environment:", err)
	}
	m["environment"] = string(b)

	return m
}

func (this *Metadata) Marshal() string {
	util := NewUtil()
	httpPorts, _ := util.TCPPortsToList(this.HTTPPort)
	tcpPorts, _ := util.TCPPortsToList(this.TCPPort)
	ports := append(httpPorts, tcpPorts...)
	this.ContainerPorts = ports
	this.ContainerPort = ports[0]

	b, err := json.Marshal(this)
	if err != nil {
		logger.Warning("error while marshaling environment:", err)
	}
	return string(b)
}

func UnmarshalMetadata(s string) (*Metadata, error) {
	meta := &Metadata{}
	err := json.Unmarshal([]byte(s), meta)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %v", err)
	}
	if meta.ContainerPort <= 0 || len(meta.ContainerPorts) <= 0 {
		util := NewUtil()
		httpPorts, _ := util.TCPPortsToList(meta.HTTPPort)
		tcpPorts, _ := util.TCPPortsToList(meta.TCPPort)
		ports := append(httpPorts, tcpPorts...)
		meta.ContainerPorts = ports
		meta.ContainerPort = ports[0]
	}
	logger.Debugf("meta: %v", meta)
	return meta, nil
}
