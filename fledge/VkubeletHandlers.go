package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var reInsideWhtsp = regexp.MustCompile(`\s+`)

//POST /createPod Pod JSON
func CreatePod(w http.ResponseWriter, r *http.Request) {
	fmt.Println("CreatePod")
	pod := GetPodFromRequest(r)

	//fmt.Println(pod.Spec)
	json, _ := json.Marshal(pod)
	fmt.Println(string(json))

	name := pod.ObjectMeta.Name
	namespace := pod.ObjectMeta.Namespace

	fmt.Printf("Creating pod namespace %s name %s\n", namespace, name)

	containers := pod.Spec.Containers
	restartPolicy := pod.Spec.RestartPolicy
	useHostnetwork := pod.Spec.HostNetwork

	fmt.Printf("Creating pod num containers %d restart policy %s use host network %t\n", len(containers), restartPolicy, useHostnetwork)

	cri.DeployPod(pod)
}

//PUT /updatePod Pod JSON
func UpdatePod(w http.ResponseWriter, r *http.Request) {
	fmt.Println("UpdatePod")
	//PrintBody(r)
	pod := GetPodFromRequest(r)
	//pod := &deserializedPod

	name := pod.ObjectMeta.Name
	namespace := pod.ObjectMeta.Namespace

	fmt.Printf("Updating pod namespace %s name %s\n", namespace, name)

	cri.UpdatePod(pod)
	//cri.UpdatePodStatus(namespace, pod)
}

//DELETE /deletePod Pod JSON
func DeletePod(w http.ResponseWriter, r *http.Request) {
	fmt.Println("DeletePod")
	//PrintBody(r)
	pod := GetPodFromRequest(r)
	//pod := &deserializedPod

	name := pod.ObjectMeta.Name
	namespace := pod.ObjectMeta.Namespace

	fmt.Printf("Deleting pod namespace %s name %s\n", namespace, name)

	cri.DeletePod(pod)
	//UpdatePodStatus(namespace, pod)
}

//GET /getPod?namespace=[namespace]&name=[pod name]
func GetPod(w http.ResponseWriter, r *http.Request) {
	fmt.Println("GetPod")
	values := r.URL.Query()

	namespace := values["namespace"][0]
	name := values["name"][0]

	fmt.Printf("namespace %s name %s\n", namespace, name)

	pod, found := cri.GetPod(namespace, name)

	if found {
		//might wanna update some values first tho
		//cri.UpdatePodStatus(namespace, pod)
		if err := json.NewEncoder(w).Encode(pod); err != nil {
			panic(err)
		}
	} else {
		w.WriteHeader(404)
	}
}

//GET /getContainerLogs?namespace=[namespace]&podName=[pod name]&containerName=[container name]&tail=[tail value]
func GetContainerLogs(w http.ResponseWriter, r *http.Request) {
	fmt.Println("GetContainerLogs")
	values := r.URL.Query()
	namespace := values["namespace"][0]
	podName := values["podName"][0]
	containerName := values["containerName"][0]
	tail := values["tail"][0]

	fmt.Printf("namespace %s pod name %s container name %s tail %s\n", namespace, podName, containerName, tail)

	reader := cri.FetchContainerLogs(namespace, podName, containerName, tail, true)
	if reader != nil {
		buf := new(bytes.Buffer)
		buf.ReadFrom(*reader)
		logs := buf.String()
		fmt.Fprintf(w, "%s", logs)
	} else {
		w.WriteHeader(404)
	}
}

//GET /getPods
func GetPods(w http.ResponseWriter, r *http.Request) {
	fmt.Println("GetPods")

	podSpecs := cri.GetPods()
	//again, might wanna update some values first tho
	pods := []v1.Pod{}
	for _, value := range podSpecs {
		//cri.UpdatePodStatus(value.ObjectMeta.Namespace, value)
		pods = append(pods, *value)
	}

	if err := json.NewEncoder(w).Encode(pods); err != nil {
		panic(err)
	}
}

//GET /getPodStatus?namespace=[namespace]&name=[pod name]
func GetPodStatus(w http.ResponseWriter, r *http.Request) {
	fmt.Println("GetPodStatus")
	values := r.URL.Query()
	namespace := values["namespace"][0]
	name := values["name"][0]

	fmt.Printf("namespace %s name %s\n", namespace, name)

	pod, found := cri.GetPod(namespace, name)

	if found {
		//might wanna update some values first tho
		//cri.UpdatePodStatus(namespace, pod)
		if err := json.NewEncoder(w).Encode(pod.Status); err != nil {
			panic(err)
		}
	} else {
		w.WriteHeader(404)
	}
}

func Capacity(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Capacity")

	resources := v1.ResourceList{} //make(map[v1.ResourceName]string)
	cpu, _ := resource.ParseQuantity(CpuCores())
	resources[v1.ResourceCPU] = cpu
	mem, _ := resource.ParseQuantity(TotalMemory() + "Mi")
	resources[v1.ResourceMemory] = mem
	stor, _ := resource.ParseQuantity(TotalStorage() + "i")
	resources[v1.ResourceStorage] = stor
	pods, _ := resource.ParseQuantity("2")
	resources[v1.ResourcePods] = pods

	contResources := GetContainerResources()

	resources[v1.ResourceRequestsCPU] = *contResources[v1.ResourceRequestsCPU]
	resources[v1.ResourceRequestsMemory] = *contResources[v1.ResourceRequestsMemory]
	resources[v1.ResourceRequestsStorage] = *contResources[v1.ResourceRequestsStorage]
	resources[v1.ResourceLimitsCPU] = *contResources[v1.ResourceLimitsCPU]
	resources[v1.ResourceLimitsMemory] = *contResources[v1.ResourceLimitsCPU]

	//fmt.Println(resources)
	if err := json.NewEncoder(w).Encode(resources); err != nil {
		panic(err)
	}
}

