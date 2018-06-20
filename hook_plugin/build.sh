#! /bin/bash

git status | grep modified | grep ".go" | awk '{print $2}' | xargs -I@ gofmt -l -w @

root=$(cd ../..;pwd)
#GOOS=linux CGO_ENABLED=1 GOARCH=amd64 GOPATH=$root:$root/Godeps/_workspace:$root/src/$1 go build -v -ldflags " -extldflags -static " -buildmode=plugin
# http://colobu.com/2017/08/26/panic-on-go-plugin-Open-for-different-plugins/
name=plugins_$(date +%s)
GOOS=linux GOPATH=$GOPATH:$root:$root/Godeps/_workspace:$root/src/$1 go build -buildmode=plugin -ldflags "-pluginpath=$name" -o hook_plugin.so
