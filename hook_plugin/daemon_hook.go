package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
)

// PreStartHook copy config from /etc/pouch/config.json to /etc/sysconfig/pouch
// and start plugin processes than daemon depended
func (d DPlugin) PreStartHook() error {
	logrus.Infof("pre-start hook in daemon is called")
	configMap := make(map[string]interface{}, 8)
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
		logrus.Infof("daemon_prestart output %s", string(b))
	}
	go activePlugins()
	return nil
}

// PreStopHook stops plugin processes than start ed by PreStartHook.
func (d DPlugin) PreStopHook() error {
	logrus.Infof("pre-stop hook in daemon is called")
	b, e := exec.Command("/opt/ali-iaas/pouch/bin/daemon_prestop.sh").CombinedOutput()
	if e != nil {
		return e
	} else {
		logrus.Infof("daemon_prestop output %s", string(b))
	}
	return nil
}
