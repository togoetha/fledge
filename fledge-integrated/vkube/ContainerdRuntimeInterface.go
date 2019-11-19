package vkube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/containerd/containerd"
	"github.com/containerd/containerd/cio"
	"github.com/containerd/containerd/mount"

	"io"
	"strings"

	"fledge/fledge-integrated/config"
	"fledge/fledge-integrated/manager"
	"github.com/containerd/containerd/contrib/nvidia"
	"github.com/containerd/containerd/namespaces"
	"github.com/containerd/containerd/oci"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PodContainer struct {
	podName   string
	container containerd.Container
	task      containerd.Task
}

type ContainerdRuntimeInterface struct {
	client                   *containerd.Client
	containerNameTaskMapping map[string]PodContainer
	podSpecs                 map[string]*v1.Pod
	ctx                      context.Context
	podsChanged              bool
}

func (cdri *ContainerdRuntimeInterface) PodsChanged() bool {
	return cdri.podsChanged
}

func (cdri *ContainerdRuntimeInterface) ResetFlags() {
	fmt.Println("Setting podsChanged false")
	cdri.podsChanged = false
}

func (cdri *ContainerdRuntimeInterface) Init() ContainerRuntimeInterface {
	//log.GetLogger(cdri.ctx).Logger.Level = logrus.DebugLevel

	cdri.ctx = namespaces.WithNamespace(context.Background(), "default")

	cdri.podSpecs = make(map[string]*v1.Pod)
	cdri.containerNameTaskMapping = make(map[string]PodContainer)
	cdri.client, _ = containerd.New("/run/containerd/containerd.sock")
	if cdri.client == nil {
		fmt.Println("Failed to create containerd client!")
	}

	mount.SetTempMountLocation("/ctdtmp")

	go func() {
		cdri.PollLoop()
	}()

	return cdri
}

func (dri *ContainerdRuntimeInterface) PollLoop() {
	for {
		for _, pod := range dri.podSpecs {
			dri.UpdatePodStatus(pod.ObjectMeta.Namespace, pod)
		}

		time.Sleep(3000 * time.Millisecond)
	}
}

func (cdri *ContainerdRuntimeInterface) GetPod(namespace string, name string) (*v1.Pod, bool) {
	pod, found := cdri.podSpecs[namespace+"_"+name]
	return pod, found
}

func (cdri *ContainerdRuntimeInterface) GetPods() []*v1.Pod {
	pods := []*v1.Pod{}

	for _, pod := range cdri.podSpecs {
		pods = append(pods, pod)
	}

	return pods
}

func (dri *ContainerdRuntimeInterface) GetContainerName(namespace string, pod v1.Pod, dc v1.Container) string {
	return namespace + "_" + pod.ObjectMeta.Name + "_" + dc.Name
}

func (dri *ContainerdRuntimeInterface) GetContainerNameAlt(namespace string, podName string, dcName string) string {
	return namespace + "_" + podName + "_" + dcName
}

func (dri *ContainerdRuntimeInterface) DeployPod(pod *v1.Pod) {
	namespace := pod.ObjectMeta.Namespace

	dri.podSpecs[namespace+"_"+pod.ObjectMeta.Name] = pod

	if config.Cfg.IgnoreKubeProxy == "true" && strings.HasPrefix(pod.ObjectMeta.Name, "kube-proxy") {
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
		_, err := dri.DeployContainer(namespace, pod, &cont)
		if err != nil {
			delete(dri.podSpecs, namespace+"_"+pod.ObjectMeta.Name)
			panic(err)
		}
	}

	UpdatePostCreationPodStatus(pod, initContainers)
	fmt.Println("Setting podsChanged true")
	dri.podsChanged = true
}

func ValidPrefix(tagPrefix string) bool {
	switch tagPrefix {
	case "docker.io":
		fallthrough
	case "k8s.gcr.io":
		return true
	default:
		return false
	}
	return false
}

func (dri *ContainerdRuntimeInterface) CheckFullTag(imageName string) string {
	urlParts := strings.Split(imageName, "/")
	if !ValidPrefix(urlParts[0]) {
		imageName = fmt.Sprintf("docker.io/%s", imageName)
	}

	parts := strings.Split(imageName, ":")
	if len(parts) == 1 {
		return fmt.Sprintf("%s:latest", imageName)
	} else {
		return imageName
	}
}

