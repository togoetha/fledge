package main

import (
	"fmt"
	"os"

	"log"
	"math"
	"net/http"
	"strings"
	"time"

	//appsv1 "k8s.io/api/apps/v1"
	//apiv1 "k8s.io/api/core/v1"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var config *Config

var cri ContainerRuntimeInterface

var startTime time.Time

//var deployment appsv1.Deployment

func main() {
	argsWithoutProg := os.Args[1:]
	cfgFile := "defaultconfig.json"
	if len(argsWithoutProg) > 0 {
		cfgFile = argsWithoutProg[0]
	}
	config, _ = LoadConfig(cfgFile)
	if config.Runtime == "containerd" {
		cri = (&ContainerdRuntimeInterface{}).Init()
	} else {
		cri = (&DockerRuntimeInterface{}).Init()
	}

	//clientset, _ := GetKubeClient()

	//clientset.AppsV1().Deployments(apiv1.NamespaceDefault)

	startTime = time.Now()

	vkuberouter := VkubeletRouter()
	kuberouter := KubeletRouter()
	go func() {
		fmt.Println("Starting vkuberouter")
		log.Fatal(http.ListenAndServe(":"+config.ServicePort, vkuberouter))
	}()
	go func() {
		NotifyNodeAvailability()
	}()
	fmt.Println("Starting kuberouter")
	log.Fatal(http.ListenAndServe(":"+config.KubeletPort, kuberouter))
}

func GetKubeClient() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	fmt.Println("Config created")
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return clientset, err
	}
	fmt.Println("Clientset created")
	return clientset, nil
}

func NotifyNodeAvailability() {
	if config.VkubeServiceURL != "" {
		fmt.Println("Remote starting virtual kubelet")
		parts := strings.Split(config.DeviceName, ".")

		namelen := int(math.Min(55, float64(len(parts[0]))))
		config.shortDeviceName = config.DeviceName[:namelen]

		err := StartVKubelet(config.shortDeviceName, config.DeviceIP, config.ServicePort, config.KubeletPort)
		if err != nil {
			fmt.Println("Call failed")
			panic(err)
		} else {
			podCidr, err := FetchPodCIDR(config.shortDeviceName)

			if err != nil {
				fmt.Println(err.Error())
			} else {
				fmt.Println("Virtual kubelet started")
				fmt.Printf("Pod subnet %s", podCidr)
				CreateHostCNI(podCidr)
			}
		}
	}
}

func CreateHostCNI(cidrSubnet string) {
	tunaddr, _ := ExecCmdBash("ip address show dev tap0 | grep -E -o '[0-9\\.]{7,15}/'")

	fmt.Printf("Got tun/tap addr %s\n", tunaddr)
	subnetPts := strings.Split(cidrSubnet, "/")
	//bridgeip := subnetPts[0][:len(subnetPts[0])-1] + "1"
	ipPts := strings.Split(subnetPts[0], ".")
	tunPts := strings.Split(tunaddr[0:len(tunaddr)-2], ".")
	subnetIpPts := strings.Split(subnetPts[0], ".")

	//so, obviously some assumptions are made here that need to be cleared up for production grade code
	//the first parameter assumes a pod subnet (cluster wide) of /16, which isn't always the case
	//however, getting the entire subnet from Kubernetes would mean another call just to get that, so for now it's not worth it
	//the last parameter also assumes that the vpn server is on the .1 ip address of the subnet, which may not be the case
	//this is something that's not really easy to work out without knowing how vpn will be deployed in production, so left it like this for now
	cmd := fmt.Sprintf("sh -x /startcni.sh %s %s %s %s", ipPts[0]+"."+ipPts[1]+".0.0", subnetPts[1], subnetIpPts[0]+"."+subnetIpPts[1]+"."+subnetIpPts[2]+".1", tunPts[0]+"."+tunPts[1]+"."+tunPts[2]+".1")

	fmt.Printf("Attempting CNI initialization %s", cmd)
	output, _ := ExecCmdBash(cmd)
	fmt.Println(output)

	InitContainerNetworking(subnetPts[0], subnetPts[1])
}
