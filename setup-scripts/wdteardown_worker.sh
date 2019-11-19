#! /bin/sh

#find a way to signal the service that we're stopping
#simple wget?

wget http://127.0.0.1:8100/shutDown

docker stop vkubelet-service
docker rm vkubelet-service
docker image rm togoetha/vkubelet-service

docker stop vpn-client
docker rm vpn-client
docker image rm togoetha/openvpn-client

#remove cni0
ip link set cni0 down
brctl delbr cni0

#routes and CNI devices should have been removed by the service, after all, it initialized them
