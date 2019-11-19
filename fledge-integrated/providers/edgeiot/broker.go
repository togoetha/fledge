package edgeiot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/remotecommand"
	stats "k8s.io/kubernetes/pkg/kubelet/apis/stats/v1alpha1"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"fledge/fledge-integrated/config"
	"fledge/fledge-integrated/manager"
	"fledge/fledge-integrated/vkube"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var reInsideWhtsp = regexp.MustCompile(`\s+`)

type BrokerProvider struct {
	nodeName        string
	operatingSystem string
	//endpoint            *url.URL
	//client              *http.Client
	daemonEndpointPort  int32
	lastMemoryPressure  bool
	lastStoragePressure bool
	lastStorageFull     bool
}

func NewBrokerProvider(nodeName, operatingSystem string, daemonEndpointPort int32) (*BrokerProvider, error) {
	var provider BrokerProvider

	provider.nodeName = nodeName
	provider.operatingSystem = operatingSystem
	//provider.client = &http.Client{}
	provider.daemonEndpointPort = daemonEndpointPort

	/*if ep := os.Getenv("WEB_ENDPOINT_URL"); ep != "" {
		epurl, err := url.Parse(ep)
		if err != nil {
			return nil, err
		}
		provider.endpoint = epurl
	}*/
	//force a node update first time
	provider.lastMemoryPressure = true
	provider.lastStorageFull = true
	provider.lastStoragePressure = true

	return &provider, nil
}

func (p *BrokerProvider) NodeChanged() bool {
	aChanged := p.AddressesChanged()
	pChanged := p.ConditionsChanged()

	fmt.Printf("NodeChanged check: addresses changed %t conditions changed %t\n", aChanged, pChanged)

	return aChanged || pChanged
}

func (p *BrokerProvider) PodsChanged() bool {
	pChanged := vkube.Cri.PodsChanged()
	fmt.Printf("PodsChanged check: %t", pChanged)
	return pChanged
}

func (p *BrokerProvider) ResetChanges() {
	vkube.Cri.ResetFlags()
}

// CreatePod accepts a Pod definition and forwards the call to the web endpoint
func (p *BrokerProvider) CreatePod(ctx context.Context, pod *v1.Pod) error {
	fmt.Println("CreatePod")
	//pod := GetPodFromRequest(r)

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

	vkube.Cri.DeployPod(pod)
	return nil
}

// UpdatePod accepts a Pod definition and forwards the call to the web endpoint
func (p *BrokerProvider) UpdatePod(ctx context.Context, pod *v1.Pod) error {
	name := pod.ObjectMeta.Name
	namespace := pod.ObjectMeta.Namespace

	fmt.Printf("Updating pod namespace %s name %s\n", namespace, name)

	vkube.Cri.UpdatePod(pod)
	return nil
}

// DeletePod accepts a Pod definition and forwards the call to the web endpoint
func (p *BrokerProvider) DeletePod(ctx context.Context, pod *v1.Pod) error {
	name := pod.ObjectMeta.Name
	namespace := pod.ObjectMeta.Namespace

	fmt.Printf("Deleting pod namespace %s name %s\n", namespace, name)

	vkube.Cri.DeletePod(pod)
	return nil
}

// GetPod returns a pod by name that is being managed by the web server
func (p *BrokerProvider) GetPod(ctx context.Context, namespace, name string) (*v1.Pod, error) {
	pod, found := vkube.Cri.GetPod(namespace, name)

	if found {
		return pod, nil
	} else {
		//TODO FIX THIS BASED ON WHAT THE WEB PROVIDER DID
		return nil, nil
	}
}

var totalNanoCores uint64