func (dri *ContainerdRuntimeInterface) SetupPorts(pod *v1.Pod, dc *v1.Container) {
	//TODO!
}

func (dri *ContainerdRuntimeInterface) CleanupPorts(pod *v1.Pod, dc *v1.Container) {

}

func (dri *ContainerdRuntimeInterface) DeployContainer(namespace string, pod *v1.Pod, dc *v1.Container) (string, error) {
	imageName := dc.Image
	fullName := dri.GetContainerName(namespace, *pod, *dc)

	imageName = dri.CheckFullTag(imageName)

	envVars := GetEnvAsStringArray(dc)

	//restart policy won't be set here, instead tasks have to be monitored for their status
	//and restarted if failed (and pod/node is not shutting down). TODO

	//determine ipc and pid mode
	var ipcMode *oci.SpecOpts = nil
	if pod.Spec.HostIPC {
		opt := oci.WithHostNamespace(specs.IPCNamespace)
		ipcMode = &opt
	}

	var pidMode *oci.SpecOpts = nil
	if pod.Spec.HostPID {
		opt := oci.WithHostNamespace(specs.PIDNamespace)
		pidMode = &opt
	}

	//handle privileged containers
	privileged := false
	if dc.SecurityContext != nil && dc.SecurityContext.Privileged != nil {
		privileged = bool(*dc.SecurityContext.Privileged)
	}

	//handle volume mounts
	vmounts := dri.BuildMounts(pod, dc)

	//handle resource limits
	cgroup := dri.SetContainerResources(pod.ObjectMeta.Namespace, pod.ObjectMeta.Name, dc)

	//pull image + policy
	var image containerd.Image
	image, err := dri.client.GetImage(dri.ctx, imageName)
	if (err != nil && dc.ImagePullPolicy == v1.PullIfNotPresent) || dc.ImagePullPolicy == v1.PullAlways {
		if dc.ImagePullPolicy == v1.PullAlways {
			dri.client.ImageService().Delete(dri.ctx, imageName)
		}
		image, err = dri.client.Pull(dri.ctx, imageName, containerd.WithPullUnpack)
		if err != nil {
			fmt.Printf("Pull failed for image %s\n", imageName)
			fmt.Println(err.Error())
			return "", err
		}
	} else {
		fmt.Println(err.Error())
		return "", err
	}

	fmt.Printf("Image exists or successfully pulled: %s\n", image.Name())

	//generate container id + snapshot
	snapshot := fmt.Sprintf("%s-snapshot", fullName)

	args := dc.Command
	args = append(args, dc.Args...)

	//tally all spec options
	specOpts := []oci.SpecOpts{
		oci.WithImageConfig(image),
		oci.WithEnv(envVars),
		oci.WithCgroup(cgroup),
		oci.WithMounts(vmounts),
		//netSpecOpts,
	}

	//find out if a gpu should be assigned, and if we have the right type for the container
	assignGpu, err := CheckGpuResourceRequired(dc)
	if err != nil {
		fmt.Println(err.Error())
		return "", err
	}
	if assignGpu {
		//to be fair, it's not guaranteed to be nvidia, should really check for AMD or other devices instead of just cuda/opencv
		//but then again, it's not like they have a container hook, so we should really just detect and advertise nvidia?
		specOpts = append(specOpts, nvidia.WithGPUs(nvidia.WithDevices(1), nvidia.WithAllCapabilities))
	}

	if len(args) > 0 {
		specOpts = append(specOpts, oci.WithProcessArgs(args...))
	}
	if dc.WorkingDir != "" {
		specOpts = append(specOpts, oci.WithProcessCwd(dc.WorkingDir))
	}
	if ipcMode != nil {
		specOpts = append(specOpts, *ipcMode)
	}
	if pidMode != nil {
		specOpts = append(specOpts, *pidMode)
	}
	//NET: trying something out
	if pod.Spec.HostNetwork {
		specOpts = append(specOpts, oci.WithHostNamespace(specs.NetworkNamespace))
	}
	//assign all caps
	if privileged {
		specOpts = append(specOpts, oci.WithPrivileged)
	}

	fmt.Printf("Creating container with snapshotter native\n")
	container, err := dri.client.NewContainer(
		dri.ctx,
		fullName,
		//containerd.WithSnapshotter("native"),
		containerd.WithImage(image),
		containerd.WithNewSnapshot(snapshot, image),
		containerd.WithNewSpec(specOpts...),
	)
	if err != nil {
		fmt.Println(err.Error())
	}

	fmt.Printf("Successfully created container with ID %s and snapshot with ID %s\n", container.ID(), snapshot)

	// create a task from the container
	task, err := container.NewTask(dri.ctx, cio.NewCreator(cio.WithStdio))
	if err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println("Task created")
	//defer task.Delete(ctx)

	// make sure we wait before calling start
	exitStatusC, err := task.Wait(dri.ctx)
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("Task awaited with status %d", exitStatusC)

	// call start on the task to execute the redis server
	if err := task.Start(dri.ctx); err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println("Task started")

	//NET: trying to fix the net namespace here
	dri.SetupPodIPs(pod, task)

	dri.containerNameTaskMapping[fullName] = PodContainer{
		podName:   pod.ObjectMeta.Name,
		container: container,
		task:      task,
	}

	return task.ID(), nil
}

