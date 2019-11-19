package main

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"strconv"
	"strings"

	v1 "k8s.io/api/core/v1"
)

func ExecCmdBash(dfCmd string) (string, error) {
	fmt.Printf("Executing %s\n", dfCmd)
	cmd := exec.Command("sh", "-c", dfCmd)
	stdout, err := cmd.Output()

	if err != nil {
		println(err.Error())
		return "", err
	}
	//fmt.Println(string(stdout))
	return string(stdout), nil
}

func TotalMemory() string {
	//Get memory
	memFree, _ := ExecCmdBash("free -m | grep 'Mem:'")
	//fmt.Println(memFree)
	memSize := strings.Split(reInsideWhtsp.ReplaceAllString(memFree, " "), " ")[1]
	return memSize
}

func TotalStorage() string {
	//Get disk
	diskFree, _ := ExecCmdBash("df -h | grep -E '[[:space:]]/$'")
	//fmt.Println(diskFree)
	diskSize := strings.Split(reInsideWhtsp.ReplaceAllString(diskFree, " "), " ")[1]
	return diskSize
}

func CpuCores() string {
	//Get # cpus
	stdout, _ := ExecCmdBash("nproc")
	numCpus := strings.Trim(string(stdout), "\n")
	//fmt.Println(numCpus)
	return numCpus
}

func IsMemoryPressure() bool {
	//Get memory
	//there's different types of output of the free command, trying the one with -/+ buffers/cache: first
	memFree, _ := ExecCmdBash("free -m | grep '-/+ buffers/cache:'")
	var memSize string
	if memFree != "" {
		//fmt.Println(memFree)
		memSize = strings.Split(reInsideWhtsp.ReplaceAllString(memFree, " "), " ")[2]
	} else {
		memFree, _ = ExecCmdBash("free -m | grep 'Mem:'")
		//fmt.Println(memFree)
		memSize = strings.Split(reInsideWhtsp.ReplaceAllString(memFree, " "), " ")[6]
	}
	memMb, _ := strconv.ParseFloat(memSize, 64)
	return memMb < 50
}

func IsStoragePressure() bool {
	//Get disk
	diskFree, _ := ExecCmdBash("df -h | grep -E '[[:space:]]/$'")
	//fmt.Println(diskFree)
	diskUsed := strings.Split(reInsideWhtsp.ReplaceAllString(diskFree, " "), " ")[4]
	diskPct, _ := strconv.Atoi(strings.TrimSuffix(diskUsed, "%"))
	return diskPct >= 90
}

func IsStorageFull() bool {
	//Get disk
	diskFree, _ := ExecCmdBash("df -h | grep -E '[[:space:]]/$'")
	//fmt.Println(diskFree)
	diskUsed := strings.Split(reInsideWhtsp.ReplaceAllString(diskFree, " "), " ")[4]
	diskPct, _ := strconv.Atoi(strings.TrimSuffix(diskUsed, "%"))
	return diskPct >= 98
}

func CreateVolumes(pod *v1.Pod) {
	for _, vol := range pod.Spec.Volumes {
		if vol.Secret != nil {
			//fmt.Printf("Creating secret volume %s\n", vol.Name)
			secret, err := FetchSecret(vol.Secret.SecretName)
			if err != nil {
				//fmt.Printf("Can't create secret volume %s because %s\n", vol.Name, err.Error())
			} else {
				strData := make(map[string]string)
				for key, value := range secret.Data {
					//fmt.Printf("Decode key %s value %s\n", key, value)
					strData[key] = string(value)
					//fmt.Printf("Decode value %s\n", decoded)
				}
				CreateVolumeFiles(pod, vol.Name, strData)
			}
		}
		if vol.ConfigMap != nil {
			//fmt.Printf("Creating configmap volume %s\n", vol.Name)
			cfgmap, err := FetchConfigMap(vol.ConfigMap.Name)
			if err != nil {
				//fmt.Printf("Can't create configmap volume %s because %s\n", vol.Name, err.Error())
			} else {
				CreateVolumeFiles(pod, vol.Name, cfgmap.Data)
			}
		}
	}
}

func CreateVolumeFiles(pod *v1.Pod, volName string, data map[string]string) {
	volDir := MakeVolume(pod.Name, volName)

	//fmt.Printf("Creating volume %s files in %s\n", volName, volDir)
	for filename, contents := range data {
		//fmt.Printf("Creating file %s/%s\n", volDir, filename)

		file := fmt.Sprintf("%s/%s", volDir, filename)
		err := ioutil.WriteFile(file, []byte(contents), 0777)
		if err != nil {
			//fmt.Println(err.Error())
		}
	}
}

func MakeVolume(podname string, volname string) string {
	volDir := fmt.Sprintf("/var/vkube/mounts/%s/%s", podname, volname)
	cmd := fmt.Sprintf("mkdir -p %s", volDir)
	ExecCmdBash(cmd)
	return volDir
}

func GetHostMountPath(pod *v1.Pod, vol v1.Volume) *string {
	if vol.VolumeSource.HostPath != nil {
		//fmt.Printf("Volume type HostPath\n")
		return &vol.HostPath.Path
	} else if vol.VolumeSource.Secret == nil {
		//fmt.Printf("Volume type Secret\n")
		mountPath := MakeVolume(pod.Name, vol.Name)
		return &mountPath
	} else if vol.VolumeSource.ConfigMap == nil {
		//fmt.Printf("Volume type ConfigMap\n")
		mountPath := MakeVolume(pod.Name, vol.Name)
		return &mountPath
	}
	fmt.Printf("Volume type not supported, can't mount\n")
	return nil
}

func GetHighestPodStatus(pod *v1.Pod) *v1.PodCondition {
	//need to figure out from the array of conditions what the "highest" ranking status is and return that
	//in order of "importance":
	//PodReasonUnschedulable: leave the pod alone, there's nothing we can do for it anymore. Happens on container errors.
	//PodReady: leave the pod alone, happens when all containers are running and all CNI/cgroup stuff is taken care of
	//ContainersReady: technically the same as PodReady, since CNI/cgroup stuff will be taken care of faster than most containers start, so we'll ignore this one
	//PodInitialized (true): all init containers have successfully run and the real containers can be started
	//PodInitialized (false): same status as above, but indicates that the init containers are NOT all ready yet, so keep checking them
	//PodScheduled: indicates the pod has been sent to the node, but nothing else has been done yet. Status should not be updated by poll thread, it will be elevated
	//to PodInitialized (false) or PodInitialized (true) by deployPod.

	//would love to get this dump of a method cleaned up, but no idea how to do it because of the way the statuses are defined
	var scheduled v1.PodCondition
	var initialized v1.PodCondition
	var podReady v1.PodCondition
	var podUnschedulable v1.PodCondition

	for _, condition := range pod.Status.Conditions {
		if condition.Type == v1.PodScheduled {
			scheduled = condition
		} else if condition.Type == v1.PodReady {
			podReady = condition
		} else if condition.Type == v1.PodReasonUnschedulable {
			podUnschedulable = condition
		} else if condition.Type == v1.PodInitialized {
			initialized = condition
		}
	}

	if podUnschedulable.Status == v1.ConditionTrue {
		return &podUnschedulable
	} else if podReady.Status == v1.ConditionTrue {
		return &podReady
	} else if initialized.Status == v1.ConditionTrue {
		return &initialized
	} else if scheduled.Status == v1.ConditionTrue {
		return &scheduled
	}
	//well this can't happen, but whatever, better catch it anyway
	return nil
}
