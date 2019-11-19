#!/bin/bash

if [[ -z "${3:-}" ]]; then
  echo "Use: wdsetup_worker.sh usevpn(true/false) interface vkubeurl" 1>&2
  exit 1
fi

usevpn=${1}
shift
interface=${1}
shift
vkubeurl=${1}
shift
ignorekproxy=${1}

echo "--- Enabling NAT ---"

sudo /proj/wall2-ilabt-iminds-be/fuse/togoetha/natscript.sh

echo "--- Installing Docker ---"
# Install Docker CE
## Set up the repository:
### Update the apt package index
### Install packages to allow apt to use a repository over HTTPS
    apt-get update && apt-get install -y apt-transport-https ca-certificates curl software-properties-common

### Add Docker’s official GPG key
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | apt-key add -

### Add docker apt repository.
    add-apt-repository \
    "deb [arch=amd64] https://download.docker.com/linux/ubuntu \
    $(lsb_release -cs) \
    stable"

## Install docker ce.
apt-get update && apt-get install -y docker-ce

echo "--- Checking VPN ---"

if [[ ! -z "$usevpn" ]]
  then
    echo "--- Fetching profile ---"
    mkdir /ovpn
    devname=$(hostname)
    wget -O /ovpn/client.ovpn "$vkubeurl/getVpnClient?devName=$devname"

    cat /ovpn/client.ovpn | grep -v 'redirect-gateway' > /ovpn/$devname.ovpn
    echo "comp-lzo no" >> /ovpn/$devname.ovpn

    echo "--- Setting up VPN client ---"
    docker run --privileged -d --name vpn-client --cap-add=NET_ADMIN --net=host -v /ovpn/$devname.ovpn:/vpn/client.ovpn togoetha/openvpn-client --config /vpn/client.ovpn

    # Allow TUN interface connections to OpenVPN server
    iptables -A INPUT -i tap+ -j ACCEPT

    # Allow TUN interface connections to be forwarded through other interfaces
    iptables -A FORWARD -i tap+ -j ACCEPT
fi

echo "--- Getting IP Address ---"

#intcreated=1
ipAddr=""

while [[ -z $ipAddr ]] #! $intcreated -eq 0 ]
do
  echo "--- Checking interface ---"
  #ifconfig $interface 
  #intcreated=$?
  ipAddr=$(ifconfig $interface | grep -o -E 'inet [a-z:]*[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}' | grep -o -E '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}')
  sleep 2
done

#ipAddr=$(ifconfig $interface | grep -o -E 'inet addr:[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}' | grep -o -E '[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}')

echo "--- Starting vkubelet service on ip address $ipAddr, port 8100, vpn $usevpn, master node at $vkubeurl ---" 

mkdir -p /var/vkube/mounts/
echo "{ \"runtime\":\"docker\", \"deviceName\":\""$devname\"", \"deviceIP\":\""$ipAddr\"", \"servicePort\":\""8100"\", \"kubeletPort\":\""8101"\", \"vkubeServiceURL\":\""$vkubeurl"\",\"ignoreKubeProxy\":\""$ignorekproxy"\" }" > /var/vkube/vkubeconfig.json

devname=$(hostname)
docker pull togoetha/vkubelet-service-amd64
docker run -d --net=host --restart always --privileged --name=vkubelet-service \
 -v /var/run/netns:/var/run/netns -v /proc:/host/proc -v /var/vkube/mounts/:/var/vkube/mounts/ \
 -v /var/run/docker.sock:/var/run/docker.sock -v /var/vkube/vkubeconfig.json:/config/vkubeconfig.json \
 togoetha/vkubelet-service-amd64 "/config/vkubeconfig.json"

