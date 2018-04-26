package main

import "github.com/alibaba/pouch/apis/types"

type VPlugin int

var VolumePlugin VPlugin

func (d VPlugin) PreVolumeCreate(config *types.VolumeCreateConfig) error {
	if config.Driver == "alilocal" {
		config.Driver = "local"
	}

	return nil
}
