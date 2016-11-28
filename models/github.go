package models

import (
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

type GitHub struct {
	client           *github.Client
	util             *Util
	oauthConf        *oauth2.Config
	oauthStateString string
}

type ByOrg []github.Organization

func (s ByOrg) Len() int           { return len(s) }
func (s ByOrg) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s ByOrg) Less(i, j int) bool { return *s[i].Login < *s[j].Login }

type ByRepos []github.Repository

func (s ByRepos) Len() int           { return len(s) }
func (s ByRepos) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s ByRepos) Less(i, j int) bool { return *s[i].Name < *s[j].Name }

var (
	githubCommonOnce sync.Once
	githubCommonInst *GitHub
)

func NewCommonGitHub() *GitHub {
	githubCommonOnce.Do(func() {
		githubCommonInst = NewGitHub(Conf.GitHub.AccessToken)
	})
	return githubCommonInst
}

func NewGitHub(token string) *GitHub {
	clientURL, err := url.Parse(Conf.GitHub.API + "/")
	if err != nil {
		logger.Error("github api url is malformed:", err)
	}

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	tc := oauth2.NewClient(oauth2.NoContext, ts)

	client := github.NewClient(tc)
	client.BaseURL = clientURL

	oauthConf := &oauth2.Config{
		ClientID:     Conf.GitHub.ClientID,
		ClientSecret: Conf.GitHub.ClientSecret,
		Scopes:       strings.Split(Conf.GitHub.Scope, ","),
		Endpoint: oauth2.Endpoint{
			AuthURL:  Conf.GitHub.OAuthAuthURL,
			TokenURL: Conf.GitHub.OAuthTokenURL,
		},
	}

	githubInst := &GitHub{
		client:           client,
		util:             NewUtil(),
		oauthConf:        oauthConf,
		oauthStateString: "Veivi5ueohg0eiKide1of3MoAip1nais",
	}

	return githubInst
}

func (this *GitHub) GetLoginURL() string {
	return this.oauthConf.AuthCodeURL(this.oauthStateString, oauth2.AccessTypeOnline)
}

func (this *GitHub) GetOAuth2Token(state, code string) (string, error) {
	if this.oauthStateString != state {
		return "", fmt.Errorf(
			"github state does not match. sent:%v, recv:%v",
			this.oauthStateString, state)
	}

	token, err := this.oauthConf.Exchange(oauth2.NoContext, code)
	if err != nil {
		return "", fmt.Errorf("error while exchange github oauth token: %v", err)
	}

	return token.AccessToken, nil
}

func (this *GitHub) GetUser() (*github.User, error) {
	user, _, err := this.client.Users.Get("")
	return user, err
}

func (this *GitHub) ListOwnerRepos() ([]github.Repository, error) {
	var (
		repos []github.Repository
		err   error
	)
	for page := 1; ; page++ {
		r, _, err := this.client.Repositories.List("", &github.RepositoryListOptions{
			Type: "owner",
			ListOptions: github.ListOptions{
				Page:    page,
				PerPage: 100,
			},
		})
		repos = append(repos, r...)
		if len(r) < 100 || err != nil {
			break
		}
	}
	sort.Sort(ByRepos(repos))
	return repos, err
}

func (this *GitHub) ListOrgRepos(org string) ([]github.Repository, error) {
	var (
		repos []github.Repository
		err   error
	)
	for page := 1; ; page++ {
		r, _, err := this.client.Repositories.ListByOrg(org, &github.RepositoryListByOrgOptions{
			ListOptions: github.ListOptions{
				Page:    page,
				PerPage: 100,
			},
		})
		repos = append(repos, r...)
		if len(r) < 100 || err != nil {
			break
		}
	}
	sort.Sort(ByRepos(repos))
	return repos, err
}

func (this *GitHub) GetRepo(owner, repo string) (*github.Repository, error) {
	r, _, err := this.client.Repositories.Get(owner, repo)
	return r, err
}

func (this *GitHub) ListOrgs() ([]github.Organization, error) {
	var (
		orgs []github.Organization
		err  error
	)
	for page := 1; ; page++ {
		o, _, err := this.client.Organizations.List("", &github.ListOptions{
			Page:    page,
			PerPage: 100,
		})
		orgs = append(orgs, o...)
		if len(o) < 100 || err != nil {
			break
		}
	}
	sort.Sort(ByOrg(orgs))
	return orgs, err
}

func (this *GitHub) GetCommit(owner, repo, sha string) (*github.RepositoryCommit, error) {
	commit, _, err := this.client.Repositories.GetCommit(owner, repo, sha)
	return commit, err
}

func (this *GitHub) GetBranch(owner, repo, branchName string) (*github.Branch, error) {
	branch, _, err := this.client.Repositories.GetBranch(owner, repo, branchName)
	return branch, err
}

func (this *GitHub) GetDefaultBranchName(owner, repo string) (string, error) {
	r, _, err := this.client.Repositories.Get(owner, repo)
	if err != nil {
		return "", err
	}
	return *r.DefaultBranch, nil
}

func (this *GitHub) ListBranches(owner, repo string, opt *github.ListOptions) ([]github.Branch, error) {
	branch, _, err := this.client.Repositories.ListBranches(owner, repo, opt)
	return branch, err
}

func (this *GitHub) ListCommits(owner, repo string, opt *github.CommitsListOptions) ([]github.RepositoryCommit, error) {
	commits, _, err := this.client.Repositories.ListCommits(owner, repo, opt)
	return commits, err
}

