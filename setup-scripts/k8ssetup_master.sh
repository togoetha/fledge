#!/bin/bash

if [[ -z "${1:-}" ]]; then
  echo "Use: wdsetup_master.sh ipaddress usevpn" 1>&2
  exit 1
fi

ipaddr=${1}
shift
usevpn=${1}
shift

echo "### Enabling NAT ###"

sudo /proj/wall2-ilabt-iminds-be/fuse/togoetha/natscript.sh

echo "### Installing Docker ###"

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


#echo "--- Checking VPN ---"

#if [[ ! -z "$usevpn" ]]
#  then
#    servername="vkubetest"

#    echo "--- Setting up VPN server ---"
#    OVPN_DATA="ovpn-data-vkube"

#    mkdir /wdovpn
    #docker volume create --name $OVPN_DATA
#    docker run -v /wdovpn:/etc/openvpn --log-driver=none --rm kylemanna/openvpn ovpn_genconfig -t -u udp://$ipaddr
#    docker run -v /wdovpn:/etc/openvpn --log-driver=none --rm -i kylemanna/openvpn ovpn_initpki "nopass" <<< "$servername"

#    docker run -v /wdovpn:/etc/openvpn --name=ovpn-server -d --net=host -p 1194:1194/udp --cap-add=NET_ADMIN kylemanna/openvpn

    # Allow TUN interface connections to OpenVPN server
#    iptables -A INPUT -i tap+ -j ACCEPT

    # Allow TUN interface connections to be forwarded through other interfaces
#    iptables -A FORWARD -i tap+ -j ACCEPT
#fi

# swap OFF
swapoff -a

echo "--- Installing K8S ---"

# install kubernetes
apt-get install -y apt-transport-https curl
curl -s https://packages.cloud.google.com/apt/doc/apt-key.gpg | apt-key add -
cat <<EOF >/etc/apt/sources.list.d/kubernetes.list
deb https://apt.kubernetes.io/ kubernetes-xenial main
EOF
apt-get update
apt-get install -y kubelet=1.13.8-00 kubeadm=1.13.8-00 kubectl=1.13.8-00
apt-mark hold kubelet kubeadm kubectl

# set kube config
echo "KUBECONFIG=/etc/kubernetes/admin.conf">>/etc/environment
export KUBECONFIG=/etc/kubernetes/admin.conf

echo "--- Starting K8S and installing Flannel ---"

# set up master with support for flannel
kubeadm init --pod-network-cidr=10.244.0.0/16

# install flannel
kubectl apply -f https://raw.githubusercontent.com/coreos/flannel/bc79dd1505b0c8681ece4de4c0d86c5cd2643275/Documentation/kube-flannel.yml

echo "--- Starting dashboard ---"

while [ ! -d "/var/run/secrets/kubernetes.io/serviceaccount" ] ;
do
    echo 'Checking /var/run/secrets/kubernetes.io/serviceaccount'
    sleep 2
done


#dashboard
kubectl apply -f kubernetes-dashboard.yaml

kubectl proxy --accept-hosts=".*" --address="0.0.0.0" &

#echo "--- Setting vkubelet rights and tokens ---"

#while [ ! -d "/var/run/secrets/kubernetes.io/serviceaccount" ] ;
#do
#    echo 'Checking /var/run/secrets/kubernetes.io/serviceaccount'
#    sleep 2
#done

#give admin cluster rights for vkubelet (pods)
#kubectl create -f admin-cluster-rights.yaml

# copy required files for virtual kubelets

#deftoken=$(kubectl get secrets | grep -o -G "default-token-[a-z0-9]*")
#kubectl get secret $deftoken -o=jsonpath="{.data.token}" | base64 -d -i - > /var/run/secrets/kubernetes.io/serviceaccount/token
#cp /etc/kubernetes/pki/ca.crt /var/run/secrets/kubernetes.io/serviceaccount

# set some taints and labels for virtual kubelet deployments

#echo "--- Getting master node name ---"

#line=$(kubectl get nodes | grep 'master')
#parsed=($line)
#masternode=${parsed[0]}

#echo "--- Applying labels for master node " $masternode

#kubectl taint node $masternode node-role.kubernetes.io/master:NoSchedule-
#kubectl label nodes $masternode vpodmaster=true

#echo "--- Starting vkubelet server ---"

echo "--- Setup complete ---"
