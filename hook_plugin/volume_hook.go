package main

import "github.com/alibaba/pouch/apis/types"

type VPlugin int

var VolumePlugin VPlugin

func (v VPlugin) PreVolumeCreate(config *types.VolumeCreateConfig) error {
	if config.Driver != "alilocal" {
		return nil
	}
	config.Driver = "local"

	// set driver alias in local.
	if config.DriverOpts == nil {
		config.DriverOpts = make(map[string]string)
	}
	config.DriverOpts["driver.alias"] = "alilocal"

	return nil
}
