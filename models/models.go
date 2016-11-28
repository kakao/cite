package models

import (
	gologging "github.com/op/go-logging"
)

var (
	err    error
	logger = gologging.MustGetLogger("stdout")
)

const (
	CITE_BUILDBOT_GITHUB_CONTEXT = "buildbot/cite-build"
	CITE_K8S_ANNOTATION_KEY = "cite.io/created-by"
)
