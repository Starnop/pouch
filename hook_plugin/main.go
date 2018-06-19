package main

import (
	"fmt"
	"time"
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
	homeDir         string
)

func main() {
	fmt.Println(ContainerPlugin, DaemonPlugin, VolumePlugin)
}
