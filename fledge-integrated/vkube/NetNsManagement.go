package vkube

import (
	"fledge/fledge-integrated/manager"
	"fmt"
	v1 "k8s.io/api/core/v1"
)

func GetNetNs(namespace string, pod string) string {
	return namespace + "-" + pod
}

func BindNetNamespace(namespace string, pod string, pid int) string {
	netNs := GetNetNs(namespace, pod)

	ip, _ := RequestIP(namespace, pod)
	cmd := fmt.Sprintf("sh -x ./setupcontainercni.sh %s %d eth1 %s %d %s", netNs, pid, ip, subnetMask, gatewayIP)
	manager.ExecCmdBash(cmd)
	return ip
}

func GetNetworkNamespace(namespace string, pod *v1.Pod) string {
	nsName := namespace + "-" + pod.ObjectMeta.Name
	//cmd := fmt.Sprintf("ip netns add %s", nsName)
	//ExecCmdBash(cmd)

	nsPath := fmt.Sprintf("/var/run/netns/%s", nsName)
	return nsPath
}

/*func SetupNetNamespace(namespace string, pod string) string {
	netNs := GetNetNs(namespace, pod)
	fmt.Printf("Setting up network namespace %s", netNs)
	ip, _ := RequestIP(namespace, pod)
	fmt.Printf("Setting up pod veth netns %s ip %s subnet %d gateway %s", netNs, ip, subnetMask, gatewayIP)
	cmd := fmt.Sprintf("sh -x /setupcontainerveth.sh %s eth1 %s %d %s", netNs, ip, subnetMask, gatewayIP)
	ExecCmdBash(cmd)
	return ip
}*/

func RemoveNetNamespace(namespace string, pod string) {
	netNs := GetNetNs(namespace, pod)
	cmd := fmt.Sprintf("sh -x ./shutdowncontainercni.sh %s eth1", netNs)
	manager.ExecCmdBash(cmd)
}
