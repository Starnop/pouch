package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os/exec"
	"strconv"
	"strings"

	. "github.com/alibaba/pouch/apis/types"
)

func (c ContPlugin) PreCreate(in io.ReadCloser) (io.ReadCloser, error) {
	fmt.Println("pre create method called")
	inputBuffer, err := ioutil.ReadAll(in)
	if err != nil {
		return nil, err
	}
	fmt.Printf("create container with body %s\n", string(inputBuffer))

	//create network if exist
	var createConfig ContainerCreateConfig
	err = json.NewDecoder(bytes.NewReader(inputBuffer)).Decode(&createConfig)
	if err != nil {
		return nil, err
	}
	if createConfig.HostConfig == nil {
		//fixme: something changed
		createConfig.HostConfig = &HostConfig{}
	}
	requestedIP := ""
	if createConfig.HostConfig.NetworkMode == "default" {
		//fixme: something changed
		createConfig.HostConfig.NetworkMode = "bridge"
	}
	networkMode := createConfig.HostConfig.NetworkMode
	if err != nil {
		return nil, err
	}
	env := createConfig.Env

	//setup network just in case
	if !strings.HasPrefix(networkMode, "container:") && networkMode != "host" && networkMode != "none" {
		requestedIP = getEnv(env, "RequestedIP")
		defaultRoute := getEnv(env, "DefaultRoute")
		mask := getEnv(env, "DefaultMask")
		nic := getEnv(env, "DefaultNic")
		if createConfig.NetworkingConfig == nil {
			//fixme: something changed
			createConfig.NetworkingConfig = &NetworkingConfig{}
		}
		if createConfig.NetworkingConfig.EndpointsConfig == nil {
			//fixme: something changed
			createConfig.NetworkingConfig.EndpointsConfig = make(map[string]*EndpointSettings)
		}

		if nwName, e := prepare_network(requestedIP, defaultRoute, mask, nic, networkMode,
			createConfig.NetworkingConfig.EndpointsConfig, env); e != nil {
			return nil, e
		} else if nwName != networkMode {
			//fixme: something changed
			createConfig.HostConfig.NetworkMode = nwName
		}

		if mustRequestedIP() {
			if len(requestedIP) == 0 {
				return nil, fmt.Errorf("-e RequestedIP not set")
			}
			for _, oneIp := range strings.Split(requestedIP, ",") {
				if net.ParseIP(oneIp) == nil {
					return nil, fmt.Errorf("-e RequestedIP=%s is invalid", requestedIP)
				}
			}
		}
	}

	// generate quota id as needed
	//if createConfig.Labels["AutoQuotaId"] == "true" {
	//	if createConfig.Labels["QuotaId"] == "" {
	//		if qid, e := GetNextQuatoId(); e != nil {
	//			return nil, e
	//		} else {
	//			//fixme: something changed
	//			createConfig.Labels["QuotaId"] = strconv.Itoa(int(qid))
	//		}
	//	} else {
	//		fmt.Printf("container already has quota id %s\n", createConfig.Labels["QuotaId"])
	//	}
	//}

	if getEnv(createConfig.Env, "ali_admin_uid") == "0" && requestedIP != "" {
		if b, ex := exec.Command("/opt/ali-iaas/pouch/bin/get_admin_uid.sh",
			requestedIP).CombinedOutput(); ex != nil {
			fmt.Printf("get admin uid error, ip is %s, error is %v\n", requestedIP, ex)
			return nil, ex
		} else {
			if uid, ex := strconv.Atoi(strings.TrimSpace(string(b))); ex != nil {
				fmt.Printf("get admin uid error, ip is %s, error is %v\n", requestedIP, ex)
				return nil, ex
			} else {
				for i, oneEnv := range createConfig.Env {
					arr := strings.SplitN(oneEnv, "=", 2)
					if len(arr) == 2 && arr[0] == "ali_admin_uid" {
						//fixme: something changed
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
		fmt.Printf("in common_vm mode, use root user to start container.\n")
		//fixme: something changed
		createConfig.User = "root"
		for i, line := range createConfig.Env {
			if line == "ali_run_mode=common_vm" {
				createConfig.Env[i] = "ali_run_mode=vm"
			}
		}
	}

	// setup disk quota
	if diskQuota := createConfig.Labels["DiskQuota"]; diskQuota != "" &&
		len(createConfig.DiskQuota) == 0 {
		//fixme: something changed
		if createConfig.DiskQuota == nil {
			createConfig.DiskQuota = make(map[string]string)
		}
		for _, kv := range strings.Split(diskQuota, ",") {
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
	if getEnv(env, "ali_run_mode") == "vm" {
		////fixme: something changed
		//createConfig.Rich = true
		//createConfig.RichMode = "systemd"

		// convert label to env
		for k, v := range createConfig.Labels {
			env = append(env, fmt.Sprintf("%s=%s", escapseLableToEnvName(k), v))
		}

		//fixme: something changed
		createConfig.Env = env
	}

	// setup quotaId
	if createConfig.Labels["AutoQuotaId"] == "true" {
		if createConfig.QuotaID == "" || createConfig.QuotaID == "0" {
			//fixme: something changed
			createConfig.QuotaID = "-1"
		}
	}

	// marshal it as stream and return to the caller
	var out bytes.Buffer
	err = json.NewEncoder(&out).Encode(createConfig)
	fmt.Printf("after process create container body is %s\n", string(out.Bytes()))

	return ioutil.NopCloser(&out), err
}
