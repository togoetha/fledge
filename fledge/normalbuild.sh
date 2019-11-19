#!/bin/bash

go build -o bin/vkubelet-service-amd64 -ldflags "-s -w"  *.go
cp *.sh bin/
cd bin && rm *build.sh && rm setupcontainerveth.sh
