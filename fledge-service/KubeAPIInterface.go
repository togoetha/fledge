package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"os/exec"
	"strings"
	"time"
)

var podNameVar string = "%podname%"
var serviceURLVar string = "%serviceUrl%"
var kubernetesHostVar string = "%kubernetesHost%"
var kubernetesPortVar string = "%kubernetesPort%"
var kubeletPortVar string = "%kubeletPort%"

func StartKubelet(name string, remoteUrl string, remoteKubeletPort string) error {
	clientset, _ := GetKubeClient()

	deploymentsClient := clientset.AppsV1().Deployments(apiv1.NamespaceDefault)
	fmt.Println("Deployments client retrieved")

	//var replicas int32 = 1
	//var hostpathtype = apiv1.HostPathDirectory
	template, err := ioutil.ReadFile(defaultPodFile)
	if err != nil {
		return err
	}
	fmt.Println("Template read")

	templateStr := string(template)
	templateStr = strings.Replace(templateStr, podNameVar, name+"-vkube", -1)
	templateStr = strings.Replace(templateStr, serviceURLVar, remoteUrl, 1)
	templateStr = strings.Replace(templateStr, kubernetesHostVar, kubernetesHost, 1)
	templateStr = strings.Replace(templateStr, kubernetesPortVar, kubernetesPort, 1)
	templateStr = strings.Replace(templateStr, kubeletPortVar, remoteKubeletPort, 1)

	fmt.Println("Template parsed")
	//fmt.Println(templateStr)
	template = []byte(templateStr)

	var deployment appsv1.Deployment
	json.Unmarshal(template, &deployment)

	// Create Deployment
	fmt.Println("Creating deployment")
	result, err := deploymentsClient.Create(&deployment)
	if err != nil {
		return err
	}
	fmt.Printf("Created deployment %q.\n", result.GetObjectMeta().GetName())

	//cmdLine version
	/*podYamlBytes, _ := ioutil.ReadFile(defaultPodFile)
	podYaml := string(podYamlBytes)

	podYaml = strings.Replace(podYaml, podNameVar, name+"-vkube", -1)
	podYaml = strings.Replace(podYaml, serviceURLVar, remoteUrl, 1)
	podYaml = strings.Replace(podYaml, kubernetesHostVar, kubernetesHost, 1)
	podYaml = strings.Replace(podYaml, kubernetesPortVar, kubernetesPort, 1)
	podYaml = strings.Replace(podYaml, kubeletPortVar, remoteKubeletPort, 1)

	_, err := ExecCmdBash("kubectl apply -f - <<EOF\n" + podYaml + "\nEOF")*/

	//contents, _ := ExecCmdBash("ls -l")
	//fmt.Println(contents)

	return nil
}

func GetNodePodCIDR(name string) (*string, *string, error) {
	clientset, _ := GetKubeClient()

	nodesClient := clientset.CoreV1().Nodes()
	var cidr string = ""
	var podaddr string = ""
	for cidr == "" {
		fmt.Printf("Getting node %s\n", name)
		node, err := nodesClient.Get(name, metav1.GetOptions{})

		if err != nil {
			//return nil, err
			fmt.Println("Failed to get node")
			fmt.Println(err.Error())
		} else {
			cidr = node.Spec.PodCIDR
			addresses := node.Status.Addresses

			for _, address := range addresses {
				if address.Type == apiv1.NodeInternalIP {
					podaddr = address.Address
				}
			}
			fmt.Printf("Node found cidr %s address %s\n", cidr, podaddr)
		}
		time.Sleep(300 * time.Millisecond)
	}
	return &cidr, &podaddr, nil
}

func StopKubelet(devName string) error {
	depName := devName + "-vkube"
	fmt.Printf("Deleting deployment %s\n", depName)

	clientset, _ := GetKubeClient()
	deploymentsClient := clientset.AppsV1().Deployments(apiv1.NamespaceDefault)

	deletePolicy := metav1.DeletePropagationForeground
	err := deploymentsClient.Delete(depName, &metav1.DeleteOptions{
		PropagationPolicy: &deletePolicy,
	})
	if err != nil {
		return err
	}
	fmt.Println("Deleted deployment.")

	return nil
}

func GetKubeSecret(name string) (*apiv1.Secret, error) {
	clientset, _ := GetKubeClient()
	secretsClient := clientset.CoreV1().Secrets(apiv1.NamespaceAll)

	secret, err := secretsClient.Get(name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return secret, nil
}

func GetKubeConfigMap(name string) (*apiv1.ConfigMap, error) {
	clientset, _ := GetKubeClient()
	cfgsClient := clientset.CoreV1().ConfigMaps("kube-system")

	cfg, err := cfgsClient.Get(name, metav1.GetOptions{})
	if err != nil {
		cfgsClient := clientset.CoreV1().ConfigMaps(apiv1.NamespaceDefault)

		cfg, err = cfgsClient.Get(name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

func ExecCmdBash(dfCmd string) (string, error) {
	fmt.Printf("Executing cmd %s\n", dfCmd)

	cmd := exec.Command("sh", "-c", dfCmd)
	//stdout, err := cmd.Output()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		fmt.Println(fmt.Sprint(err) + ": " + stderr.String())
		return "", err
	}

	//fmt.Println(string(stdout))
	return stdout.String(), nil
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
