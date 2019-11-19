package vkube

import (
	"fledge/fledge-integrated/config"
	"fmt"
	"io"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"regexp"
	"strings"
	"time"
)

var reInsideWhtsp = regexp.MustCompile(`\s+`)
var Cri ContainerRuntimeInterface
var StartTime time.Time

type ContainerRuntimeInterface interface {
	Init() ContainerRuntimeInterface
	GetContainerName(namespace string, pod v1.Pod, dc v1.Container) string
	GetContainerNameAlt(namespace string, podName string, dcName string) string
	DeployPod(pod *v1.Pod)
	DeployContainer(namespace string, pod *v1.Pod, dc *v1.Container) (string, error)
	//UpdatePostCreationPodStatus(pod *v1.Pod)
	UpdatePod(pod *v1.Pod)
	//UpdateContainer(namespace string, pod *v1.Pod, dc *v1.Container)
	DeletePod(pod *v1.Pod)
	GetPod(namespace string, name string) (*v1.Pod, bool)
	GetPods() []*v1.Pod
	//StopContainer(namespace string, pod *v1.Pod, dc *v1.Container) bool
	//UpdatePodStatus(namespace string, pod *v1.Pod)
	FetchContainerLogs(namespace string, podName string, containerName string, tail string, timestamps bool) *io.ReadCloser
	ShutdownPods()
	PodsChanged() bool
	ResetFlags()
}

func GetEnvAsStringArray(dc *v1.Container) []string {
	//fmt.Printf("Number of env vars %d\n", len(dc.Env))
	envVars := []string{}
	for _, evar := range dc.Env {
		fmt.Printf("Handling env var %s\n", evar.Name)
		if evar.ValueFrom == nil {
			fmt.Printf("Handling env var %s value type %s\n", evar.Name, evar.Value)
			envVars = append(envVars, evar.Name+"='"+evar.Value+"'")
		}
		//replace it in the container cmds
		var cmds = []string{}
		for _, cmd := range dc.Command {
			cmds = append(cmds, strings.Replace(cmd, "$("+evar.Name+")", evar.Value, -1))
		}
		dc.Command = cmds
	}
	return envVars
}

func IgnoreKubeProxy(pod *v1.Pod) {
	fmt.Printf("Ignoring kube proxy: %s\n", pod.ObjectMeta.Name)
	pod.Status.Phase = v1.PodFailed
	pod.Status.Reason = "Kube proxy disabled"
	pod.Status.Message = "Config doesn't allow kube proxy to be started"

	cond := v1.PodCondition{
		Type:               v1.PodReasonUnschedulable,
		Status:             v1.ConditionTrue,
		LastProbeTime:      metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             "Kube proxy disabled",
		Message:            "Config doesn't allow kube proxy to be started",
	}
	pod.Status.Conditions = append(pod.Status.Conditions, cond)
}

func UpdatePodStatus(podStatus v1.PodCondition, containerStatuses []v1.ContainerStatus, pod *v1.Pod, noErrors bool, allContainersRunning bool, allContainersDone bool) bool {
	pod.Status.ContainerStatuses = containerStatuses

	changed := false
	fmt.Printf("Updating pod %s noErrors %t allContainersRunning %t allContainersDone %t\n", pod.ObjectMeta.Name, noErrors, allContainersRunning, allContainersDone)
	curPhase := pod.Status.Phase
	if !noErrors {
		pod.Status.Phase = v1.PodFailed
		pod.Status.Reason = "Failed"
		pod.Status.Message = "Container errors detected"
	} else if !allContainersRunning && !allContainersDone {
		pod.Status.Phase = v1.PodPending
		pod.Status.Reason = "Starting"
		pod.Status.Message = "Starting pod"
	} else if allContainersRunning && !allContainersDone {
		pod.Status.Phase = v1.PodRunning
		pod.Status.Reason = "Running"
		pod.Status.Message = "All containers running"
	} else if allContainersDone {
		pod.Status.Phase = v1.PodSucceeded //split this up later based on return codes
		pod.Status.Reason = "Finished"
		pod.Status.Message = "All containers exited"
	}
	if curPhase != pod.Status.Phase {
		changed = true
	}

	if allContainersRunning && podStatus.Type != v1.PodReady {
		cond := v1.PodCondition{
			Type:               v1.PodReady,
			Status:             v1.ConditionTrue,
			LastProbeTime:      metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "Running",
			Message:            "All containers up and running",
		}
		pod.Status.Conditions = append(pod.Status.Conditions, cond)

		cond = v1.PodCondition{
			Type:               v1.ContainersReady,
			Status:             v1.ConditionTrue,
			LastProbeTime:      metav1.Now(),
			LastTransitionTime: metav1.Now(),
			Reason:             "Running",
			Message:            "All containers up and running",
		}
		pod.Status.Conditions = append(pod.Status.Conditions, cond)
		changed = true
	}
	return changed
}

func UpdateInitPodStatus(pod *v1.Pod, noErrors bool, allContainersDone bool) {
	pod.Status.Phase = v1.PodPending
	if noErrors && allContainersDone {
		//set initialized status + true
		conditions := []v1.PodCondition{}
		for _, cond := range pod.Status.Conditions {
			if cond.Type != v1.PodInitialized {
				conditions = append(conditions, cond)
			}
			initCond := v1.PodCondition{
				Type:               v1.PodInitialized,
				Status:             v1.ConditionTrue,
				LastProbeTime:      metav1.Now(),
				LastTransitionTime: metav1.Now(),
				Reason:             "Initialized",
				Message:            "Init containers done",
			}
			conditions = append(conditions, initCond)
		}
		pod.Status.Conditions = conditions
	} else {
		pod.Status.Phase = v1.PodFailed
		pod.Status.Reason = "Failed"
		pod.Status.Message = "Container errors detected"
	}
}

func UpdatePostCreationPodStatus(pod *v1.Pod, initContainers bool) {
	pod.Status.HostIP = config.Cfg.DeviceIP

	time := metav1.Now()
	pod.Status.StartTime = &time

	containerStatuses := []v1.ContainerStatus{}
	for _, cont := range pod.Spec.Containers {
		state := v1.ContainerState{
			Waiting: &v1.ContainerStateWaiting{
				Reason:  "Starting",
				Message: "Starting container",
			},
		}

		status := v1.ContainerStatus{
			Name:         cont.Name,
			State:        state,
			Ready:        false,
			RestartCount: 0,
			Image:        cont.Image,
			ImageID:      "",
		}

		containerStatuses = append(containerStatuses, status)
	}
	pod.Status.ContainerStatuses = containerStatuses

	pod.Status.Phase = v1.PodPending
	pod.Status.Reason = "Starting"
	pod.Status.Message = "Starting pod"

	//pod conditions?
	status := v1.ConditionTrue
	reason := "Starting"
	if initContainers {
		status = v1.ConditionFalse
		reason = "Initializing"
	}

	condition := v1.PodCondition{
		Type:               v1.PodInitialized,
		Status:             status,
		LastProbeTime:      metav1.Now(),
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            "Starting pod/init containers",
	}
	pod.Status.Conditions = append(pod.Status.Conditions, condition)
}
