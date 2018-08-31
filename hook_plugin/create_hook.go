package main

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"

	. "github.com/alibaba/pouch/apis/types"
	"github.com/alibaba/pouch/pkg/utils"
	"github.com/sirupsen/logrus"
)

// PreCreate transfer parameters from labels and environments to Config ans HostConfig
// 1. change NetworkMode to bridge if not set
// 2. create network if network doesn't exist
// 3. generate admin uid if env ali_admin_uid=0 exist
// 4. set user to root if running in rich container mode
// 5. convert label DiskQuota to DiskQuota in ContainerConfig parameter
// 6. in rich container mode, change ali_run_mode=common_vm to ali_run_mode=vm
// 7. in rich container mode, convert 3 lables to env
// 8. in rich container mode, add some capabilities by default
// 9. in rich container mode, don't bind /etc/hosts /etc/hostname /etc/resolv.conf files into container
// 10. in rich container mode, set ShmSize to half of the limit of memory
// 11. generate quota id as needed
// 12. set HOSTNAME env if HostName specified
// 13. if VolumesFrom specifed and the container name has a prefix of slash, trim it
// 14. add net-priority into spec-annotations
func (c ContPlugin) PreCreate(createConfig *ContainerCreateConfig) error {
	logrus.Infof("pre create method called")

	if createConfig.HostConfig == nil {
		createConfig.HostConfig = &HostConfig{}
	}
	requestedIP := ""
	if createConfig.HostConfig.NetworkMode == "default" || createConfig.HostConfig.NetworkMode == "" {
		createConfig.HostConfig.NetworkMode = "bridge"
	}
	networkMode := createConfig.HostConfig.NetworkMode
	env := createConfig.Env

	//setup network just in case
	if !strings.HasPrefix(networkMode, "container:") && networkMode != "host" && networkMode != "none" {
		requestedIP = getEnv(env, "RequestedIP")
		defaultRoute := getEnv(env, "DefaultRoute")
		mask := getEnv(env, "DefaultMask")
		nic := getEnv(env, "DefaultNic")
		if createConfig.NetworkingConfig == nil {
			createConfig.NetworkingConfig = &NetworkingConfig{}
		}
		if createConfig.NetworkingConfig.EndpointsConfig == nil {
			createConfig.NetworkingConfig.EndpointsConfig = make(map[string]*EndpointSettings)
		}

		if nwName, e := prepareNetwork(requestedIP, defaultRoute, mask, nic, networkMode,
			createConfig.NetworkingConfig.EndpointsConfig, env); e != nil {
			return e
		} else if nwName != networkMode {
			createConfig.HostConfig.NetworkMode = nwName
		}

		if mustRequestedIP() {
			if len(requestedIP) == 0 {
				return fmt.Errorf("-e RequestedIP not set")
			}
			for _, oneIp := range strings.Split(requestedIP, ",") {
				if net.ParseIP(oneIp) == nil {
					return fmt.Errorf("-e RequestedIP=%s is invalid", requestedIP)
				}
			}
		}
	}

	// generate admin uid
	if getEnv(createConfig.Env, "ali_admin_uid") == "0" && requestedIP != "" {
		if b, ex := exec.Command("/opt/ali-iaas/pouch/bin/get_admin_uid.sh",
			requestedIP).CombinedOutput(); ex != nil {
			logrus.Errorf("get admin uid error, ip is %s, error is %v", requestedIP, ex)
			return ex
		} else {
			if uid, ex := strconv.Atoi(strings.TrimSpace(string(b))); ex != nil {
				logrus.Errorf("get admin uid error, ip is %s, error is %v", requestedIP, ex)
				return ex
			} else {
				for i, oneEnv := range createConfig.Env {
					arr := strings.SplitN(oneEnv, "=", 2)
					if len(arr) == 2 && arr[0] == "ali_admin_uid" {
						createConfig.Env[i] = fmt.Sprintf("%s=%d", arr[0], uid)
						break
					}
				}
			}
		}
	}

	// common vm must run as root
	mode := getEnv(createConfig.Env, "ali_run_mode")
	if ("common_vm" == mode || "vm" == mode) && createConfig.User != "root" {
		logrus.Infof("in common_vm mode, use root user to start container.")
		createConfig.User = "root"
	}

	// setup disk quota
	diskQuota := createConfig.Labels["DiskQuota"]
	if diskQuota != "" && len(createConfig.DiskQuota) == 0 {
		if createConfig.DiskQuota == nil {
			createConfig.DiskQuota = make(map[string]string)
		}
		for _, kv := range strings.Split(diskQuota, ";") {
			kv = strings.TrimSpace(kv)
			if kv == "" {
				continue
			}
			arr := strings.SplitN(kv, "=", 2)
			var k, v string
			if len(arr) == 2 {
				k, v = arr[0], arr[1]
			} else {
				k = ".*"
				v = arr[0]
			}
			createConfig.DiskQuota[k] = v
		}
	}

	// common vm use rich container which introduced by pouch
	if getEnv(env, "ali_run_mode") == "vm" || getEnv(env, "ali_run_mode") == "common_vm" {
		// change common_vm to vm
		for i, line := range createConfig.Env {
			if line == "ali_run_mode=common_vm" {
				createConfig.Env[i] = "ali_run_mode=vm"
			}
		}

		keySet := map[string]struct{}{
			"ali_host_dns":                         {},
			"com_alipay_acs_container_server_type": {},
			"ali_call_scm":                         {},
		}

		// convert label to env
		for k, v := range createConfig.Labels {
			lowerKey := strings.ToLower(k)
			if _, ok := keySet[lowerKey]; !ok {
				continue
			}
			createConfig.Env = append(createConfig.Env, fmt.Sprintf("%s=%s", escapseLableToEnvName(k), v))
		}

		createConfig.HostConfig.CapAdd = append(createConfig.HostConfig.CapAdd, "SYS_RESOURCE", "SYS_MODULE",
			"SYS_PTRACE", "SYS_PACCT", "NET_ADMIN", "SYS_ADMIN")

		createConfig.HostConfig.CapAdd = utils.UniqueStringSlice(createConfig.HostConfig.CapAdd)

		//don't bind /etc/hosts /etc/hostname /etc/resolv.conf files into container
		createConfig.DisableNetworkFiles = true

		if (createConfig.HostConfig.ShmSize == nil || *createConfig.HostConfig.ShmSize == 0) &&
			createConfig.HostConfig.Memory > 0 {
			partOfMemSize := createConfig.HostConfig.Memory / 2
			createConfig.HostConfig.ShmSize = &partOfMemSize
		}
	}

	// generate quota id as needed
	if createConfig.Labels["AutoQuotaId"] == "true" || (diskQuota != "" &&
		!strings.Contains(diskQuota, ";") && !strings.Contains(diskQuota, "=")) {
		if createConfig.QuotaID == "" || createConfig.QuotaID == "0" {
			qid := createConfig.Labels["QuotaId"]
			if qid != "" && qid != "0" {
				createConfig.QuotaID = qid
			} else {
				createConfig.QuotaID = "-1"
			}
		}
	}

	// set hostname to env
	if getEnv(env, "HOSTNAME") == "" && createConfig.Hostname != "" {
		found := false
		for i, line := range createConfig.Env {
			if strings.HasPrefix(line, "HOSTNAME=") {
				createConfig.Env[i] = fmt.Sprintf("HOSTNAME=%s", createConfig.Hostname)
				found = true
				break
			}
		}
		if !found {
			createConfig.Env = append(createConfig.Env, fmt.Sprintf("HOSTNAME=%s", createConfig.Hostname))
		}
	}

	if len(createConfig.HostConfig.VolumesFrom) > 0 {
		for i, one := range createConfig.HostConfig.VolumesFrom {
			createConfig.HostConfig.VolumesFrom[i] = strings.TrimPrefix(one, "/")
		}
	}

	// add net-priority into spec-annotations
	if createConfig.NetPriority != 0 {
		if createConfig.SpecAnnotation == nil {
			createConfig.SpecAnnotation = make(map[string]string)
		}
		createConfig.SpecAnnotation["net-priority"] = strconv.FormatInt(createConfig.NetPriority, 10)
	}
	return nil
}