// GetStatsSummary returns a stats summary of the virtual node
func (p *BrokerProvider) GetStatsSummary(context.Context) (*stats.Summary, error) {
	//we can delete the other service perhaps?
	nodenameStr, _ := manager.ExecCmdBash("hostname")
	nodename := strings.TrimSuffix(nodenameStr, "\n")

	//CPU STUFF, REFACTOR TO METHOD
	cpuStatsStr, _ := manager.ExecCmdBash("mpstat 1 1 | grep 'all'")

	nProc, _ := manager.ExecCmdBash("nproc")
	numCpus, _ := strconv.Atoi(strings.Trim(nProc, "\n"))

	cpuStatsLines := strings.Split(cpuStatsStr, "\n")
	//cpuStatsStr = strings.TrimSuffix(cpuStatsStr, "\n")
	cpuCats := strings.Split(reInsideWhtsp.ReplaceAllString(cpuStatsLines[0], " "), " ")
	cpuIdle, _ := strconv.ParseFloat(cpuCats[len(cpuCats)-1], 64)

	cpuNanos := uint64((100-cpuIdle)*10000000) * uint64(numCpus) //pct is already 10^2, so * 10^7, then * cores.

	//TODO: take time into account here (cpuNanos * seconds passed since last check)
	totalNanoCores += cpuNanos

	cpuStats := stats.CPUStats{
		Time:                 metav1.Now(),
		UsageNanoCores:       &cpuNanos,
		UsageCoreNanoSeconds: &totalNanoCores,
	}

	//MEM STUFF, REFACTOR TO METHOD
	memStatsStr, _ := manager.ExecCmdBash("free | grep 'Mem:'")
	cats := strings.Split(reInsideWhtsp.ReplaceAllString(memStatsStr, " "), " ")
	memFree, _ := strconv.ParseUint(cats[6], 10, 64)
	memSize, _ := strconv.ParseUint(cats[1], 10, 64)

	memStatsStr, _ = manager.ExecCmdBash("free | grep '+'")
	//bailout for older free versions, in which case this is more accurate for "available" memory
	if memStatsStr != "" {
		cats := strings.Split(reInsideWhtsp.ReplaceAllString(memStatsStr, " "), " ")
		memFree, _ = strconv.ParseUint(cats[2], 10, 64)
	}

	memUsed := memSize - memFree

	memStats := stats.MemoryStats{
		Time:            metav1.Now(),
		UsageBytes:      &memUsed,
		AvailableBytes:  &memFree,
		WorkingSetBytes: &memUsed,
	}

	//NETWORK STUFF, REFACTOR TO METHOD

	//ifnames: / # ip a | grep -o -E '[0-9]: [a-z0-9]*: '

	ifacesStr, _ := manager.ExecCmdBash("ip a | grep -o -E '[0-9]{1,2}: [a-z0-9]*: ' | grep -o -E '[a-z0-9]{2,}'")
	ifaces := strings.Split(ifacesStr, "\n")

	//ifstats: ifconfig enp1s0f0 | grep 'bytes'
	//      RX bytes:726654708 (692.9 MiB)  TX bytes:456250038 (435.1 MiB)

	ifacesStats := []stats.InterfaceStats{}
	for _, iface := range ifaces {
		ifaceStatsStr, _ := manager.ExecCmdBash("ifconfig " + iface + "| grep 'bytes'")
		fmt.Println(ifaceStatsStr)
		//TODO from here on
	}

	netStats := stats.NetworkStats{
		Time:       metav1.Now(),
		Interfaces: ifacesStats,
	}

	nodeStats := stats.NodeStats{
		NodeName:  nodename,
		StartTime: metav1.NewTime(vkube.StartTime),
		CPU:       &cpuStats,
		Memory:    &memStats,
		Network:   &netStats,
		//Fs: ,
		//Runtime: ,
		//Rlimit: ,
	}

	summary := stats.Summary{
		Node: nodeStats,
	}
	return &summary, nil
}

// GetContainerLogs returns the logs of a container running in a pod by name.
func (p *BrokerProvider) GetContainerLogs(ctx context.Context, namespace, podName, containerName string, tail int) (string, error) {
	reader := vkube.Cri.FetchContainerLogs(namespace, podName, containerName, strconv.Itoa(tail), true)
	if reader != nil {
		buf := new(bytes.Buffer)
		buf.ReadFrom(*reader)
		logs := buf.String()
		//fmt.Fprintf(w, "%s", logs)
		return logs, nil
	} else {
		return "", nil
	}
}

// Get full pod name as defined in the provider context
// TODO: Implementation
func (p *BrokerProvider) GetPodFullName(namespace string, pod string) string {
	//TODO check how web provider did it
	return ""
}

// ExecInContainer executes a command in a container in the pod, copying data
// between in/out/err and the container's stdin/stdout/stderr.
// TODO: Implementation
func (p *BrokerProvider) ExecInContainer(name string, uid types.UID, container string, cmd []string, in io.Reader, out, err io.WriteCloser, tty bool, resize <-chan remotecommand.TerminalSize, timeout time.Duration) error {
	log.Printf("receive ExecInContainer %q\n", container)
	return nil
}

// GetPodStatus retrieves the status of a given pod by name.
func (p *BrokerProvider) GetPodStatus(ctx context.Context, namespace, name string) (*v1.PodStatus, error) {
	pod, found := vkube.Cri.GetPod(namespace, name)

	if found {
		return &pod.Status, nil
	} else {
		//TODO check how web did it
		return nil, nil
	}
}

// GetPods retrieves a list of all pods scheduled to run.
func (p *BrokerProvider) GetPods(ctx context.Context) ([]*v1.Pod, error) {
	podSpecs := vkube.Cri.GetPods()
	//again, might wanna update some values first tho
	pods := []*v1.Pod{}
	for _, value := range podSpecs {
		//cri.UpdatePodStatus(value.ObjectMeta.Namespace, value)
		pods = append(pods, value)
	}
	return pods, nil
	//return nil, nil
}

