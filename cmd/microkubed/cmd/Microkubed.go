/*
 * Copyright 2018 The microkube authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Package cmd contains the implementation of the microkubed command
package cmd

import (
	"context"
	"github.com/coreos/go-systemd/daemon"
	log "github.com/sirupsen/logrus"
	"github.com/vs-eth/microkube/internal/cmd"
	log2 "github.com/vs-eth/microkube/internal/log"
	"github.com/vs-eth/microkube/internal/manifests"
	"github.com/vs-eth/microkube/pkg/handlers"
	"github.com/vs-eth/microkube/pkg/handlers/etcd"
	"github.com/vs-eth/microkube/pkg/handlers/kube"
	"github.com/vs-eth/microkube/pkg/helpers"
	kube2 "github.com/vs-eth/microkube/pkg/kube"
	"github.com/vs-eth/microkube/pkg/pki"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path"
	"strings"
	"time"
)

// serviceConstructor describes a function that can create a service, given the I/O handlers
type serviceConstructor func(handlers.OutputHandler, handlers.ExitHandler) (handlers.ServiceHandler, error)

// serviceEntry describes all information about a running service needed to check it's health and stop it
type serviceEntry struct {
	exitChan   chan bool
	healthChan chan handlers.HealthMessage
	handler    handlers.ServiceHandler
	name       string
}

// Microkubed handles an invocation of the 'microkubed' command line tool
type Microkubed struct {
	// Are we in 'graceful termination mode', that is, everything has started successfully and in order to stop we
	// should first stop all pods?
	gracefulTerminationMode bool
	// List of service handlers. This is used to handle shutdown on fatal errors
	serviceHandlers []handlers.ServiceHandler
	// Base directory of microkubed, which is where all state will be stored
	baseDir string
	// Extra binary dir which is added to the binary search path
	extraBinDir string

	// CIDR of pod IPs (commandline argument)
	podRangeNet *net.IPNet
	// CIDR of service IPs (commandline argument)
	serviceRangeNet *net.IPNet
	// CIDR of pod and service network combined
	clusterIPRange *net.IPNet

	// Struct with all credentials needed for any given service
	cred *pki.MicrokubeCredentials
	// Template struct with information needed for execution of programs
	baseExecEnv handlers.ExecutionEnvironment

	// Path to etcd server binary
	etcdBin string
	// Path to hyperkube binary
	hyperkubeBin string

	// A list of running services
	serviceList []serviceEntry
	// Whether to deploy the kubernetes dashboard cluster addon
	enableKubeDash bool
	// Whether to deploy the CoreDNS cluster addon
	enableDns bool
	// Kubernetes client used for checking node status and service information
	kCl *kube2.KubeClient
}

// Create directories and copy CNI plugins if appropriate
func (m *Microkubed) createDirectories() {
	cmd.EnsureDir(m.baseDir, "", 0770)
	cmd.EnsureDir(m.baseDir, "kube", 0770)
	cmd.EnsureDir(m.baseDir, "etcdtls", 0770)
	cmd.EnsureDir(m.baseDir, "kubesched", 0770)
	cmd.EnsureDir(m.baseDir, "kubetls", 0770)
	cmd.EnsureDir(m.baseDir, "kubectls", 0770)
	cmd.EnsureDir(m.baseDir, "kubestls", 0770)
	cmd.EnsureDir(m.baseDir, "etcddata", 0770)

	// Special case: in case the extra binaries directory contains CNI plugins, copy them to the right location
	cmd.EnsureDir(m.baseDir, path.Join("kube", "kubelet"), 0755)
	cmd.EnsureDir(m.baseDir, path.Join("kube", "kubelet", "cni"), 0755)
	cniPlugins := []string{
		"bridge",
		"host-local",
		"loopback",
	}
	for _, plugin := range cniPlugins {
		pluginPath, err := helpers.FindBinary(plugin, m.baseDir, m.extraBinDir)
		if err == nil {
			_, err := os.Stat(path.Join(m.baseDir, "kube", "kubelet", "cni", plugin))
			if err != nil {
				destPath := path.Join(m.baseDir, "kube", "kubelet", "cni", plugin)
				err = os.Link(pluginPath, destPath)
				if err != nil {
					// Try to copy :/
					lctx := log.WithFields(log.Fields{
						"src":       path.Join(m.extraBinDir, plugin),
						"dest":      path.Join(m.baseDir, "kube", "kubelet", "cni", plugin),
						"app":       "microkube",
						"component": "prep",
					})
					lctx.WithError(err).Info("Couldn't link CNI plugin, trying to copy")
					// Delete the destination, just in case we got a 0 byte file above...
					os.Remove(destPath)
					in, err := os.Open(pluginPath)
					if err != nil {
						lctx.WithError(err).Fatal("Couldn't open source file")
					}
					out, err := os.OpenFile(destPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
					if err != nil {
						lctx.WithError(err).Fatal("Couldn't open destination file")
					}
					n, err := io.Copy(in, out)
					if err != nil {
						lctx.WithError(err).Fatal("Couldn't copy file")
					}
					if n == 0 {
						lctx.WithError(err).Fatal("File copy is empty!")
					}
					in.Close()
					out.Close()
				}
			}
		}
	}
}

// Find binaries
func (m *Microkubed) findBinaries() {
	var err error
	m.etcdBin, err = helpers.FindBinary("etcd", m.baseDir, m.extraBinDir)
	if err != nil {
		log.WithError(err).Fatal("Couldn't find etcd binary")
	}
	m.hyperkubeBin, err = helpers.FindBinary("hyperkube", m.baseDir, m.extraBinDir)
	if err != nil {
		log.WithError(err).Fatal("Couldn't find hyperkube binary")
	}
}

// Start etcd
func (m *Microkubed) startEtcd() {
	etcdHandler, etcdChan, etcdHealthChan := m.startService("etcd", func(etcdOutputHandler handlers.OutputHandler,
		etcdExitHandler handlers.ExitHandler) (handlers.ServiceHandler, error) {

		execEnv := handlers.ExecutionEnvironment{
			Binary:        m.etcdBin,
			ExitHandler:   etcdExitHandler,
			OutputHandler: etcdOutputHandler,
			Workdir:       path.Join(m.baseDir, "etcddata"),
		}
		execEnv.CopyInformationFromBase(&m.baseExecEnv)
		return etcd.NewEtcdHandler(execEnv, m.cred), nil
	}, log2.NewETCDLogParser())
	m.serviceHandlers = append(m.serviceHandlers, etcdHandler)
	log.Info("ETCD ready")

	m.serviceList = append(m.serviceList, serviceEntry{
		handler:    etcdHandler,
		exitChan:   etcdChan,
		healthChan: etcdHealthChan,
		name:       "etcd",
	})
}

// Start Kube APIServer
func (m *Microkubed) startKubeAPIServer() {
	log.Info("Starting kube api server...")
	kubeAPIHandler, kubeAPIChan, kubeAPIHealthChan := m.startService("kube-apiserver",
		func(kubeAPIOutputHandler handlers.OutputHandler,
			kubeAPIExitHandler handlers.ExitHandler) (handlers.ServiceHandler, error) {

			execEnv := handlers.ExecutionEnvironment{
				Binary:        m.hyperkubeBin,
				ExitHandler:   kubeAPIExitHandler,
				OutputHandler: kubeAPIOutputHandler,
			}
			execEnv.CopyInformationFromBase(&m.baseExecEnv)
			return kube.NewKubeAPIServerHandler(execEnv, m.cred, m.serviceRangeNet.String()), nil
		}, log2.NewKubeLogParser("kube-api"))
	m.serviceHandlers = append(m.serviceHandlers, kubeAPIHandler)
	log.Info("Kube api server ready")

	// Generate kubeconfig for kubelet and kubectl
	log.Info("Generating kubeconfig...")
	kubeconfig := path.Join(m.baseDir, "kube/", "kubeconfig")
	_, err := os.Stat(kubeconfig)
	if err != nil {
		log.Debug("Creating kubeconfig")
		err = kube.CreateClientKubeconfig(m.baseExecEnv, m.cred, kubeconfig, m.baseExecEnv.ListenAddress.String())
		if err != nil {
			log.WithError(err).Fatal("Couldn't create kubeconfig!")
			return
		}
	}
	m.cred.Kubeconfig = kubeconfig

	m.serviceList = append(m.serviceList, serviceEntry{
		handler:    kubeAPIHandler,
		exitChan:   kubeAPIChan,
		healthChan: kubeAPIHealthChan,
		name:       "kube-api",
	})
}

// Start controller-manager
func (m *Microkubed) startKubeControllerManager() {
	log.Info("Starting controller-manager...")
	kubeCtrlMgrHandler, kubeCtrlMgrChan, kubeCtrlMgrHealthChan := m.startService("kube-controller-manager",
		func(kubeCtrlMgrOutputHandler handlers.OutputHandler,
			kubeCtrlMgrExitHandler handlers.ExitHandler) (handlers.ServiceHandler, error) {

			execEnv := handlers.ExecutionEnvironment{
				Binary:        m.hyperkubeBin,
				ExitHandler:   kubeCtrlMgrExitHandler,
				OutputHandler: kubeCtrlMgrOutputHandler,
			}
			execEnv.CopyInformationFromBase(&m.baseExecEnv)
			return kube.NewControllerManagerHandler(execEnv, m.cred, m.podRangeNet.String()), nil
		}, log2.NewKubeLogParser("kube-controller-manager"))
	m.serviceHandlers = append(m.serviceHandlers, kubeCtrlMgrHandler)
	log.Info("Kube controller-manager ready")

	m.serviceList = append(m.serviceList, serviceEntry{
		handler:    kubeCtrlMgrHandler,
		exitChan:   kubeCtrlMgrChan,
		healthChan: kubeCtrlMgrHealthChan,
		name:       "kube-controller-manager",
	})
}

// Start scheduler
func (m *Microkubed) startKubeScheduler() {
	log.Info("Starting kube-scheduler...")
	kubeSchedHandler, kubeSchedChan, kubeSchedHealthChan := m.startService("kube-scheduler",
		func(kubeSchedOutputHandler handlers.OutputHandler,
			kubeSchedExitHandler handlers.ExitHandler) (handlers.ServiceHandler, error) {

			execEnv := handlers.ExecutionEnvironment{
				Binary:        m.hyperkubeBin,
				Workdir:       path.Join(m.baseDir, "kubesched"),
				ExitHandler:   kubeSchedExitHandler,
				OutputHandler: kubeSchedOutputHandler,
			}
			execEnv.CopyInformationFromBase(&m.baseExecEnv)
			return kube.NewKubeSchedulerHandler(execEnv, m.cred)
		}, log2.NewKubeLogParser("kube-scheduler"))
	m.serviceHandlers = append(m.serviceHandlers, kubeSchedHandler)
	log.Info("Kube-scheduler ready")

	m.serviceList = append(m.serviceList, serviceEntry{
		handler:    kubeSchedHandler,
		exitChan:   kubeSchedChan,
		healthChan: kubeSchedHealthChan,
		name:       "kube-scheduler",
	})
}

// Start kubelet
func (m *Microkubed) startKubelet() {
	log.Info("Starting kubelet...")
	kubeletHandler, kubeletChan, kubeletHealthChan := m.startService("kubelet",
		func(kubeletOutputHandler handlers.OutputHandler,
			kubeletExitHandler handlers.ExitHandler) (handlers.ServiceHandler, error) {

			execEnv := handlers.ExecutionEnvironment{
				Binary:        m.hyperkubeBin,
				Workdir:       path.Join(m.baseDir, "kube"),
				ExitHandler:   kubeletExitHandler,
				OutputHandler: kubeletOutputHandler,
			}
			execEnv.CopyInformationFromBase(&m.baseExecEnv)
			return kube.NewKubeletHandler(execEnv, m.cred)
		}, log2.NewKubeLogParser("kubelet"))
	m.serviceHandlers = append(m.serviceHandlers, kubeletHandler)
	log.Info("Kubelet ready")

	m.serviceList = append(m.serviceList, serviceEntry{
		handler:    kubeletHandler,
		exitChan:   kubeletChan,
		healthChan: kubeletHealthChan,
		name:       "kubelet",
	})
}

// Start kube-proxy
func (m *Microkubed) startKubeProxy() {
	log.Info("Starting kube-proxy...")
	kubeProxyHandler, kubeProxyChan, kubeProxyHealthChan := m.startService("kube-proxy",
		func(output handlers.OutputHandler, exit handlers.ExitHandler) (handlers.ServiceHandler, error) {

			execEnv := handlers.ExecutionEnvironment{
				Binary:        m.hyperkubeBin,
				Workdir:       path.Join(m.baseDir, "kube"),
				ExitHandler:   exit,
				OutputHandler: output,
			}
			execEnv.CopyInformationFromBase(&m.baseExecEnv)
			return kube.NewKubeProxyHandler(execEnv, m.cred, m.clusterIPRange.String())
		}, log2.NewKubeLogParser("kube-proxy"))
	defer kubeProxyHandler.Stop()
	m.serviceHandlers = append(m.serviceHandlers, kubeProxyHandler)
	log.Info("kube-proxy ready")

	m.serviceList = append(m.serviceList, serviceEntry{
		handler:    kubeProxyHandler,
		exitChan:   kubeProxyChan,
		healthChan: kubeProxyHealthChan,
		name:       "kube-proxy",
	})
}

func (m *Microkubed) checkService(handler serviceEntry) {
	unhealthyCount := 0
	for {
		select {
		case <-handler.exitChan:
			if !m.gracefulTerminationMode {
				log.Fatal("Service " + handler.name + " exitted, aborting!")
			}
		case msg := <-handler.healthChan:
			if !msg.IsHealthy {
				log.WithFields(log.Fields{
					"app":   handler.name,
					"count": unhealthyCount,
				}).Warn("unhealthy!")
				unhealthyCount++
				if unhealthyCount == 10 {
					log.WithFields(log.Fields{
						"app":   handler.name,
						"count": unhealthyCount,
					}).Fatal("Too many failed health checks, aborting!")
				}
			} else {
				log.WithField("app", handler.name).Debug("healthy")
				unhealthyCount = 0
			}
		}
	}
}

// Start periodic health checks
func (m *Microkubed) enableHealthChecks() {
	for _, handler := range m.serviceList {
		log.WithField("app", handler.name).Debug("Enabling health check...")
		handler.handler.EnableHealthChecks(handler.healthChan, true)
		go m.checkService(handler)
	}
}

// Wait until node is ready
func (m *Microkubed) waitUntilNodeReady() chan bool {
	var err error
	m.kCl, err = kube2.NewKubeClient(path.Join(m.baseDir, "kube/", "kubeconfig"))
	if err != nil {
		log.WithError(err).Fatalf("Couldn't init kube client")
	}
	log.Info("Waiting for node...")
	m.kCl.WaitForNode(context.Background())
	// Since we got to this point: Handle quitting gracefully (that is stop all pods!)
	sigChan := make(chan os.Signal, 1)
	exitChan := make(chan bool, 1)
	go func() {
		<-sigChan
		log.Info("Shutting down...")
		m.kCl.DrainNode(context.Background())
		exitChan <- true
	}()
	// Unregister "terminate immediately" serviceHandlers set during startup
	signal.Reset(os.Interrupt, os.Kill)
	// Register ordinary exit handler
	signal.Notify(sigChan, os.Interrupt, os.Kill)
	log.Info("Exit handler enabled...")
	m.gracefulTerminationMode = true

	return exitChan
}

// startServices deploys certain manifests into the cluster
func (m *Microkubed) startServices() {
	services := []manifests.KubeManifestConstructor{}
	if m.enableKubeDash {
		services = append(services, manifests.NewKubeDash)
	}
	if m.enableDns {
		services = append(services, manifests.NewDNS)
	}
	kmri := manifests.KubeManifestRuntimeInfo{
		ExecEnv: m.baseExecEnv,
	}

	for _, service := range services {
		manifest, err := service(kmri)
		logCtx := log.WithFields(log.Fields{
			"app":       "microkube",
			"component": "services",
			"service":   manifest.Name(),
		})
		if err != nil {
			logCtx.WithError(err).Warn("Couldn't init service!")
			continue
		}

		err = manifest.ApplyToCluster(m.cred.Kubeconfig)
		if err != nil {
			logCtx.WithError(err).Warn("Couldn't apply service to cluster!")
			continue
		}
		err = manifest.InitHealthCheck(m.cred.Kubeconfig)
		if err != nil {
			logCtx.WithError(err).Warn("Couldn't initialize health check!")
			continue
		}

		go func() {
			// Delay first report since the service needs some time to start
			time.Sleep(30 * time.Second)
			for {
				ok, err := manifest.IsHealthy()
				if !ok {
					logCtx.WithError(err).Warn("Service is unhealthy!")
				} else {
					logCtx.Debug("Service is healthy")
				}
				time.Sleep(10 * time.Second)
			}
		}()
	}
}

func printIndented(message string) {
	msg := ""
	if message == "" {
		msg = strings.Repeat("#", 120)
	} else {
		pad := (120 - len(message) - 2) / 2
		msg = msg + strings.Repeat("#", pad)
		msg = msg + " " + message + " "
		msg = msg + strings.Repeat("#", pad)
	}
	log.Info(msg)
}

func (m *Microkubed) PrintInfoMessage() {
	printIndented("")
	printIndented("Microkube is up!")
	printIndented("")
	printIndented("Information")
	log.Info("# To access the cluster, use the kubeconfig at '" + m.cred.Kubeconfig + "'")
	log.Info("# Example:")
	log.Info("# kubectl --kubeconfig " + m.cred.Kubeconfig + " get service --all-namespaces")
	log.Info("# The following 'Cluster Addons' are available:")

	if m.enableKubeDash {
		ip, port := m.kCl.FindService("kubernetes-dashboard")
		secret := m.kCl.FindDashboardAdminSecret()
		if ip != "" && port == 443 && secret != "" {
			log.Info("# Kubernetes Dashboard at https://" + ip)
			log.Info("# Sign in with Token: " + secret)
			log.Info("# You might need to remove the line breaks first, depending on your terminal emulator :/")
		}
	}
	if m.enableDns {
		ip, port := m.kCl.FindService("kube-dns")
		if ip != "" && port == 53 {
			log.Info("# Core DNS at " + ip + "")
		}
	}
	printIndented("")
}

// Run the actual command invocation. This function will not return until the program should exit
func (m *Microkubed) Run() {
	argHandler := cmd.NewArgHandler(true)
	m.baseExecEnv = *argHandler.HandleArgs()
	m.baseDir = argHandler.BaseDir
	m.extraBinDir = argHandler.ExtraBinDir
	m.podRangeNet = argHandler.PodRangeNet
	m.serviceRangeNet = argHandler.ServiceRangeNet
	m.clusterIPRange = argHandler.ClusterIPRange
	m.enableDns = argHandler.EnableDns
	m.enableKubeDash = argHandler.EnableKubeDash

	if !argHandler.Verbose {
		log2.GetLoggerFor("etcd").SetLevel(log.FatalLevel)
		log2.GetLoggerFor("kube").SetLevel(log.FatalLevel)
	}

	m.gracefulTerminationMode = false
	log.RegisterExitHandler(func() {
		// Fatal() will not run the normal exit serviceHandlers, therefore, we need to run them manually. However, after
		// startup, this shouldn't be used anymore
		if !m.gracefulTerminationMode {
			for _, h := range m.serviceHandlers {
				h.Stop()
			}
		}
	})

	m.start()

	exitChan := m.waitUntilNodeReady()

	m.enableHealthChecks()
	// All good. Launch stuff
	m.startServices()
	// Print info message if allowed
	m.PrintInfoMessage()
	daemon.SdNotify(false, daemon.SdNotifyReady)

	// Wait until exit
	<-exitChan
	log.WithField("app", "microkube").Info("Exit signal received, stopping now.")
	daemon.SdNotify(false, daemon.SdNotifyStopping)
	for _, h := range m.serviceHandlers {
		h.Stop()
	}

	// Give services time to stop. If we exit immediately, systemd will simply kill them.
	time.Sleep(7 * time.Second)

	return
}

// start starts all cluster services
func (m *Microkubed) start() {
	m.createDirectories()
	m.cred = &pki.MicrokubeCredentials{}
	err := m.cred.CreateOrLoadCertificates(m.baseDir, m.baseExecEnv.ListenAddress, m.baseExecEnv.ServiceAddress)
	if err != nil {
		log.WithError(err).Fatal("Couldn't init credentials!")
	}

	m.findBinaries()

	m.startEtcd()
	m.startKubeAPIServer()
	m.startKubeControllerManager()
	m.startKubeScheduler()
	m.startKubelet()
	m.startKubeProxy()
}

// Starts a service. This function takes care of setting up the infrastructure required by a service constructor
func (m *Microkubed) startService(name string, constructor serviceConstructor,
	logParser log2.Parser) (handlers.ServiceHandler, chan bool, chan handlers.HealthMessage) {

	outputHandler := func(output []byte) {
		err := logParser.HandleData(output)
		if err != nil {
			log.WithError(err).Warn("Couldn't parse log line!")
		}
	}
	stateChan := make(chan bool, 2)
	healthChan := make(chan handlers.HealthMessage, 2)
	exitHandler := func(success bool, exitError *exec.ExitError) {
		log.WithFields(log.Fields{
			"success": success,
			"app":     name,
		}).WithError(exitError).Error(name + " stopped!")
		if !m.gracefulTerminationMode {
			log.WithFields(log.Fields{
				"success": success,
				"app":     name,
			}).WithError(exitError).Fatal("App exitted during startup phase, bailing out _now_")
		}
		stateChan <- success
	}

	serviceHandler, err := constructor(outputHandler, exitHandler)
	if err != nil {
		log.WithError(err).Fatal("Couldn't create " + name + " handler")
	}
	err = serviceHandler.Start()
	if err != nil {
		log.WithError(err).Fatal("Couldn't start " + name)
	}

	msg := handlers.HealthMessage{
		IsHealthy: false,
	}
	for retries := 0; retries < 8 && !msg.IsHealthy; retries++ {
		time.Sleep(1 * time.Second)
		serviceHandler.EnableHealthChecks(healthChan, false)
		msg = <-healthChan
		log.WithFields(log.Fields{
			"app":    name,
			"health": msg.IsHealthy,
		}).Debug("Healthcheck")
	}
	if !msg.IsHealthy {
		log.WithError(msg.Error).Fatal(name + " didn't become healthy in time!")
	}

	return serviceHandler, stateChan, healthChan
}
