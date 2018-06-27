package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/magiconair/properties"
	"github.com/sirupsen/logrus"
)

// PostUpdate updates env file /etc/profile.d/dockernv.sh and /etc/instanceInfo
func (c ContPlugin) PostUpdate(rootfs string, env []string) error {
	logrus.Infof("post update method called")

	// if rootfs not exist, return
	if _, err := os.Stat(rootfs); err != nil {
		return nil
	}

	var (
		str              string
		propertiesEnvStr string
	)
	for _, kv := range env {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) == 1 {
			parts = append(parts, "")
		}
		if len(parts[1]) > 0 && !strings.Contains(parts[0], ".") {
			s := strings.Replace(parts[1], "\\", "\\\\", -1)
			s = strings.Replace(s, "\"", "\\\"", -1)
			s = strings.Replace(s, "$", "\\$", -1)
			if parts[0] == "PATH" {
				// Note(cao.yin): refer to https://aone.alipay.com/project/532482/task/9745028
				s = parts[1] + ":$PATH"
			}
			str += fmt.Sprintf("export %s=\"%s\"\n", parts[0], s)
			propertiesEnvStr += fmt.Sprintf("env_%s=%s\n", parts[0], s)
		}
	}
	ioutil.WriteFile(filepath.Join(rootfs, "/etc/profile.d/pouchenv.sh"), []byte(str), 0755)

	properenv, err := os.Create(filepath.Join(rootfs, "/etc/instanceInfo"))
	if err != nil {
		return fmt.Errorf("Create env properties file faield: %v", err)
	}
	defer properenv.Close()

	p, err := properties.LoadString(propertiesEnvStr)
	if err != nil {
		return fmt.Errorf("Properties unable to load env string: %v", err)
	}
	_, err = p.Write(properenv, properties.ISO_8859_1)
	if err != nil {
		return fmt.Errorf("Unable to write container's env to properties file: %v", err)
	}

	return nil
}
