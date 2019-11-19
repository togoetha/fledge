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

echo "--- Installing Containerd ---"
modprobe overlay
modprobe br_netfilter

# Setup required sysctl params, these persist across reboots.
cat > /etc/sysctl.d/99-kubernetes-cri.conf <<EOF
net.bridge.bridge-nf-call-iptables  = 1
net.ipv4.ip_forward                 = 1
net.bridge.bridge-nf-call-ip6tables = 1
EOF

sysctl --system
apt-get install -y libseccomp2

# Export required environment variables.
export CONTAINERD_VERSION="1.2.4"
# Download containerd tar.
wget https://storage.googleapis.com/cri-containerd-release/cri-containerd-${CONTAINERD_VERSION}.linux-amd64.tar.gz
# Unpack.
tar --no-overwrite-dir -C / -xzf cri-containerd-${CONTAINERD_VERSION}.linux-amd64.tar.gz
# Start containerd.
systemctl start containerd

rm cri-containerd-${CONTAINERD_VERSION}.linux-amd64.tar.gz

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
    ctr images pull docker.io/togoetha/openvpn-client:latest
    ctr containers create --privileged --net-host --mount type=bind,src=/ovpn/$devname.ovpn,dst=/vpn/client.ovpn,options=rbind:ro --mount type=bind,src=/dev/net/tun,dst=/dev/net/tun,options=rbind:rw docker.io/togoetha/openvpn-client:latest vpn-client openvpn --config /vpn/client.ovpn
    ctr tasks start -d vpn-client
#    docker run --privileged -d --name vpn-client --cap-add=NET_ADMIN --net=host -v /ovpn/$devname.ovpn:/vpn/client.ovpn togoetha/openvpn-client --config /vpn/client.ovpn

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
mkdir -p /var/run/netns/
mkdir -p /ctdtmp

echo "{ \"runtime\":\"containerd\", \"deviceName\":\""$devname\"", \"deviceIP\":\""$ipAddr\"", \"servicePort\":\""8100"\", \"kubeletPort\":\""8101"\", \"vkubeServiceURL\":\""$vkubeurl"\",\"ignoreKubeProxy\":\""$ignorekproxy"\"  }" > /var/vkube/vkubeconfig.json

# network ns is taken care of with the --net-host flag below. Hopefully.
devname=$(hostname)
mkdir -p /var/run/secrets/kubernetes.io/serviceaccount
cp /proj/wall2-ilabt-iminds-be/fuse/togoetha/token /var/run/secrets/kubernetes.io/serviceaccount
cp /proj/wall2-ilabt-iminds-be/fuse/togoetha/ca.crt /var/run/secrets/kubernetes.io/serviceaccount

cd ../fledge-integrated/bin
env KUBERNETES_SERVICE_HOST=10.2.0.166 KUBERNETES_SERVICE_PORT=6443 ./fledge-integrated-amd64 "/var/vkube/vkubeconfig.json"
