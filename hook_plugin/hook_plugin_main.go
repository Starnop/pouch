package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type ContPlugin int

type DPlugin int

const (
	Prefix     = "com.docker.network"
	MacAddress = Prefix + ".endpoint.macaddress"
)

var (
	finalPoint, _   = time.Parse("2006-01-02T15:04:05.000Z", "2099-01-01T00:00:00.000Z")
	ContainerPlugin ContPlugin
	DaemonPlugin    DPlugin
)

func (d DPlugin) PreStartHook() error {
	logrus.Infof("pre-start hook in daemon is called")
	configMap := make(map[string]interface{}, 8)
	homeDir := ""
	if _, ex := os.Stat("/etc/pouch/config.json"); ex == nil {
		f, err := os.OpenFile("/etc/pouch/config.json", os.O_RDONLY, 0)
		if err != nil {
			return err
		}
		err = json.NewDecoder(f).Decode(&configMap)
		f.Close()
		if err != nil {
			return err
		}
		if s, ok := configMap["home-dir"].(string); ok {
			homeDir = s
		}
	}
	setupEnv()
	b, e := exec.Command("/opt/ali-iaas/pouch/bin/daemon_prestart.sh", homeDir).CombinedOutput()
	if e != nil {
		return fmt.Errorf("daemon prestart execute error. %s %v", string(b), e)
	} else {
		fmt.Printf("daemon_prestart output %s\n", string(b))
	}
	go activePlugins()
	return nil
}

func (d DPlugin) PreStopHook() error {
	fmt.Println("pre-stop hook in daemon is called")
	b, e := exec.Command("/opt/ali-iaas/pouch/bin/daemon_prestop.sh").CombinedOutput()
	if e != nil {
		return e
	} else {
		fmt.Printf("daemon_prestop output %s\n", string(b))
	}
	return nil
}

func (c ContPlugin) PreStart(interface{}) ([]int, [][]string, error) {
	fmt.Println("pre start method called")
	//copy file into the ns, put entrypoint in container. like the function of pouch_container_create.sh in old version
	return []int{-100}, [][]string{{"/opt/ali-iaas/pouch/bin/prestart_hook"}}, nil
}

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

func main() {
	fmt.Println(ContainerPlugin, DaemonPlugin)
}
