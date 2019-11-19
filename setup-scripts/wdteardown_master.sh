#! /bin/sh

#find a way to signal the service that we're stopping
#simple wget?

kubectl drain $(kubectl get nodes)
kubectl delete node $(kubectl get nodes)

kubeadm reset

docker stop ovpn-server
docker rm ovpn-server


#routes and CNI devices should have been removed by the service, after all, it initialized them
