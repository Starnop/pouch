package main

import (
	"net"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Prestart copy files to container rootfs
func (c ContPlugin) PreStart(interface{}) ([]int, [][]string, error) {
	logrus.Infof("pre start method called")

	// invoke script at /opt/ali-iaas/pouch/bin/start_hook.sh
	// copy file into the ns, put entrypoint in container. like the function of pouch_container_create.sh in old version
	return []int{-100}, [][]string{{"/opt/ali-iaas/pouch/bin/prestart_hook"}}, nil
}

// PreCreateEndpoint does three things before create endpoint:
// 1. pass Overlay parameters to network plugin like alinet
// 2. generate mac address from ip address
// 3. generate priority for the network interface
func (c ContPlugin) PreCreateEndpoint(cid string, env []string) (priority int, disableResolver bool, genericParam map[string]interface{}) {
	genericParam = make(map[string]interface{})
	if getEnv(env, "OverlayNetwork") == "true" {
		genericParam["OverlayNetwork"] = "true"
		genericParam["OverlayTunnelId"] = getEnv(env, "OverlayTunnelId")
		genericParam["OverlayGwIp"] = getEnv(env, "OverlayGwIp")
	}

	if getEnv(env, "VpcECS") == "true" {
		genericParam["VpcECS"] = "true"
	}

	for _, oneEnv := range env {
		arr := strings.SplitN(oneEnv, "=", 2)
		if len(arr) == 2 && strings.HasPrefix(arr[0], "alinet_") {
			genericParam[arr[0]] = arr[1]
		}
	}

	if ip := getEnv(env, "RequestedIP"); ip != "" {
		if strings.Contains(ip, ",") {
			ip = strings.Split(ip, ",")[0]
		}
		genericParam[MacAddress] = GenerateMACFromIP(net.ParseIP(ip))
	}

	return int(finalPoint.Unix() - time.Now().Unix()), true, genericParam
}
