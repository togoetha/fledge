#! /bin/sh

#find a way to signal the service that we're stopping
#simple wget?

wget http://127.0.0.1:8100/shutDown

ctr tasks kill vkubelet-service
sleep 3
ctr container rm vkubelet-service
ctr image rm docker.io/togoetha/vkubelet-service-amd64:latest
ctr image rm docker.io/togoetha/vkubelet-service-arm64:latest

ctr tasks kill vpn-client
sleep 3
ctr container rm vpn-client
ctr image rm docker.io/togoetha/openvpn-client:latest

#remove cni0
ip link set cni0 down
brctl delbr cni0

#routes and CNI devices should have been removed by the service, after all, it initialized them
