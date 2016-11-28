package controller

import (
	githubClient "github.com/google/go-github/github"
)

type SortGithubDeploymentStatusesByCreatedAt []githubClient.DeploymentStatus

func (c SortGithubDeploymentStatusesByCreatedAt) Len() int { return len(c) }

func (c SortGithubDeploymentStatusesByCreatedAt) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

func (c SortGithubDeploymentStatusesByCreatedAt) Less(i, j int) bool {
	return c[i].CreatedAt.Before(c[j].CreatedAt.Time)
}

type ServiceAnnotation struct {
	Domain string `json:"domain,omitempty"`
}
