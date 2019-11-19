#!/bin/sh

if [[ -z "${4:-}" ]]; then
  echo "Use: startcni.sh subnet mask bridgeip externalrouteip" 1>&2
  exit 1
fi

subnet=${1}
shift
mask=${1}
shift
bridgeip=${1}
shift
routeip=${1}

brctl addbr cni0
ip link set cni0 up
ip addr add $bridgeip/$mask dev cni0

#find a proper way to add other pod subnets?
#below is easy but pretty dirty
ip route add $subnet/16 via $routeip dev tap0

iptables -t filter -A FORWARD -s $subnet/16 -j ACCEPT
iptables -t filter -A FORWARD -d $subnet/16 -j ACCEPT