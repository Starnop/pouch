package cri

import (
	criv1alpha1 "github.com/alibaba/pouch/cri/v1alpha1"
	servicev1alpha1 "github.com/alibaba/pouch/cri/v1alpha1/service"
	criv1alpha2 "github.com/alibaba/pouch/cri/v1alpha2"
	servicev1alpha2 "github.com/alibaba/pouch/cri/v1alpha2/service"
	"github.com/alibaba/pouch/ctrd"
	"github.com/alibaba/pouch/daemon/config"
	"github.com/alibaba/pouch/daemon/mgr"

	"github.com/sirupsen/logrus"
)

// RunCriService start cri service if pouchd is specified with --enable-cri.
func RunCriService(daemonconfig *config.Config, client ctrd.APIClient, containerMgr mgr.ContainerMgr, imageMgr mgr.ImageMgr, stopCh chan error) {
	var err error

	defer func() {
		stopCh <- err
		close(stopCh)
	}()
	if !daemonconfig.IsCriEnabled {
		return
	}
	switch daemonconfig.CriConfig.CriVersion {
	case "v1alpha1":
		runv1alpha1(daemonconfig, client, containerMgr, imageMgr)
	case "v1alpha2":
		runv1alpha2(daemonconfig, client, containerMgr, imageMgr)
	default:
		logrus.Errorf("invalid CRI version,failed to start CRI service")
	}
	return
}

// Start CRI service with CRI version: v1alpha1
func runv1alpha1(daemonconfig *config.Config, client ctrd.APIClient, containerMgr mgr.ContainerMgr, imageMgr mgr.ImageMgr) {
	logrus.Infof("Start CRI service with CRI version: v1alpha1")
	criMgr, err := criv1alpha1.NewCriManager(daemonconfig, client, containerMgr, imageMgr)
	if err != nil {
		logrus.Errorf("failed to get CriManager with error: %v", err)
		return
	}

	service, err := servicev1alpha1.NewService(daemonconfig, criMgr)
	if err != nil {
		logrus.Errorf("failed to start CRI service with error: %v", err)
		return
	}

	grpcServerCloseCh := make(chan struct{})
	go func() {
		if err := service.Serve(); err != nil {
			logrus.Errorf("failed to start grpc server: %v", err)
		}
		close(grpcServerCloseCh)
	}()

	streamServerCloseCh := make(chan struct{})
	go func() {
		if err := criMgr.StreamServerStart(); err != nil {
			logrus.Errorf("failed to start stream server: %v", err)
		}
		close(streamServerCloseCh)
	}()

	<-streamServerCloseCh
	logrus.Infof("CRI Stream server stopped")
	<-grpcServerCloseCh
	logrus.Infof("CRI GRPC server stopped")

	logrus.Infof("CRI service stopped")
	return
}

// Start CRI service with CRI version: v1alpha2
func runv1alpha2(daemonconfig *config.Config, client ctrd.APIClient, containerMgr mgr.ContainerMgr, imageMgr mgr.ImageMgr) {
	logrus.Infof("Start CRI service with CRI version: v1alpha2")
	criMgr, err := criv1alpha2.NewCriManager(daemonconfig, client, containerMgr, imageMgr)
	if err != nil {
		logrus.Errorf("failed to get CriManager with error: %v", err)
		return
	}

	service, err := servicev1alpha2.NewService(daemonconfig, criMgr)
	if err != nil {
		logrus.Errorf("failed to start CRI service with error: %v", err)
		return
	}
	// TODO: Stop the whole CRI service if any of the critical service exits
	grpcServerCloseCh := make(chan struct{})
	go func() {
		if err := service.Serve(); err != nil {
			logrus.Errorf("failed to start grpc server: %v", err)
		}
		close(grpcServerCloseCh)
	}()

	streamServerCloseCh := make(chan struct{})
	go func() {
		if err := criMgr.StreamServerStart(); err != nil {
			logrus.Errorf("failed to start stream server: %v", err)
		}
		close(streamServerCloseCh)
	}()
	// TODO: refactor it with select
	<-streamServerCloseCh
	logrus.Infof("CRI Stream server stopped")
	<-grpcServerCloseCh
	logrus.Infof("CRI GRPC server stopped")

	logrus.Infof("CRI service stopped")
	return
}
