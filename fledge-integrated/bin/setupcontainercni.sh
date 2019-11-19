#! /bin/sh

if [[ -z "${6:-}" ]]; then
  echo "Use: setupcontainercni.sh containername pid cniif containerip subnetsize gwip" 1>&2
  exit 1
fi

containername=${1}
shift
pid=${1}
shift
cniif=${1}
shift
containerip=${1}
shift
subnetsize=${1}
shift
gwip=${1}

#create netns folder if it doesn't exist (it should, mounted by Docker)
#soft link the process to the container's network namespace

mkdir -p /var/run/netns
ln -s /proc/$pid/ns/net /var/run/netns/$containername

#generate device name and create veth, linking it to container device
rand=$(tr -dc 'A-F0-9' < /dev/urandom | head -c4)
hostif="veth$rand"
ip link add $cniif type veth peer name $hostif 

#link $hostif to cni0
ip link set $hostif up 
ip link set $hostif master cni0 

#delete any stuff docker made first, we don't want that interfering
ip netns exec $containername ip link delete eth0
ip netns exec $containername ip link delete $cniif

#link cniif, add it to the right namespace and add a route 
ip link set $cniif netns $containername
ip netns exec $containername ip link set $cniif up
ip netns exec $containername ip addr add $containerip/$subnetsize dev $cniif
ip netns exec $containername ip route replace default via $gwip dev $cniif 
