package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	. "github.com/alibaba/pouch/apis/types"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	networkLock sync.Mutex
	pouchClient = http.Client{
		Transport: &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return net.Dial("unix", "/var/run/pouchd.sock")
			},
		},
		Timeout: time.Second * 30,
	}
)

func getNetworkMode(m map[string]interface{}) (nm string, err error) {
	hostConfig := m["HostConfig"]
	switch r := hostConfig.(type) {
	case map[string]interface{}:
		networkMode := r["NetworkMode"]
		ok := false
		if nm, ok = networkMode.(string); ok {
			return
		}
		return "", fmt.Errorf("%v of networkMode is not a string\n", networkMode)
	default:
		return "", fmt.Errorf("HostConfig format error in body. %q\n", hostConfig)
	}
}

func getAllEnv(m map[string]interface{}) (arr []string, err error) {
	env := m["Env"]
	switch r := env.(type) {
	case []interface{}:
		ok := false
		ret := make([]string, len(r))
		for i, line := range r {
			if ret[i], ok = line.(string); !ok {
				return arr, fmt.Errorf("%v in env is not a string\n", line)
			}
		}
		return ret, nil
	case []string:
		return r, nil
	default:
		return arr, fmt.Errorf("env format error in body. %q\n", env)
	}
}

func getEnv(env []string, key string) string {
	for _, pair := range env {
		parts := strings.SplitN(pair, "=", 2)
		if parts[0] == key {
			return parts[1]
		}
	}
	return ""
}

func addParamsForOverlay(m map[string]string, env []string) {
	if getEnv(env, "OverlayNetwork") == "true" {
		m["OverlayNetwork"] = "true"
		m["OverlayTunnelId"] = getEnv(env, "OverlayTunnelId")
		m["OverlayGwIp"] = getEnv(env, "OverlayGwIp")
	}
	if getEnv(env, "VpcECS") == "true" {
		m["VpcECS"] = "true"
	}
	for _, oneEnv := range env {
		arr := strings.SplitN(oneEnv, "=", 2)
		if len(arr) == 2 && strings.HasPrefix(arr[0], "alinet_") {
			m[arr[0]] = arr[1]
		}
	}
}

func prepare_network(requestedIP, defaultRoute, mask, nic string, networkMode string, EndpointsConfig map[string]*EndpointSettings, rawEnv []string) (nwName string, err error) {
	nwName = networkMode
	nwIf := nic

	if requestedIP == "" || defaultRoute == "" || mask == "" || nic == "" {
		return
	}

	if nic == "bond0" || nic == "docker0" {
		nwName = "p0_" + defaultRoute
		nwIf = "p0"
	} else if networkMode == "aisnet" {
		nwName = "aisnet_" + defaultRoute
	} else {
		nwName = nwName + "_" + defaultRoute
	}
	if getEnv(rawEnv, "OverlayNetwork") == "true" {
		nwName = nwName + ".overlay"
	}
	fmt.Printf("create container network params %s %s %s %s %s\n", requestedIP, defaultRoute, mask, nic, networkMode)
	if networkMode == "default" || "bridge" == networkMode || networkMode == nwName {
		//create network if not exist
		networkLock.Lock()
		defer networkLock.Unlock()
		nwArr, err := getAllNetwork()
		if err != nil {
			return "", err
		}
		var nw *NetworkInfo
		for _, one := range nwArr.Networks {
			if one != nil && one.Name == nwName {
				nw = one
				break
			}
		}
		if nw == nil {
			//create network since it is not exist
			network := net.IPNet{IP: net.ParseIP(requestedIP).To4(), Mask: net.IPMask(net.ParseIP(mask).To4())}
			nc := NetworkCreate{
				Driver: "alinet",
				IPAM: &IPAM{
					Driver: "alinet",
					Config: []IPAMConfig{{Subnet: network.String(), IPRange: network.String(), Gateway: defaultRoute}},
				},
				Options: map[string]string{
					"nic": nwIf,
				},
			}
			arr := strings.Split(nwIf, ".")
			if len(arr) == 2 && arr[1] != "" {
				nc.Options["vlan-id"] = arr[1]
			}

			createNwReq := NetworkCreateConfig{Name: nwName, NetworkCreate: nc}
			addParamsForOverlay(nc.Options, rawEnv)
			err := CreateNetwork(&createNwReq)
			if err != nil {
				return "", err
			}
		}

		if defaultObj, exist := EndpointsConfig[nwName]; !exist || defaultObj.IPAMConfig == nil {
			EndpointsConfig[nwName] = &EndpointSettings{IPAMConfig: &EndpointIPAMConfig{}}
		}
		if EndpointsConfig[nwName].IPAMConfig.IPV4Address != requestedIP {
			EndpointsConfig[nwName].IPAMConfig.IPV4Address = requestedIP
			//EndpointsConfig[nwName].Time = time.Now().Unix() - 1
			//EndpointsConfig[nwName].SkipResolver = true
		}

		fmt.Printf("create container network params from endpoint config %s %s %s %s %s\n", EndpointsConfig[nwName].IPAMConfig.IPV4Address, defaultRoute, mask, nic, nwName)

	}
	return nwName, nil
}