func CheckGpuResourceRequired(dc *v1.Container) (bool, error) {
	if dc.Resources.Limits == nil {
		dc.Resources.Limits = v1.ResourceList{}
	}
	if dc.Resources.Requests == nil {
		dc.Resources.Requests = v1.ResourceList{}
	}

	cudaLimit := dc.Resources.Limits["device/cudagpu"]
	cudaRequest := dc.Resources.Requests["device/cudagpu"]
	opencvLimit := dc.Resources.Limits["device/opencvgpu"]
	opencvRequest := dc.Resources.Requests["device/opencvgpu"]

	if opencvLimit.Value() > 0 || opencvRequest.Value() > 0 {
		if !manager.HasOpenCLCaps() {
			return false, errors.New("OpenCV requested but not present")
		} else {
			return true, nil
		}
	}
	if cudaLimit.Value() > 0 || cudaRequest.Value() > 0 {
		if !manager.HasCudaCaps() {
			return false, errors.New("CUDA requested but not present")
		} else {
			return true, nil
		}
	}
	return false, nil
}

func (dri *ContainerdRuntimeInterface) SetupPodIPs(pod *v1.Pod, task containerd.Task) {
	pod.Status.HostIP = config.Cfg.DeviceIP
	if pod.Status.PodIP == "" {
		if pod.Spec.HostNetwork {
			pod.Status.PodIP = config.Cfg.DeviceIP
		} else {
			pids, _ := task.Pids(dri.ctx)
			pidJson, _ := json.Marshal(pids)
			fmt.Printf("Container pids %s", string(pidJson))
			pod.Status.PodIP = BindNetNamespace(pod.ObjectMeta.Namespace, pod.ObjectMeta.Name, int(pids[0].Pid))
		}
	}
}

func (dri *ContainerdRuntimeInterface) BuildMounts(pod *v1.Pod, dc *v1.Container) []specs.Mount {
	mounts := []specs.Mount{}
	//mountNames := make(map[string]struct{})
	for _, cVol := range dc.VolumeMounts {
		cmount := dri.CreateMount(pod, cVol)
		if cmount != nil {
			mounts = append(mounts, *cmount)
			//mountNames[cVol.Name] = struct{}{}
		}
	}
	return mounts
}

func (dri *ContainerdRuntimeInterface) CreateMount(pod *v1.Pod, volMount v1.VolumeMount) *specs.Mount {
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

	mntOpts := []string{}

	if volMount.ReadOnly {
		mntOpts = append(mntOpts, "rbind")
		mntOpts = append(mntOpts, "ro")
	} else {
		mntOpts = append(mntOpts, "rbind")
		mntOpts = append(mntOpts, "rw")
	}

	cMount := specs.Mount{
		Source:      *hostPath + volMount.SubPath,
		Destination: volMount.MountPath,
		Options:     mntOpts, //volMount.ReadOnly,
		Type:        "bind",
	}
	fmt.Printf("Mount source %s target %s propagation %s\n", cMount.Source, cMount.Destination, "whatever")

	return &cMount

}

