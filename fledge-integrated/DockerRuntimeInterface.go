package main

import (
//"encoding/base64"
/*	"fmt"
	"io"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"golang.org/x/net/context"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	//"reflect"
	"strconv"
	//"strings"
	"time"
	//"time"
	//"k8s.io/apimachinery/pkg/api/resource"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"*/
)

/*type DockerRuntimeInterface struct {
	containerNameIdMapping map[string]string
	podSpecs               map[string]*v1.Pod
	ctx                    context.Context
	cli                    *client.Client
}

func (dri *DockerRuntimeInterface) Init() ContainerRuntimeInterface {
	dri.containerNameIdMapping = make(map[string]string)
	dri.podSpecs = make(map[string]*v1.Pod)
	dri.ctx = context.Background()
	dri.cli, _ = client.NewClientWithOpts(client.WithVersion("1.39"))

	go func() {
		dri.PollLoop()
	}()

	return dri
}

func (dri *DockerRuntimeInterface) PollLoop() {
	for {
		for _, pod := range dri.podSpecs {
			dri.UpdatePodStatus(pod.ObjectMeta.Namespace, pod)
		}

		time.Sleep(3000 * time.Millisecond)
	}
}

func (dri *DockerRuntimeInterface) GetContainerName(namespace string, pod v1.Pod, dc v1.Container) string {
	return namespace + "_" + pod.ObjectMeta.Name + "_" + dc.Name
}

func (dri *DockerRuntimeInterface) GetContainerNameAlt(namespace string, podName string, dcName string) string {
	return namespace + "_" + podName + "_" + dcName
}

func (dri *DockerRuntimeInterface) GetPod(namespace string, name string) (*v1.Pod, bool) {
	pod, found := dri.podSpecs[namespace+"_"+name]
	return pod, found
}

func (dri *DockerRuntimeInterface) GetPods() []*v1.Pod {
	pods := []*v1.Pod{}

	for _, pod := range dri.podSpecs {
		pods = append(pods, pod)
	}

	return pods
}

func (dri *DockerRuntimeInterface) DeployPod(pod *v1.Pod) {
	namespace := pod.ObjectMeta.Namespace

	dri.podSpecs[namespace+"_"+pod.ObjectMeta.Name] = pod

	if config.IgnoreKubeProxy == "true" && strings.HasPrefix(pod.ObjectMeta.Name, "kube-proxy") {
		IgnoreKubeProxy(pod)
		return
	}
	CreateVolumes(pod)

	initContainers := false
	var containers []v1.Container
	if len(pod.Spec.InitContainers) > 0 {
		initContainers = true
		containers = pod.Spec.InitContainers
	} else {
		containers = pod.Spec.Containers
	}
	for _, cont := range containers {
		dri.DeployContainer(namespace, pod, &cont)
	}

	UpdatePostCreationPodStatus(pod, initContainers)
}

func (dri *DockerRuntimeInterface) DeployContainer(namespace string, pod *v1.Pod, dc *v1.Container) (string, error) {
	//nameSuffix := dc.Name
	imageName := dc.Image
	fullName := dri.GetContainerName(namespace, *pod, *dc) //namespace + "_" + pod.ObjectMeta.Name + "_" + dc.Name

	fmt.Printf("Number of env vars %d\n", len(dc.Env))
	envVars := GetEnvAsStringArray(dc)

	expPorts, hostPorts := dri.CreatePorts(dc)
	networkMode := dri.GetNetworkMode(pod)
	restartPolicy := dri.GetRestartPolicy(pod)

	ipcMode := container.IpcMode("")
	if pod.Spec.HostIPC {
		ipcMode = container.IpcMode("host")
	}

	pidMode := container.PidMode("container")
	if pod.Spec.HostPID {
		pidMode = container.PidMode("host")
	}

	privileged := false
	if dc.SecurityContext != nil && dc.SecurityContext.Privileged != nil {
		privileged = bool(*dc.SecurityContext.Privileged)
	}

	contResources := dri.BuildResources(dc)

	mounts, mountNames := dri.BuildMounts(pod, dc)

	fmt.Printf("Creating container %s\n", fullName)

	//do image pull based on imagepullpolicy
	results, err := dri.cli.ImageSearch(dri.ctx, imageName, types.ImageSearchOptions{})
	if (len(results) == 0 && dc.ImagePullPolicy == v1.PullIfNotPresent) || dc.ImagePullPolicy == v1.PullAlways {
		reader, err := dri.cli.ImagePull(dri.ctx, imageName, types.ImagePullOptions{})
		if err != nil {
			panic(err)
		}
		io.Copy(os.Stdout, reader)
	}

	resp, err := dri.cli.ContainerCreate(dri.ctx,
		&container.Config{
			Image:        imageName,
			Env:          envVars,
			Cmd:          dc.Args,
			WorkingDir:   dc.WorkingDir,
			Entrypoint:   dc.Command,
			Labels:       make(map[string]string),
			ExposedPorts: expPorts,
			Volumes:      mountNames,
		},
		&container.HostConfig{
			NetworkMode:   networkMode,
			RestartPolicy: restartPolicy,
			PortBindings:  hostPorts,
			PidMode:       pidMode,
			IpcMode:       ipcMode,
			Privileged:    privileged,
			Resources:     contResources,
			Mounts:        mounts,
		},
		&network.NetworkingConfig{},
		fullName)
	if err != nil {
		fmt.Println("Failed to create container")
		panic(err)
	}

	err = dri.cli.ContainerStart(dri.ctx, resp.ID, types.ContainerStartOptions{})
	if err != nil {
		fmt.Println("Failed to start container")
		panic(err)
	}

	fmt.Println(resp.ID)

	dri.containerNameIdMapping[fullName] = resp.ID
	return resp.ID, nil
}

func (dri *DockerRuntimeInterface) BuildMounts(pod *v1.Pod, dc *v1.Container) ([]mount.Mount, map[string]struct{}) {
	mounts := []mount.Mount{}
	mountNames := make(map[string]struct{})
	for _, cVol := range dc.VolumeMounts {
		cmount := dri.CreateMount(pod, cVol)
		if cmount != nil {
			mounts = append(mounts, *cmount)
			mountNames[cVol.Name] = struct{}{}
		}
	}
	return mounts, mountNames
}

func (dri *DockerRuntimeInterface) BuildResources(dc *v1.Container) container.Resources {
	//some default values
	oneCpu, _ := resource.ParseQuantity("1")
	defaultMem, _ := resource.ParseQuantity("150Mi")

	fmt.Printf("Checking cpu limiting support\n")
	supportCheck, _ := ExecCmdBash("ls /sys/fs/cgroup/cpu/ | grep -E 'cpu.cfs_[a-z]*_us'")
	cpuSupported := supportCheck != ""
	fmt.Printf("Cpu limit support %s\n", cpuSupported)

	contResources := container.Resources{}

	if dc.Resources.Limits == nil {
		dc.Resources.Limits = v1.ResourceList{}
	}
	if dc.Resources.Requests == nil {
		dc.Resources.Requests = v1.ResourceList{}
	}
	memory := dc.Resources.Limits.Memory()
	if memory.IsZero() {
		//if the memory limit isn't filled in, set it to request
		memory = dc.Resources.Requests.Memory()
		dc.Resources.Limits[v1.ResourceMemory] = *memory
	}
	if !memory.IsZero() {
		contResources.Memory = memory.Value()
	} else {
		//this means neither was set, so update both limit and request with a default value
		dc.Resources.Limits[v1.ResourceMemory] = defaultMem
		dc.Resources.Requests[v1.ResourceMemory] = defaultMem
		contResources.Memory = 150 * 1024 * 1024 //150 Mi
	}

	if cpuSupported {
		cpu := dc.Resources.Limits.Cpu()
		if cpu.IsZero() {
			//same if cpu limit isn't filled in
			cpu = dc.Resources.Requests.Cpu()
			dc.Resources.Limits[v1.ResourceCPU] = *cpu
		}
		if !cpu.IsZero() {
			contResources.NanoCPUs = cpu.MilliValue() * 1000000
		} else {
			dc.Resources.Limits[v1.ResourceCPU] = oneCpu
			dc.Resources.Requests[v1.ResourceCPU] = oneCpu
			contResources.NanoCPUs = 1 * 1000 * 1000 * 1000 // 1 CPU
		}
	}

	return contResources
}

func (dri *DockerRuntimeInterface) GetRestartPolicy(pod *v1.Pod) container.RestartPolicy {
	restartPolicy := container.RestartPolicy{
		Name: "no",
	}
	if pod.Spec.RestartPolicy == v1.RestartPolicyAlways {
		restartPolicy.Name = "always"
	} else if pod.Spec.RestartPolicy == v1.RestartPolicyOnFailure {
		restartPolicy.Name = "on-failure"
	}
	return restartPolicy
}

func (dri *DockerRuntimeInterface) GetNetworkMode(pod *v1.Pod) container.NetworkMode {
	networkMode := container.NetworkMode("default")
	if pod.Spec.HostNetwork {
		networkMode = container.NetworkMode("host")
	}
	return networkMode
}

func (dri *DockerRuntimeInterface) CreatePorts(dc *v1.Container) (nat.PortSet, nat.PortMap) {
	expPorts := nat.PortSet{}
	hostPorts := nat.PortMap{}
	for _, prt := range dc.Ports {
		newPort, _ := nat.NewPort(string(prt.Protocol), strconv.Itoa(int(prt.ContainerPort)))

		expPorts[newPort] = struct{}{}
		hostPorts[newPort] = []nat.PortBinding{{HostIP: prt.HostIP, HostPort: strconv.Itoa(int(prt.HostPort))}}
	}
	return expPorts, hostPorts
}

func (dri *DockerRuntimeInterface) CreateMount(pod *v1.Pod, volMount v1.VolumeMount) *mount.Mount {
	//fmt.Printf("Creating mount for volumemount %s\n", volMount.Name)
	vName := volMount.Name
	var volume v1.Volume

	for _, vol := range pod.Spec.Volumes {
		if vol.Name == vName {
			volume = vol
		}
	}
	//fmt.Printf("Matching volume %s\n", volume.Name)

	//the vkubelet can handle hostpath volumes, secret volumes and configmap volumes
	hostPath := GetHostMountPath(pod, volume)

	if hostPath == nil {
		fmt.Printf("No hostpath found, mount not supported?\n")
		return nil
	}
	//fmt.Printf("Attempting to mount hostpath %s\n", hostPath)

	propagation := mount.PropagationShared
	if volMount.MountPropagation != nil {
		if *volMount.MountPropagation == v1.MountPropagationHostToContainer {
			propagation = mount.PropagationSlave
		} else if *volMount.MountPropagation == v1.MountPropagationNone {
			propagation = mount.PropagationPrivate
		}
	}

	cMount := mount.Mount{
		Source:   *hostPath + volMount.SubPath,
		Target:   volMount.MountPath,
		ReadOnly: volMount.ReadOnly,
		Type:     mount.TypeBind,
	}
	fmt.Printf("Mount source %s target %s propagation %s\n", cMount.Source, cMount.Target, propagation)
	cMount.BindOptions = &mount.BindOptions{
		Propagation: propagation,
	}

	return &cMount

}

func (dri *DockerRuntimeInterface) UpdatePod(pod *v1.Pod) {
	containers := pod.Spec.Containers
	namespace := pod.ObjectMeta.Namespace

	dri.podSpecs[namespace+"_"+pod.ObjectMeta.Name] = pod

	for _, cont := range containers {
		dri.UpdateContainer(namespace, pod, &cont)
	}
}

func (dri *DockerRuntimeInterface) UpdateContainer(namespace string, pod *v1.Pod, dc *v1.Container) {
	dri.StopContainer(namespace, pod, dc)
	dri.DeployContainer(namespace, pod, dc)
}

func (dri *DockerRuntimeInterface) DeletePod(pod *v1.Pod) {
	containers := pod.Spec.Containers
	namespace := pod.ObjectMeta.Namespace

	delete(dri.podSpecs, namespace+"_"+pod.ObjectMeta.Name)

	for _, cont := range containers {
		dri.StopContainer(namespace, pod, &cont)
	}
	RemoveNetNamespace(pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)

	FreeIP(pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
}

func (dri *DockerRuntimeInterface) StopContainer(namespace string, pod *v1.Pod, dc *v1.Container) bool {
	fullName := dri.GetContainerName(namespace, *pod, *dc) //namespace + "_" + pod.ObjectMeta.Name + "_" + dc.Name
	fmt.Printf("Stopping container %s\n", fullName)

	contID, found := dri.containerNameIdMapping[fullName]
	if found {
		fmt.Printf("Stopping container id %s\n", contID)
		//time, _ := time.ParseDuration("10s")
		err := dri.cli.ContainerStop(dri.ctx, contID, nil)

		if err == nil {
			delete(dri.containerNameIdMapping, fullName)
			fmt.Printf("Removing container %s\n", contID)
			err = dri.cli.ContainerRemove(dri.ctx, contID, types.ContainerRemoveOptions{
				Force: true,
			})
		} else {
			fmt.Println(err.Error())
		}
		return err == nil
	}
	return true
}

func (dri *DockerRuntimeInterface) UpdatePodStatus(namespace string, pod *v1.Pod) {
	pod.Status.HostIP = config.DeviceIP
	fmt.Printf("Update pod status %s\n", pod.ObjectMeta.Name)
	latestStatus := GetHighestPodStatus(pod)
	if latestStatus == nil {
		fmt.Println("No correct status found, returning")
		return
	}
	fmt.Printf("Pod %s status %s\n", pod.ObjectMeta.Name, latestStatus.Type)
	switch latestStatus.Type {
	case v1.PodReady:
		fmt.Println("Pod ready, just updating")
		//everything good, just update statuses
		//check pod status phases running or succeeded
		dri.UpdateContainerStatuses(namespace, pod, *latestStatus)
		dri.SetupPodIPs(pod)
	case v1.PodInitialized:
		if latestStatus.Status == v1.ConditionTrue {
			fmt.Println("Pod initialized, just updating and upgrading to ready if possible")
			//update statuses, check for PodReady, check for phase running
			dri.UpdateContainerStatuses(namespace, pod, *latestStatus)
		} else {
			fmt.Println("Pod initialized, checking init containers")
			//check init containers
			dri.CheckInitContainers(namespace, pod)
		}
	case v1.PodReasonUnschedulable:
		fmt.Println("Pod unschedulable, ignoring")
		//don't do anything really
	case v1.PodScheduled:
		fmt.Println("Pod Scheduled, ignoring")
		//don't do anything either
	}
}

func (dri *DockerRuntimeInterface) SetupPodIPs(pod *v1.Pod) {
	pod.Status.HostIP = config.DeviceIP
	if pod.Status.PodIP == "" {
		if pod.Spec.HostNetwork {
			pod.Status.PodIP = config.DeviceIP
		} else {
			container := dri.GetContainerName(pod.ObjectMeta.Namespace, *pod, pod.Spec.Containers[0])
			contJSON, _ := dri.cli.ContainerInspect(dri.ctx, dri.containerNameIdMapping[container])
			pod.Status.PodIP = BindNetNamespace(pod.ObjectMeta.Namespace, pod.ObjectMeta.Name, contJSON.State.Pid)
		}
	}
}

func (dri *DockerRuntimeInterface) CheckInitContainers(namespace string, pod *v1.Pod) {
	allContainersDone := true
	noErrors := true

	containerStatuses := []v1.ContainerStatus{}
	for _, cont := range pod.Spec.Containers {
		fullName := dri.GetContainerNameAlt(namespace, pod.ObjectMeta.Name, cont.Name)
		contID, found := dri.containerNameIdMapping[fullName]
		if found {
			state := v1.ContainerState{}

			contJSON, _ := dri.cli.ContainerInspect(dri.ctx, contID)
			switch contJSON.State.Status {
			case "created":
				allContainersDone = false
				state.Waiting = &v1.ContainerStateWaiting{
					Reason:  "Starting",
					Message: "Starting container",
				}
			case "running":
				fallthrough
			case "paused":
				fallthrough
			case "restarting":
				//startTime, _ := time.Parse(time.RFC3339, contJson.State.StartedAt)
				allContainersDone = false
				state.Running = &v1.ContainerStateRunning{ //add real time later
					StartedAt: metav1.Now(),
				}
			case "removing":
				fallthrough
			case "exited":
				fallthrough
			case "dead":
				if contJSON.State.ExitCode > 0 {
					noErrors = false
				}
				state.Terminated = &v1.ContainerStateTerminated{
					Reason:      "Stopped",
					Message:     "Container stopped",
					FinishedAt:  metav1.Now(), //add real time later
					ContainerID: contID,
				}
			}

			status := v1.ContainerStatus{
				Name:         cont.Name,
				State:        state,
				Ready:        false,
				RestartCount: 0,
				Image:        cont.Image,
				ImageID:      "",
				ContainerID:  contID,
			}

			containerStatuses = []v1.ContainerStatus{status}
		}
	}
	pod.Status.ContainerStatuses = containerStatuses

	if noErrors && allContainersDone {
		//start actual containers
		for _, container := range pod.Spec.Containers {
			(*dri).DeployContainer(pod.ObjectMeta.Namespace, pod, &container)
		}
	}
	UpdateInitPodStatus(pod, noErrors, allContainersDone)
}

func (dri *DockerRuntimeInterface) UpdateContainerStatuses(namespace string, pod *v1.Pod, podStatus v1.PodCondition) {
	allContainersRunning := true
	allContainersDone := true
	noErrors := true

	containerStatuses := []v1.ContainerStatus{}
	for _, cont := range pod.Spec.Containers {
		fullName := dri.GetContainerNameAlt(namespace, pod.ObjectMeta.Name, cont.Name)
		contID, found := dri.containerNameIdMapping[fullName]
		if found {
			state := v1.ContainerState{}

			contJSON, _ := dri.cli.ContainerInspect(dri.ctx, contID)
			switch contJSON.State.Status {
			case "created":
				allContainersRunning = false
				allContainersDone = false
				state.Waiting = &v1.ContainerStateWaiting{
					Reason:  "Starting",
					Message: "Starting container",
				}
			case "running":
				fallthrough
			case "paused":
				fallthrough
			case "restarting":
				//startTime, _ := time.Parse(time.RFC3339, contJson.State.StartedAt)
				allContainersDone = false
				state.Running = &v1.ContainerStateRunning{ //add real time later
					StartedAt: metav1.Now(),
				}
			case "removing":
				fallthrough
			case "exited":
				fallthrough
			case "dead":
				if contJSON.State.ExitCode > 0 {
					noErrors = false
				}
				state.Terminated = &v1.ContainerStateTerminated{
					Reason:      "Stopped",
					Message:     "Container stopped",
					FinishedAt:  metav1.Now(), //add real time later
					ContainerID: contID,
				}
			}

			status := v1.ContainerStatus{
				Name:         cont.Name,
				State:        state,
				Ready:        false,
				RestartCount: 0,
				Image:        cont.Image,
				ImageID:      "",
				ContainerID:  contID,
			}

			containerStatuses = []v1.ContainerStatus{status}
		}
	}
	UpdatePodStatus(podStatus, containerStatuses, pod, noErrors, allContainersRunning, allContainersDone)
}

func (dri *DockerRuntimeInterface) FetchContainerLogs(namespace string, podName string, containerName string, tail string, timestamps bool) *io.ReadCloser {

	fullName := dri.GetContainerNameAlt(namespace, podName, containerName) //namespace + "_" + pod.ObjectMeta.Name + "_" + dc.Name

	contID, found := dri.containerNameIdMapping[fullName]
	if found {
		opts := types.ContainerLogsOptions{
			Timestamps: timestamps,
			Tail:       tail,
		}
		reader, err := dri.cli.ContainerLogs(dri.ctx, contID, opts)

		if err != nil {
			return &reader
		}
	}
	return nil
}

func (dri *DockerRuntimeInterface) ShutdownPods() {
	for _, pod := range dri.podSpecs {
		dri.DeletePod(pod)
	}
}
*/