func (this *GitHub) ListStatuses(owner, repo, ref string) ([]github.RepoStatus, error) {
	statuses, _, err := this.client.Repositories.ListStatuses(owner, repo, ref, &github.ListOptions{
		PerPage: 100,
	})
	return statuses, err
}

func (this *GitHub) ListDeployments(owner, repo string, opt *github.DeploymentsListOptions) ([]github.Deployment, error) {
	deployments, _, err := this.client.Repositories.ListDeployments(owner, repo, opt)
	return deployments, err
}

func (this *GitHub) ListDeploymentStatuses(owner, repo string, deployment int) ([]github.DeploymentStatus, error) {
	deploymentStatuses, _, err := this.client.Repositories.ListDeploymentStatuses(owner, repo, deployment, &github.ListOptions{
		PerPage: 100,
	})
	return deploymentStatuses, err
}

func (this *GitHub) GetSHA(owner, repo, branch string) (string, error) {
	br, _, err := this.client.Repositories.GetBranch(owner, repo, branch)
	if err != nil {
		return branch, err
	}
	return *br.Commit.SHA, nil
}

func (this *GitHub) CheckDockerfile(owner string, repo string) (bool, error) {
	branches, err := this.ListBranches(owner, repo, &github.ListOptions{
		PerPage: 100,
	})
	if err != nil {
		return false, err
	}
	for _, branch := range branches {
		_, _, _, err := this.client.Repositories.GetContents(
			owner, repo, "/Dockerfile", &github.RepositoryContentGetOptions{
				Ref: *branch.Name,
			})
		if err == nil {
			return true, nil
		}
	}
	return false, nil
}

func (this *GitHub) CreateCommitStatus(owner, repo, ref, statusStr string) {
	req := &github.RepoStatus{
		State: github.String(statusStr),
	}
	status, _, err := this.client.Repositories.CreateStatus(owner, repo, ref, req)
	if err != nil {
		logger.Warning(fmt.Sprintf("failed to update commit status on %v: %v", status.URL, err))
	}
}

func (this *GitHub) UpsertHook(owner, repo string) error {
	hooks := map[string]*github.Hook{
		"buildbot": &github.Hook{
			Name:   github.String("web"),
			Events: []string{"push"},
			Config: map[string]interface{}{
				"url":          github.String(Conf.Buildbot.WebHook),
				"content_type": github.String("json"),
			},
		},
		"cite": &github.Hook{
			Name:   github.String("web"),
			Events: []string{"status"},
			Config: map[string]interface{}{
				"url":          github.String(Conf.Cite.Host + Conf.Cite.ListenPort + Conf.GitHub.WebhookURI),
				"content_type": github.String("json"),
			},
		},
	}

	githubHooks, err := this.ListHooks(owner, repo)
	if err != nil {
		logger.Error(fmt.Sprintf("failed to list hooks on %s/%s: %v", owner, repo, err))
		return err
	}

	for _, githubHook := range githubHooks {
		for _, hook := range hooks {
			hookURL := hook.Config["url"].(*string)
			if url, ok := githubHook.Config["url"]; ok && url == *hookURL {
				hook.ID = githubHook.ID
				continue
			}
		}
	}

	for hookName, hook := range hooks {
		var err error
		if hook.ID != nil && *hook.ID > -1 {
			log.Printf("edit %s hook %d", hookName, *hook.ID)
			_, _, err = this.client.Repositories.EditHook(owner, repo, *hook.ID, hook)
		} else {
			log.Printf("create %s hook", hookName)
			_, _, err = this.client.Repositories.CreateHook(owner, repo, hook)
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func (this *GitHub) ListHooks(owner, repo string) ([]github.Hook, error) {
	hooks, _, err := this.client.Repositories.ListHooks(owner, repo, &github.ListOptions{
		PerPage: 100,
	})
	return hooks, err
}

func (this *GitHub) CreateDeployment(owner, repo, ref, description string) (int, error) {
	logger.Info(fmt.Sprintf("create deployment. owner:%s, repo:%s, ref:%s", owner, repo, ref))
	req := &github.DeploymentRequest{
		AutoMerge:        github.Bool(false),
		Ref:              github.String(ref),
		Description:      github.String(description),
		RequiredContexts: &[]string{},
	}
	deployment, _, err := this.client.Repositories.CreateDeployment(owner, repo, req)
	if err != nil {
		return -1, err
	}
	return *deployment.ID, nil
}

func (this *GitHub) CreateDeploymentStatus(owner, repo string, id int, state string) {
	req := &github.DeploymentStatusRequest{
		State: github.String(state),
	}
	deployStatus, _, err := this.client.Repositories.CreateDeploymentStatus(owner, repo, id, req)
	if err != nil {
		logger.Warning(fmt.Sprintf("failed to create deployment on %v: %v", deployStatus.ID, err))
	}
}

func (this *GitHub) AddCollaborator(owner, repo, collaborator string) error {
	isCollaborator, _, err := this.client.Repositories.IsCollaborator(owner, repo, collaborator)
	if err != nil {
		return fmt.Errorf("failed to check if user %s is collaborator on repository %s/%s: %v",
			collaborator, owner, repo, err)
	}
	if isCollaborator {
		logger.Info(fmt.Sprintf("user %s is already a collaborator.", collaborator))
		return nil
	}

	_, err = this.client.Repositories.AddCollaborator(owner, repo, collaborator, nil)
	return err
}