func (dri *ContainerdRuntimeInterface) SetContainerResources(namespace string, podname string, dc *v1.Container) string {
	//some default values
	oneCpu, _ := resource.ParseQuantity("1")
	defaultMem, _ := resource.ParseQuantity("150Mi")

	fmt.Printf("Checking cpu limiting support\n")
	supportCheck, _ := manager.ExecCmdBash("ls /sys/fs/cgroup/cpu/ | grep -E 'cpu.cfs_[a-z]*_us'")
	cpuSupported := supportCheck != ""
	fmt.Printf("Cpu limit support %s\n", cpuSupported)

	var cpuLimit float64
	var memLimit int64

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
		memLimit = memory.Value()
	} else {
		//this means neither was set, so update both limit and request with a default value
		dc.Resources.Limits[v1.ResourceMemory] = defaultMem
		dc.Resources.Requests[v1.ResourceMemory] = defaultMem
		memLimit = 150 * 1024 * 1024 //150 Mi
	}

	if cpuSupported {
		cpu := dc.Resources.Limits.Cpu()
		if cpu.IsZero() {
			//same if cpu limit isn't filled in
			cpu = dc.Resources.Requests.Cpu()
			dc.Resources.Limits[v1.ResourceCPU] = *cpu
		}
		if !cpu.IsZero() {
			cpuLimit = float64(cpu.MilliValue()) / 1000.0
		} else {
			dc.Resources.Limits[v1.ResourceCPU] = oneCpu
			dc.Resources.Requests[v1.ResourceCPU] = oneCpu
			cpuLimit = 1 // 1 CPU
		}
	}

	cgroup := CreateCgroupIfNotExists(namespace, podname, dc.Name)
	SetMemoryLimit(cgroup, memLimit)
	SetCpuLimit(cgroup, cpuLimit)
	return cgroup
}

func (dri *ContainerdRuntimeInterface) UpdatePod(pod *v1.Pod) {
	containers := pod.Spec.Containers
	namespace := pod.ObjectMeta.Namespace

	dri.podSpecs[namespace+"_"+pod.ObjectMeta.Name] = pod

	for _, cont := range containers {
		dri.UpdateContainer(namespace, pod, &cont)
	}
	fmt.Println("Setting podsChanged true")
	dri.podsChanged = true
}

func (dri *ContainerdRuntimeInterface) UpdateContainer(namespace string, pod *v1.Pod, dc *v1.Container) {
	dri.StopContainer(namespace, pod, dc)
	dri.DeployContainer(namespace, pod, dc)
}

func (dri *ContainerdRuntimeInterface) DeletePod(pod *v1.Pod) {
	containers := pod.Spec.Containers
	namespace := pod.ObjectMeta.Namespace

	delete(dri.podSpecs, namespace+"_"+pod.ObjectMeta.Name)

	for _, cont := range containers {
		dri.StopContainer(namespace, pod, &cont)
	}
	RemoveNetNamespace(pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)

	FreeIP(pod.ObjectMeta.Namespace, pod.ObjectMeta.Name)
	fmt.Println("Setting podsChanged true")
	dri.podsChanged = true
}

func (dri *ContainerdRuntimeInterface) StopContainer(namespace string, pod *v1.Pod, dc *v1.Container) bool {
	fullName := dri.GetContainerName(namespace, *pod, *dc) //namespace + "_" + pod.ObjectMeta.Name + "_" + dc.Name
	fmt.Printf("Stopping container %s\n", fullName)

	tuple, found := dri.containerNameTaskMapping[fullName]
	if found {
		fmt.Printf("Stopping and removing task id %s\n", tuple.task.ID())
		//time, _ := time.ParseDuration("10s")
		//err := dri.cli.ContainerStop(dri.ctx, contID, nil)
		exitStatus, err := tuple.task.Delete(dri.ctx)
		fmt.Printf("Task stopped status %d \n", exitStatus)

		if err == nil {
			delete(dri.containerNameTaskMapping, fullName)
			fmt.Printf("Removing container %s\n", fullName)

			err = tuple.container.Delete(dri.ctx, containerd.WithSnapshotCleanup)
			DeleteCgroup(GetCgroup(namespace, pod.ObjectMeta.Name, dc.Name))
			if err != nil {
				fmt.Println(err.Error())
			}
		} else {
			fmt.Println(err.Error())
		}
		return err == nil
	}
	return true
}

