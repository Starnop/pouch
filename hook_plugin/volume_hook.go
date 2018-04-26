package main

import "github.com/alibaba/pouch/apis/types"

type VPlugin int

var VolumePlugin VPlugin

func (v VPlugin) PreVolumeCreate(config *types.VolumeCreateConfig) error {
	return nil
}