func NodeConditions(w http.ResponseWriter, r *http.Request) {
	fmt.Println("NodeConditions")
	conditionReady := v1.NodeCondition{Type: v1.NodeReady, Status: v1.ConditionTrue, LastHeartbeatTime: metav1.Now(), LastTransitionTime: metav1.Now(), Reason: "Started", Message: "Rocket ranger, ready to rock it"}

	var memPressure v1.ConditionStatus
	if IsMemoryPressure() {
		memPressure = v1.ConditionTrue
	} else {
		memPressure = v1.ConditionFalse
	}
	conditionMemPressure := v1.NodeCondition{Type: v1.NodeMemoryPressure, Status: memPressure, LastHeartbeatTime: metav1.Now(), LastTransitionTime: metav1.Now(), Reason: "Memory pressure", Message: "We're giving 'er all she's got captain"}

	var storagePressure v1.ConditionStatus
	if IsStoragePressure() {
		storagePressure = v1.ConditionTrue
	} else {
		storagePressure = v1.ConditionFalse
	}
	conditionStoragePressure := v1.NodeCondition{Type: v1.NodeDiskPressure, Status: storagePressure, LastHeartbeatTime: metav1.Now(), LastTransitionTime: metav1.Now(), Reason: "Storage pressure", Message: "She won't take it much longer"}

	var storageFull v1.ConditionStatus
	if IsStorageFull() {
		storageFull = v1.ConditionTrue
	} else {
		storageFull = v1.ConditionFalse
	}
	conditionStorageFull := v1.NodeCondition{Type: v1.NodeOutOfDisk, Status: storageFull, LastHeartbeatTime: metav1.Now(), LastTransitionTime: metav1.Now(), Reason: "Storage full", Message: "He's dead Jim"}

	//add more conditions later, with info from cmd
	conditions := []v1.NodeCondition{conditionReady, conditionMemPressure, conditionStoragePressure, conditionStorageFull}

	if err := json.NewEncoder(w).Encode(conditions); err != nil {
		panic(err)
	}
}

func NodeAddresses(w http.ResponseWriter, r *http.Request) {
	fmt.Println("NodeAddresses")
	nodenameStr, _ := ExecCmdBash("hostname")
	nodename := strings.TrimSuffix(nodenameStr, "\n")
	addresshost := v1.NodeAddress{Type: v1.NodeHostName, Address: nodename}
	addressip := v1.NodeAddress{Type: v1.NodeInternalIP, Address: config.DeviceIP}
	addresses := []v1.NodeAddress{addresshost, addressip}

	if err := json.NewEncoder(w).Encode(addresses); err != nil {
		panic(err)
	}
}

func ShutDown(w http.ResponseWriter, r *http.Request) {
	err := StopVirtualKubelet(config.shortDeviceName)
	if err != nil {
		fmt.Printf("Failed to stop vkubelet %s\n", err.Error())
	} else {

	}
}

func GetPodFromRequest(r *http.Request) *v1.Pod {
	decoder := json.NewDecoder(r.Body)
	var pod v1.Pod
	err := decoder.Decode(&pod)
	if err != nil {
		//panic(err)
	}

	return &pod
}

//don't know enough go to write this in a decent way yet
//then again, there's probably no decent way to write this in go
//table flip
func GetContainerResources() map[v1.ResourceName]*resource.Quantity {
	cpuRequest, _ := resource.ParseQuantity("0")
	memRequest, _ := resource.ParseQuantity("0")
	storRequest, _ := resource.ParseQuantity("0")
	cpuLimit, _ := resource.ParseQuantity("0")
	memLimit, _ := resource.ParseQuantity("0")

	podSpecs := cri.GetPods()
	fmt.Printf("Podspecs count %d\n", len(podSpecs))
	//iterate pods => podspec => iterate containers => resourcerequirements
	for _, pod := range podSpecs {
		fmt.Printf("Tallying podspec %s\n", pod.ObjectMeta.Name)
		for _, container := range pod.Spec.Containers {
			fmt.Printf("Tallying container %s\n", container.Name)
			if container.Resources.Requests != nil {
				val := container.Resources.Requests.Cpu()
				if val != nil {
					cpuRequest.Add(*val)
				}
				val = container.Resources.Requests.Memory()
				if val != nil {
					memRequest.Add(*val)
				}
				sval, err := container.Resources.Requests[v1.ResourceStorage]
				if !err {
					storRequest.Add(sval)
				}
				val = container.Resources.Limits.Cpu()
				if val != nil {
					cpuLimit.Add(*val)
				}
				val = container.Resources.Limits.Memory()
				if val != nil {
					memLimit.Add(*val)
				}
			}
		}
	}

	resources := make(map[v1.ResourceName]*resource.Quantity)
	resources[v1.ResourceRequestsCPU] = &cpuRequest
	resources[v1.ResourceRequestsMemory] = &memRequest
	resources[v1.ResourceRequestsStorage] = &storRequest
	resources[v1.ResourceLimitsCPU] = &cpuLimit
	resources[v1.ResourceLimitsMemory] = &memLimit
	fmt.Println("Resources used")
	fmt.Println(resources)

	return resources
}
