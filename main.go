package main

import (
	"net/http"

	"github.com/kakao/cite/controller"
	"github.com/kakao/cite/models"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
)

func main() {
	// echo instance
	e := echo.New()

	// error handler
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if _, ok := err.(*echo.HTTPError); !ok {
			err = echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		e.DefaultHTTPErrorHandler(err, c)
	}

	// middleware
	e.Pre(middleware.RemoveTrailingSlash())
	e.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
		Format: "${time_rfc3339} ${remote_ip} ${method} ${status} ${latency_human} ${path}\n",
	}))
	e.Use(middleware.Recover())
	e.Use(middleware.Gzip())
	e.Use()
	// static resources
	e.Static("/static", "static")
	e.File("/favicon.ico", "static/favicon.ico")

	// set ace renderer
	e.Renderer = controller.AceRenderer{}

	// routes
	api := e.Group("/v1")
	{
		api.POST("/github", controller.PostGithubCallback)
		api.GET("/cite/service", controller.GetCiteService)
		api.GET("/cite/gc", controller.GetGarbageCollection)
		api.GET("/notification/watchcenter", controller.GetWatchcenterGroupID)
		api.GET("/notification/slack", controller.GetSlackOAuthToken)
	}

	ajax := e.Group("ajax")
	{
		ajax.Use(controller.AuthAPI)
		ajax.GET("/github/repos", controller.GetGithubRepos)
		ajax.GET("/github/branches", controller.GetGithubBranches)
		ajax.GET("/docker/tags", controller.GetDockerTags)
	}

	webPublic := e.Group("")
	{
		webPublic.GET("/login", controller.GetLogin)
		webPublic.GET("/logout", controller.GetLogout)
		webPublic.GET("/github-callback", controller.GetGithubCallback)
	}

	web := e.Group("")
	{
		web.Use(controller.AuthWeb)
		web.GET("/", controller.GetIndex)
		web.GET("/settings/profile", controller.GetProfileSettings)
		web.GET("/deploy_log/:github_org/:github_repo/:deploy_id", controller.GetDeployLog)
		web.GET("/new", controller.GetNewService)
		web.POST("/new", controller.PostNewService)
		web.GET("/delete/:type/:nsName/:name", controller.DeleteService)          // TODO: change method to DELETE
		web.GET("/scale/:nsName/:svcName/:rcName/:replicas", controller.PutScale) // TODO: change method to PUT
		web.GET("/namespaces", controller.GetNamespaces)
		web.GET("/namespaces/:namespace", controller.GetNamespace)
		web.GET("/namespaces/:namespace/services/:service", controller.GetService)
		web.GET("/namespaces/:namespace/services/:service/settings", controller.GetServiceSettings)
		web.POST("/namespaces/:namespace/services/:service/settings", controller.PostServiceSettings)
		web.GET("/namespaces/:namespace/services/:service/build/:sha", controller.PostBuild)                 // TODO: change method to POST
		web.GET("/namespaces/:namespace/services/:service/deploy/:sha", controller.PostDeploy)               // TODO: change method to POST
		web.GET("/namespaces/:namespace/services/:service/activate/:sha/:deploy_id", controller.PutActivate) // TODO: change method to PUT

		// github
		web.GET("/namespaces/:namespace/services/:service/commits", controller.GetGitHubCommits)
		web.GET("/namespaces/:namespace/services/:service/deployments", controller.GetGitHubDeployments)
	}

	test := e.Group("/test")
	{
		test.GET("/github", controller.GetGithub)
		test.GET("/github/commit", controller.GetGithubCommit)
		test.GET("/github/hook", controller.GetGithubHook)
		test.POST("/github/hook", controller.PostGithubHook)
		test.GET("/github/hook_patch", controller.GetGithubHookPatch)
		test.POST("/github/hook_proxy", controller.PostGithubHookProxy)
		test.POST("/github/collaborator", controller.PostGithubCollaborator)
		test.GET("/config", controller.GetConfig)
		test.GET(`/route/a`, controller.GetRouteA)
		test.GET(`/route/a/b`, controller.GetRouteAB)
		test.GET(`/route/a/:b/c/d`, controller.GetRouteABC)
		test.GET(`/route/b/*`, controller.GetRoute)
		test.GET("/logging", controller.GetLogging)
		test.GET("/buildbot", controller.GetBuildbot)
		test.POST("/buildbot", controller.PostBuildbot)
		test.GET("/ace", controller.GetAce)
		test.GET("/meta", controller.GetMetadata)
		test.GET("/docker", controller.GetDocker)
		test.GET("/env", controller.GetEnvironment)
		test.POST("/kibana", controller.PostKibana)
		test.GET("/set", controller.GetSet)
		test.GET("/panic", controller.GetPanic)
		test.GET("/panic_goroutine", controller.GetGoroutinePanic)
		test.GET("/error", controller.GetError)
		test.GET("/annotation", controller.GetAnnotation)
		test.GET("/normalize", controller.GetNormalize)
		test.GET("/k8s/svc", controller.GetKubernetesService)
		test.GET("/session", controller.GetSession)
		test.GET("/session_set", controller.PostSession)
		test.GET("/session_unset", controller.DeleteSession)
		test.GET("/submit", controller.GetFormSubmit)
		test.POST("/submit", controller.PostFormSubmit)
		test.GET("/noti", controller.GetNotifier)
		test.GET("/noti_system", controller.GetSystemNotifier)
	}

	// start server
	e.Logger.Fatal(e.Start(models.Conf.Cite.ListenPort))
}
