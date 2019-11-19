#! /bin/sh

if [[ -z "${5:-}" ]]; then
  echo "Use: setupcontainercni.sh netns cniif containerip subnetsize gwip" 1>&2
  exit 1
fi

netns=${1}
shift
cniif=${1}
shift
containerip=${1}
shift
subnetsize=${1}
shift
gwip=${1}

ip netns add $netns

#generate device name and create veth, linking it to container device
rand=$(tr -dc 'A-F0-9' < /dev/urandom | head -c4)
hostif="veth$rand"
ip link add $cniif type veth peer name $hostif 

#link $hostif to cni0
ip link set $hostif up 
ip link set $hostif master cni0 

#link cniif, add it to the right namespace and add a route 
ip link set $cniif netns $netns
ip netns exec $netns ip link set $cniif up
ip netns exec $netns ip addr add $containerip/$subnetsize dev $cniif
ip netns exec $netns ip route replace default via $gwip dev $cniif 
