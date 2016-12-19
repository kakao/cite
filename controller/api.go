package controller

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kakao/cite/models"
	"github.com/labstack/echo"
	k8sApi "k8s.io/kubernetes/pkg/api"
	k8sLabels "k8s.io/kubernetes/pkg/labels"
)

func GetCiteService(c echo.Context) error {
	owner := c.QueryParam("owner")
	repo := c.QueryParam("repo")
	branch := c.QueryParam("branch")
	logger.Infof("service query owner:%s, repo:%s, branch:%s", owner, repo, branch)

	nsName := util.NormalizeByHyphen("", owner)
	svcLabels := k8s.GetLabels(repo, branch)
	svcs, err := k8s.GetServices(nsName, svcLabels)

	if err != nil || len(svcs) < 1 {
		errMsg := fmt.Sprintf("service not found. owner:%s, repo:%s, branch:%s",
			owner, repo, branch)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusNotFound, errMsg)
	}
	if len(svcs) > 1 {
		errMsg := fmt.Sprintf("multiple services found. owner:%s, repo:%s, branch:%s, services:%v",
			owner, repo, branch, svcs)
		logger.Error(errMsg)
		return echo.NewHTTPError(http.StatusNotFound, errMsg)
	}
	return c.JSON(http.StatusOK, svcs[0].Annotations)
}

func GetGarbageCollection(c echo.Context) error {
	dryrun, _ := strconv.ParseBool(c.QueryParam("dryrun"))
	ttl, _ := time.ParseDuration(models.Conf.Cite.RCRetentionDuration)
	// FOR TEST
	// dryrun = true
	// ttl, _ = time.ParseDuration("1s")
	rcMap := make(map[string]k8sApi.ReplicationController)
	logger.Infof("is dryrun? %v", dryrun)
	logger.Debugf("unused rc ttl: %v", ttl)

	go func() {
		nss, err := k8s.GetAllNamespaces()
		// FOR TEST
		// testNs, _ := k8s.GetNamespace("test")
		// nss = []k8sApi.Namespace{*testNs}
		if err != nil {
			msg := fmt.Sprintf("failed to list namespaces: %v", err)
			logger.Errorf(msg)
			noti.SendSystem(msg)
			return
		}
		for _, ns := range nss {
			if ns.Name == "default" || ns.Name == "kube-system" {
				continue
			}
			logger.Debugf("namespace: %s", ns.Name)

			// get all RCs
			rcs, err := k8s.GetReplicationControllers(ns.Name, map[string]string{})
			if err != nil {
				msg := fmt.Sprintf("failed to list replication controllers on namespace %s: %v", ns.Name, err)
				logger.Errorf(msg)
				noti.SendSystem(msg)
				return
			}

			// convert RC list into map[selector]RC, excluding young RCs
			for _, rc := range rcs {
				if time.Since(rc.CreationTimestamp.Time) < ttl {
					continue
				}
				rcLabels := k8sLabels.FormatLabels(rc.Labels)
				rcMap[rcLabels] = rc
			}

			// get all SVCs
			svcs, err := k8s.GetServices(ns.Name, map[string]string{})
			if err != nil {
				msg := fmt.Sprintf("failed to list service on namespace %s: %v", ns.Name, err)
				logger.Errorf(msg)
				noti.SendSystem(msg)
				return
			}

			for _, svc := range svcs {
				logger.Debugf("service: %s/%s", svc.Namespace, svc.Name)
				svcSelector := k8sLabels.FormatLabels(svc.Spec.Selector)

				// remove active RC from rcMap
				delete(rcMap, svcSelector)
			}
		}

		// remove remaining RCs
		if len(rcMap) > 0 {
			var rcList []string
			for _, rc := range rcMap {
				if !dryrun {
					err = k8s.DeleteReplicationController(rc.Namespace, rc.Name)
					if err != nil {
						msg := fmt.Sprintf("failed to delete RC %s/%s: %v", rc.Namespace, rc.Name, err)
						logger.Errorf(msg)
						noti.SendSystem(msg)
						return
					}
				}
				rcList = append(rcList, fmt.Sprintf("%s/%s", rc.Namespace, rc.Name))
			}
			sort.Strings(rcList)

			msgHead := fmt.Sprintf("* deleted RCs: %v", len(rcMap))
			if dryrun {
				msgHead += " (dryrun)"
			}
			msg := fmt.Sprintf("%s\n%s", msgHead, strings.Join(rcList, "\n"))

			noti.SendSystem(msg)
		}
	}()

	return c.JSON(http.StatusOK, "garbage collection initiated.")
}
