package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	//"github.com/gorilla/mux"
)

//GET /startVirtualKubelet?devName=[devName]&serviceUrl=[serviceUrl]&kubeletPort=[kubeletPort]
func StartVirtualKubelet(w http.ResponseWriter, r *http.Request) {
	fmt.Println("StartVirtualKubelet")
	values := r.URL.Query()

	devName := values["devName"][0]
	serviceIP := values["serviceIP"][0]
	servicePort := values["servicePort"][0]
	kubeletPort := values["kubeletPort"][0]

	serviceURL := "http://" + serviceIP + ":" + servicePort

	fmt.Printf("Starting virtual kubelet devName %s serviceUrl %s kubeletPort %s\n", devName, serviceURL, kubeletPort)

	err := StartKubelet(devName, serviceURL, kubeletPort)
	if err != nil {
		fmt.Println(err.Error())
		w.WriteHeader(404)
	}
}

func GetVkubeletPodCIDR(w http.ResponseWriter, r *http.Request) {
	fmt.Println("GetVkubeletPodCIDR")
	values := r.URL.Query()

	devName := values["devName"][0]

	fmt.Printf("GetVkubeletPodCIDR devName %s\n", devName)

	//get pod cidr and public IP of the node
	podcidr, publicIP, _ := GetNodePodCIDR(devName)

	//send back the pod cidr, the vkubelet will need this to set up routes and organize the pod network
	fmt.Fprintf(w, "%s", *podcidr)

	//create the route to the pod subnetwork on this side
	CreateRoute(podcidr, publicIP)
}

//GET /stopVirtualKubelet?devName=[devName]
func StopVirtualKubelet(w http.ResponseWriter, r *http.Request) {
	fmt.Println("StopVirtualKubelet")
	values := r.URL.Query()

	devName := values["devName"][0]

	fmt.Printf("devName %s\n", devName)

	err := StopKubelet(devName)
	if err != nil {
		w.WriteHeader(404)
	}
}

func BuildVPNClient(w http.ResponseWriter, r *http.Request) {
	fmt.Println("BuildVPNClient")
	values := r.URL.Query()

	devName := values["devName"][0]

	fmt.Printf("devName %s\n", devName)

	cmd := fmt.Sprintf("docker run -v /wdovpn:/etc/openvpn --log-driver=none --rm -i kylemanna/openvpn easyrsa build-client-full %s nopass", devName)
	_, err := ExecCmdBash(cmd)

	cmd = fmt.Sprintf("docker run -v /wdovpn:/etc/openvpn --log-driver=none --rm kylemanna/openvpn ovpn_getclient %s", devName)
	res, err := ExecCmdBash(cmd)

	if err != nil {
		w.WriteHeader(404)
	} else {
		fmt.Fprintf(w, "%s", res)
	}
}

func CreateRoute(podCidr *string, publicIP *string) {
	fmt.Printf("Creating route for subnet %s on machine %s\n", *podCidr, *publicIP)
	//ip route get <ip> | grep -E -o 'via [0-9\.]* dev [a-z0-9]*'
	cmd := fmt.Sprintf("ip route get %s | grep -E -o '[0-9\\.]* dev [a-z0-9]*'", *publicIP)
	route, err := ExecCmdBash(cmd)

	if err != nil {
		fmt.Printf("Error retrieving route for %s", *publicIP)
		fmt.Println(err.Error())
	}
	//add the route
	cmd = fmt.Sprintf("ip route add %s via %s", *podCidr, route)
	output, err := ExecCmdBash(cmd)

	if err != nil {
		fmt.Printf("Error adding route for %s", *podCidr)
		fmt.Println(err.Error())
		fmt.Println(output)
	}
}

func GetSecret(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()

	secretName := values["secretName"][0]
	fmt.Printf("GetSecret secretName %s\n", secretName)

	secret, err := GetKubeSecret(secretName)

	if err == nil {
		if err := json.NewEncoder(w).Encode(secret); err != nil {
			panic(err)
		}
	} else {
		w.WriteHeader(404)
	}
}

func GetConfigMap(w http.ResponseWriter, r *http.Request) {
	values := r.URL.Query()

	mapName := values["mapName"][0]
	fmt.Printf("GetConfigMap mapName %s\n", mapName)

	cfgmap, err := GetKubeConfigMap(mapName)

	if err == nil {
		if err := json.NewEncoder(w).Encode(cfgmap); err != nil {
			fmt.Println(err.Error())
		}
	} else {
		fmt.Println(err.Error())
		w.WriteHeader(404)
	}
}