// Capacity returns a resource list containing the capacity limits
func (p *BrokerProvider) Capacity(ctx context.Context) v1.ResourceList {
	resources := v1.ResourceList{} //make(map[v1.ResourceName]string)
	cpu, _ := resource.ParseQuantity(manager.CpuCores())
	resources[v1.ResourceCPU] = cpu
	mem, _ := resource.ParseQuantity(manager.TotalMemory() + "Mi")
	resources[v1.ResourceMemory] = mem
	stor, _ := resource.ParseQuantity(manager.TotalStorage() + "i")
	resources[v1.ResourceStorage] = stor
	pods, _ := resource.ParseQuantity("2")
	resources[v1.ResourcePods] = pods

	contResources := GetContainerResources()

	resources[v1.ResourceRequestsCPU] = *contResources[v1.ResourceRequestsCPU]
	resources[v1.ResourceRequestsMemory] = *contResources[v1.ResourceRequestsMemory]
	resources[v1.ResourceRequestsStorage] = *contResources[v1.ResourceRequestsStorage]
	resources[v1.ResourceLimitsCPU] = *contResources[v1.ResourceLimitsCPU]
	resources[v1.ResourceLimitsMemory] = *contResources[v1.ResourceLimitsCPU]

	if manager.HasOpenCLCaps() {
		q, _ := resource.ParseQuantity("1")
		resources["device/openclgpu"] = q
	}
	if manager.HasCudaCaps() {
		q, _ := resource.ParseQuantity("1")
		resources["device/cudagpu"] = q
	}

	return resources
	//return nil
}

// NodeConditions returns a list of conditions (Ready, OutOfDisk, etc), for updates to the node status
func (p *BrokerProvider) NodeConditions(ctx context.Context) []v1.NodeCondition {
	conditionReady := v1.NodeCondition{Type: v1.NodeReady, Status: v1.ConditionTrue, LastHeartbeatTime: metav1.Now(), LastTransitionTime: metav1.Now(), Reason: "Started", Message: "Rocket ranger, ready to rock it"}

	var memPressure v1.ConditionStatus
	p.lastMemoryPressure = manager.IsMemoryPressure()
	if p.lastMemoryPressure {
		memPressure = v1.ConditionTrue
	} else {
		memPressure = v1.ConditionFalse
	}
	conditionMemPressure := v1.NodeCondition{Type: v1.NodeMemoryPressure, Status: memPressure, LastHeartbeatTime: metav1.Now(), LastTransitionTime: metav1.Now(), Reason: "Memory pressure", Message: "We're giving 'er all she's got captain"}

	var storagePressure v1.ConditionStatus
	p.lastStoragePressure = manager.IsStoragePressure()
	if p.lastStoragePressure {
		storagePressure = v1.ConditionTrue
	} else {
		storagePressure = v1.ConditionFalse
	}
	conditionStoragePressure := v1.NodeCondition{Type: v1.NodeDiskPressure, Status: storagePressure, LastHeartbeatTime: metav1.Now(), LastTransitionTime: metav1.Now(), Reason: "Storage pressure", Message: "She won't take it much longer"}

	var storageFull v1.ConditionStatus
	p.lastStorageFull = manager.IsStorageFull()
	if p.lastStorageFull {
		storageFull = v1.ConditionTrue
	} else {
		storageFull = v1.ConditionFalse
	}
	conditionStorageFull := v1.NodeCondition{Type: v1.NodeOutOfDisk, Status: storageFull, LastHeartbeatTime: metav1.Now(), LastTransitionTime: metav1.Now(), Reason: "Storage full", Message: "He's dead Jim"}

	//add more conditions later, with info from cmd
	conditions := []v1.NodeCondition{conditionReady, conditionMemPressure, conditionStoragePressure, conditionStorageFull}
	return conditions
}

func (p *BrokerProvider) ConditionsChanged() bool {
	if manager.IsMemoryPressure() != p.lastMemoryPressure {
		return true
	}
	if manager.IsStoragePressure() != p.lastStoragePressure {
		return true
	}
	if manager.IsStorageFull() != p.lastStorageFull {
		return true
	}
	return false
}

// NodeAddresses returns a list of addresses for the node status
// within Kubernetes.
func (p *BrokerProvider) NodeAddresses(ctx context.Context) []v1.NodeAddress {
	nodenameStr, _ := manager.ExecCmdBash("hostname")
	p.nodeName = strings.TrimSuffix(nodenameStr, "\n")
	addresshost := v1.NodeAddress{Type: v1.NodeHostName, Address: p.nodeName}
	addressip := v1.NodeAddress{Type: v1.NodeInternalIP, Address: config.Cfg.DeviceIP}
	addresses := []v1.NodeAddress{addresshost, addressip}
	return addresses
}

func (p *BrokerProvider) AddressesChanged() bool {
	nodenameStr, _ := manager.ExecCmdBash("hostname")
	nodename := strings.TrimSuffix(nodenameStr, "\n")
	return p.nodeName != nodename
}

// NodeDaemonEndpoints returns NodeDaemonEndpoints for the node status
// within Kubernetes.
func (p *BrokerProvider) NodeDaemonEndpoints(ctx context.Context) *v1.NodeDaemonEndpoints {
	return &v1.NodeDaemonEndpoints{
		KubeletEndpoint: v1.DaemonEndpoint{
			Port: p.daemonEndpointPort,
		},
	}
}

// OperatingSystem returns the operating system for this provider.
func (p *BrokerProvider) OperatingSystem() string {
	return p.operatingSystem
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

	podSpecs := vkube.Cri.GetPods()
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
