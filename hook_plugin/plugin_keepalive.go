package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/pouch/apis/types"
	"github.com/sirupsen/logrus"
)

var pluginLock sync.Mutex
var collectdClient = &http.Client{Timeout: 20 * time.Second}

func activePlugins() {
	activePluginsOnce()
	go cleanVmcommonDir()

	for range time.NewTicker(time.Second * 30).C {
		activePluginsOnce()
	}
}

func cleanVmcommonDir() {
	for range time.NewTicker(time.Minute * 5).C {
		files, err := ioutil.ReadDir(homeDir)
		if err != nil {
			logrus.Errorf("read graph dir error. %s %v", homeDir, err)
			continue
		}
		existDir := make(map[string]struct{})
		for _, oneFile := range files {
			if !oneFile.IsDir() {
				continue
			}
			oneDir := filepath.Join(homeDir, oneFile.Name(), "top_foot_vm")
			if fi, ex := os.Stat(oneDir); ex == nil && fi.IsDir() {
				existDir[oneDir] = struct{}{}
			}
		}
		var ca []string
		var c types.Container
		var afterWait bool
	checkIfExist:
		if len(existDir) > 0 {
			if afterWait {
				time.Sleep(time.Minute * 5)
			}
			ca, err = getAllContainers()
			if err != nil {
				logrus.Errorf("get all containers error %v", err)
				continue
			}
			for _, id := range ca {
				c, err = getOneContainers(id)
				if err != nil {
					logrus.Errorf("get one container error. %s %v", id, err)
					break
				}
				for _, oneMount := range c.Mounts {
					delete(existDir, oneMount.Source)
				}
			}
			if err != nil {
				continue
			}
			if afterWait {
				for oneDir := range existDir {
					logrus.Infof("remove dir %s because it is useless", oneDir)
					os.RemoveAll(oneDir)
				}
			} else {
				afterWait = true
				goto checkIfExist
			}
		}
	}
}

func activePluginsOnce() {
	pluginLock.Lock()
	defer pluginLock.Unlock()
	for _, one := range strings.Split(os.Getenv("EmbedPlugins"), ",") {
		if one == "" {
			continue
		}
		if one == "collectd" {
			resp, err := collectdClient.Get("http://127.0.0.1:5678/debug/version")
			if err == nil && resp.StatusCode == http.StatusOK {
				io.Copy(ioutil.Discard, resp.Body)
				resp.Body.Close()
				continue
			}
		} else {
			socketPath := fmt.Sprintf("/run/docker/plugins/%s/%s.sock", one, one)
			if _, ex := os.Stat(socketPath); ex == nil {
				c, err := net.Dial("unix", socketPath)
				if err == nil {
					c.Close()
					continue
				}
				os.Remove(socketPath)
			}
		}

		if f, e := os.OpenFile(fmt.Sprintf("/var/log/%s.log", one), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); e == nil {
			plugin := fmt.Sprintf("/opt/ali-iaas/pouch/plugins/%s", one)
			activeOne := osexec.Cmd{
				Path:   plugin,
				Args:   []string{plugin, getNodeIp()},
				Stdout: f,
				Stderr: f,
			}
			if one == "aisnet" {
				activeOne.Args[1] = "-d"
			}
			if one == "nvidia-docker" {
				if _, err := os.Stat("/usr/bin/nvidia-modprobe"); err != nil {
					continue
				} else {
					activeOne.Args = []string{
						plugin,
						"-s", "/run/pouch/plugins/nvidia-docker/"}
				}
			}
			if e = activeOne.Start(); e != nil {
				logrus.Errorf("start plugins error %s, %v", one, e)
			} else {
				logrus.Infof("start plugin success. %s", one)
				go func() {
					activeOne.Wait()
				}()
			}
			f.Close()
		}
	}
}

func getNodeIp() string {
	if b, e := osexec.Command("hostname", "-i").CombinedOutput(); e == nil {
		scanner := bufio.NewScanner(bytes.NewReader(b))
		for scanner.Scan() {
			ip := scanner.Text()
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}
	return ""
}
