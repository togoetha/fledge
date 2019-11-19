package manager

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
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

var computeCUDA bool
var computeOpenCL bool
var computeOpenCLVers string

var versionMatcher = regexp.MustCompile(`[0-9]+\.[0-9]+`)

func LoadGpuInfo() {
	if computeOpenCLVers == "" {
		computeCUDA = false
		computeOpenCL = false
		computeOpenCLVers = ""

		computeInfo, _ := ExecCmdBash("./clvers")
		computeDevices := strings.Split(computeInfo, "\n")

		//GPU##NVIDIA CUDA##GeForce GTX 1080 Ti##OpenCL 1.2 CUDA
		for _, device := range computeDevices {
			fmt.Println(device)
			caps := strings.Split(device, "##")
			if caps[0] == "GPU" && len(caps) == 4 {
				fmt.Printf("Valid %s device\n", caps[0])
				if strings.Contains(caps[1], "CUDA") || strings.Contains(caps[3], "CUDA") {
					computeCUDA = true
				}
				if strings.Contains(caps[3], "OpenCL") {
					computeOpenCL = true
					computeOpenCLVers = versionMatcher.FindString(caps[3])
				}
			}
		}
	}
}

func HasCudaCaps() bool {
	LoadGpuInfo()
	return computeCUDA
}

func HasOpenCLCaps() bool {
	LoadGpuInfo()
	return computeOpenCL
}

func OpenCLVersSupport() string {
	LoadGpuInfo()
	return computeOpenCLVers
}

var reInsideWhtsp = regexp.MustCompile(`\s+`)

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