func (dri *ContainerdRuntimeInterface) UpdatePodStatus(namespace string, pod *v1.Pod) {
	pod.Status.HostIP = config.Cfg.DeviceIP
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
		//dri.SetupPodIPs(pod)
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

func (dri *ContainerdRuntimeInterface) CheckInitContainers(namespace string, pod *v1.Pod) {
	//ctx := namespaces.WithNamespace(context.Background(), namespace)
	allContainersDone := true
	noErrors := true

	containerStatuses := []v1.ContainerStatus{}
	for _, cont := range pod.Spec.Containers {
		fullName := dri.GetContainerNameAlt(namespace, pod.ObjectMeta.Name, cont.Name)
		tuple, found := dri.containerNameTaskMapping[fullName]
		if found {
			state := v1.ContainerState{}

			taskStatus, _ := tuple.task.Status(dri.ctx)
			switch taskStatus.Status { //contJSON.State.Status {
			case containerd.Created: //"created":
				allContainersDone = false
				state.Waiting = &v1.ContainerStateWaiting{
					Reason:  "Starting",
					Message: "Starting container",
				}
			case containerd.Running: //"running":
				fallthrough
			case containerd.Paused: //"paused":
				fallthrough
			case containerd.Pausing: //"restarting":
				allContainersDone = false
				state.Running = &v1.ContainerStateRunning{ //add real time later
					StartedAt: metav1.Now(),
				}
			case containerd.Stopped: //"dead":
				if taskStatus.ExitStatus > 0 {
					noErrors = false
				}
				state.Terminated = &v1.ContainerStateTerminated{
					Reason:      "Stopped",
					Message:     "Container stopped",
					FinishedAt:  metav1.Now(), //add real time later
					ContainerID: tuple.container.ID(),
				}
			}

			status := v1.ContainerStatus{
				Name:         cont.Name,
				State:        state,
				Ready:        false,
				RestartCount: 0,
				Image:        cont.Image,
				ImageID:      "",
				ContainerID:  tuple.container.ID(),
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

func (dri *ContainerdRuntimeInterface) UpdateContainerStatuses(namespace string, pod *v1.Pod, podStatus v1.PodCondition) {
	//ctx := namespaces.WithNamespace(context.Background(), namespace)
	allContainersRunning := true
	allContainersDone := true
	noErrors := true

	containerStatuses := []v1.ContainerStatus{}
	for _, cont := range pod.Spec.Containers {
		fullName := dri.GetContainerNameAlt(namespace, pod.ObjectMeta.Name, cont.Name)
		tuple, found := dri.containerNameTaskMapping[fullName]
		if found {
			state := v1.ContainerState{}

			taskStatus, _ := tuple.task.Status(dri.ctx)
			switch taskStatus.Status { //contJSON.State.Status {
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
				allContainersDone = false
				state.Running = &v1.ContainerStateRunning{ //add real time later
					StartedAt: metav1.Now(),
				}
			case "removing":
				fallthrough
			case "exited":
				fallthrough
			case "dead":
				if taskStatus.ExitStatus > 0 { //contJSON.State.ExitCode > 0 {
					noErrors = false
				}
				state.Terminated = &v1.ContainerStateTerminated{
					Reason:      "Stopped",
					Message:     "Container stopped",
					FinishedAt:  metav1.Now(), //add real time later
					ContainerID: tuple.container.ID(),
				}
			}

			status := v1.ContainerStatus{
				Name:         cont.Name,
				State:        state,
				Ready:        false,
				RestartCount: 0,
				Image:        cont.Image,
				ImageID:      "",
				ContainerID:  tuple.container.ID(),
			}

			containerStatuses = []v1.ContainerStatus{status}
		}
	}
	changed := UpdatePodStatus(podStatus, containerStatuses, pod, noErrors, allContainersRunning, allContainersDone)
	if changed {
		fmt.Println("Setting podsChanged true")
		dri.podsChanged = true
	}
}

func (dri *ContainerdRuntimeInterface) FetchContainerLogs(namespace string, podName string, containerName string, tail string, timestamps bool) *io.ReadCloser {
	//TODO
	return nil
}

func (dri *ContainerdRuntimeInterface) ShutdownPods() {
	for _, pod := range dri.podSpecs {
		dri.DeletePod(pod)
	}
}