func getAllNetwork() (nr *NetworkListResp, err error) {
	resp, err := pouchClient.Get("http://127.0.0.1/v1.24/networks")
	if err != nil {
		return
	}
	defer resp.Body.Close()
	nr = &NetworkListResp{}
	if err = json.NewDecoder(resp.Body).Decode(nr); err != nil {
		return
	}
	return
}

func CreateNetwork(c *NetworkCreateConfig) error {
	var rw bytes.Buffer
	err := json.NewEncoder(&rw).Encode(c)
	if err != nil {
		return err
	}
	resp, err := pouchClient.Post("http://127.0.0.1/v1.24/networks/create", "application/json", &rw)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return err
	}
	fmt.Printf("create network return %s\n", string(b))
	if strings.Contains(string(b), "failed") {
		return fmt.Errorf(string(b))
	}
	return nil
}

func mustRequestedIP() bool {
	if b, err := ioutil.ReadFile("/etc/sysconfig/pouch"); err != nil {
		return false
	} else {
		for _, line := range bytes.Split(b, []byte{'\n'}) {
			if bytes.Contains(line, []byte("--must-requested-ip")) && !bytes.HasPrefix(line, []byte("#")) {
				return true
			}
		}
	}
	return false
}

func setupEnv() {
	if b, err := ioutil.ReadFile("/etc/sysconfig/pouch"); err != nil {
		fmt.Printf("read config file error %v\n", err)
	} else {
		for _, line := range bytes.Split(b, []byte{'\n'}) {
			if bytes.Contains(line, []byte("--set-env")) && !bytes.HasPrefix(line, []byte("#")) {
				splitByComma := bytes.Contains(line, []byte("--set-env-comma"))
				splitChar := byte(' ')
				index := -1
				if splitByComma {
					index = bytes.Index(line, []byte("--set-env-comma"))
					if index != -1 {
						index += len("--set-env-comma")
					}
				} else {
					index = bytes.Index(line, []byte("--set-env"))
					if index != -1 {
						index += len("--set-env")
					}
				}
				if index < len(line) {
					splitChar = line[index]
				}

				arr := bytes.SplitN(line, []byte{splitChar}, 2)
				if len(arr) < 2 {
					continue
				}
				val := arr[1]
				var kv [][]byte
				if splitByComma {
					kv = bytes.Split(val, []byte{','})
				} else {
					kv = bytes.Split(val, []byte{':'})
				}
				for _, oneKv := range kv {
					tmpArr := bytes.SplitN(oneKv, []byte{'='}, 2)
					if len(tmpArr) == 2 {
						os.Setenv(string(bytes.TrimSpace(tmpArr[0])), string(bytes.TrimSpace(tmpArr[1])))
					} else {
						os.Setenv(string(bytes.TrimSpace(tmpArr[0])), "")
					}
				}
			}
		}
	}
}

func escapseLableToEnvName(k string) string {
	k = strings.Replace(k, "\\", "_", -1)
	k = strings.Replace(k, "$", "_", -1)
	k = strings.Replace(k, ".", "_", -1)
	k = strings.Replace(k, " ", "_", -1)
	k = strings.Replace(k, "\"", "_", -1)
	k = strings.Replace(k, "'", "_", -1)
	return fmt.Sprintf("label__%s", k)
}

// GenerateMACFromIP returns a locally administered MAC address where the 4 least
// significant bytes are derived from the IPv4 address.
func GenerateMACFromIP(ip net.IP) net.HardwareAddr {
	return genMAC(ip)
}

func genMAC(ip net.IP) net.HardwareAddr {
	hw := make(net.HardwareAddr, 6)
	// The first byte of the MAC address has to comply with these rules:
	// 1. Unicast: Set the least-significant bit to 0.
	// 2. Address is locally administered: Set the second-least-significant bit (U/L) to 1.
	hw[0] = 0x02
	// The first 24 bits of the MAC represent the Organizationally Unique Identifier (OUI).
	// Since this address is locally administered, we can do whatever we want as long as
	// it doesn't conflict with other addresses.
	hw[1] = 0x42
	// Fill the remaining 4 bytes based on the input
	if ip == nil {
		rand.Read(hw[2:])
	} else {
		copy(hw[2:], ip.To4())
	}
	return hw
}
