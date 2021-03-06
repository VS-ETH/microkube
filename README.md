# Microkube
A small tool to quickly bootstrap a kubernetes cluster against a local docker daemon

[![Build Status](https://travis-ci.com/vs-eth/microkube.svg?branch=master)](https://travis-ci.com/vs-eth/microkube)
[![Go Report Card](https://goreportcard.com/badge/github.com/vs-eth/microkube?style=flat)](https://goreportcard.com/report/github.com/vs-eth/microkube)
[![codecov](https://codecov.io/gh/vs-eth/microkube/branch/master/graph/badge.svg)](https://codecov.io/gh/vs-eth/microkube)
[![Go Doc](https://img.shields.io/badge/godoc-reference-blue.svg?style=flat)](http://godoc.org/github.com/vs-eth/microkube)
[![Release](https://img.shields.io/github/tag/vs-eth/microkube.svg?style=flat)](https://github.com/vs-eth/microkube/releases/latest)

## Motivation
##### Debugging 'Kubernetes Apps' 
Traditionally, when debugging kubernetes applications locally, you'd use minikube.
However, this quickly results in quite some overhead:
* Local directories are only accessible from the cluster if you set up NFS
* Service IPs are inaccessible, you have to manually use `kubectl proxy` or expose them on the minikube node
* Cluster DNS is hidden
* Local images have to be pushed to some registry which is nontrivial as unencrypted docker registries normally trigger Dockers Insecure registry logic

## How it works
Microkube generates configuration and certificates and then starts kubernetes
*directly* against a local docker daemon. At the moment it only supports Linux
due to directly launching kube-proxy, but in theory Windows support should be
possible at some point

## Installation
Microkube can be installed either as a package or by building everthing manually, see below.

### Dev Setup
* You need `etcd`, `hyperkube` and the default CNI plugins. The easiest way to get them is to use the [microkube-deps](https://github.com/vs-eth/microkube-deps) repo and invoking `./build.sh`. This will build kubernetes, so you'll require about 15 GB of free disk space
* If you want to run tests or run microkube from the repository, create a folder `third_party` in the repository root and copy all binaries there
* If you're only interested in running `microkubed` from the command line, you can also specify the folder with the binaries as `-extra-bin-dir`
* Running it requires `pkexec` from Polkit (for obtaining root for `kube-proxy` and `kubelet`) and `conntrack` + `iptables` for `kube-proxy`
* Unittests additionally require the `openssl` command line utility
* Regenerating the log parser requires [ldetool](https://github.com/sirkon/ldetool)
* To build everything, do `make`
* Try running `./microkubed -verbose`

### Packaging
Apart from Docker, you'll need `kubernetes-hyperkube`, `etcd-server` and `cni-plugins`. Deployment happens
on Debian 9 while CI runs Ubuntu 14.04, so any deb-based Linux still supported by Docker should do. To build
the package, simply do `dpkg-buildpackage -us -uc -b`. If everything works correctly, the packages should
appear in the parent directory.
The installation process will automatically create a service user and grant it the necessary privileges via
sudoers, so in this case you don't need to worry about this. After installation, microkube can be started
using `sudo systemctl start microkubed`.

## License
Apache 2.0