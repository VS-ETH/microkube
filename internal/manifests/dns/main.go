/*
 * THIS FILE IS AUTOGENERATED by github.com/uubk/microkube/cmd/codegen/Manifest.go
 * DO NOT TOUCH.
 * In case of issues, please fix the code generator ;)
 */

package main

import (
	"flag"
	"github.com/mitchellh/go-homedir"
	log "github.com/sirupsen/logrus"
	"github.com/uubk/microkube/internal/manifests"
	"time"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "~/.mukube/kube/kubeconfig", "Path to Kubeconfig")
	flag.Parse()
	var err error
	*kubeconfig, err = homedir.Expand(*kubeconfig)
	if err != nil {
		log.WithError(err).WithField("root", *kubeconfig).Fatal("Couldn't expand kubeconfig")
	}
	obj := manifests.NewDNS()
	err = obj.ApplyToCluster(*kubeconfig)
	if err != nil {
		log.WithError(err).WithField("root", *kubeconfig).Fatal("Couldn't apply object to cluster")
	}
	err = obj.InitHealthCheck(*kubeconfig)
	if err != nil {
		log.WithError(err).WithField("root", *kubeconfig).Fatal("Couldn't enable health checks")
	}
	ok := false
	for i := 0; i < 10 && !ok; i++ {
		ok, err = obj.IsHealthy()
		if err != nil {
			log.WithError(err).WithField("root", *kubeconfig).Fatal("Couldn't enable health checks")
		}
		if ok {
			break
		}
		time.Sleep(1 * time.Second)
	}
	log.WithField("status", ok).Info("Health check done")
}