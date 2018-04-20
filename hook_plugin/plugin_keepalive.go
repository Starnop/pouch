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
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

var pluginLock sync.Mutex
var collectdClient = &http.Client{Timeout: 20 * time.Second}

func activePlugins() {
	activePluginsOnce()
	for range time.NewTicker(time.Second * 30).C {
		activePluginsOnce()
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
