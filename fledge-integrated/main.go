package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	kubeinformers "k8s.io/client-go/informers"
	//coordv1informers "k8s.io/client-go/informers/coordination/v1"
	corev1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	//corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"math"
	"net/http"
	"os"
	"os/signal"
	//"strconv"
	//"github.com/pkg/profile"
	"fledge/fledge-integrated/config"
	"fledge/fledge-integrated/log"
	"fledge/fledge-integrated/manager"
	"fledge/fledge-integrated/providers"
	"fledge/fledge-integrated/providers/register"
	"fledge/fledge-integrated/vkube"
	"fledge/fledge-integrated/vkubelet"
	"strings"
	"syscall"
	"time"
)

func main() {
	argsWithoutProg := os.Args[1:]
	cfgFile := "defaultconfig.json"
	if len(argsWithoutProg) > 0 {
		cfgFile = argsWithoutProg[0]
	}

	//fmt.Printf("Loading config file %s\n", cfgFile)
	config.LoadConfig(cfgFile)
	if config.Cfg.Runtime == "containerd" {
		//fmt.Println("Created containerd runtime interface")
		vkube.Cri = (&vkube.ContainerdRuntimeInterface{}).Init()
	} else {
		vkube.Cri = nil //(&DockerRuntimeInterface{}).Init()
	}

	vkube.StartTime = time.Now()

	//vkubelet router is replaced by starting the virtual kubelet thing with the "iotedge" provider
	StartVirtualKubelet()
}

func CreateVirtualKubelet() *vkubelet.Server {
	k8snil := k8sClient == nil
	fmt.Printf("CreateVirtualKubelet %t", k8snil)
	vk := vkubelet.New(vkubelet.Config{
		Client:          k8sClient,
		Namespace:       corev1.NamespaceAll,
		NodeName:        config.Cfg.ShortDeviceName,
		Taint:           taint,
		Provider:        *p,
		ResourceManager: rm,
		PodSyncWorkers:  10,
		PodInformer:     podInformer,
	})

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		rootContextCancel()
		//prof.Stop()
	}()

	return vk
}

func GetKubeClient() (*kubernetes.Clientset, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	fmt.Printf("GetKubeClient: creating config with host %s port %s\n", host, port)

	config, err := rest.InClusterConfig()
	if err != nil {
		fmt.Println(err.Error())
		return nil, err
	}
	cfgJson, _ := json.Marshal(config)
	fmt.Printf("GetKubeClient: Config created %s\n", string(cfgJson))
	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	clientsetnil := clientset == nil
	fmt.Printf("GetKubeClient: Clientset nil %t\n", clientsetnil)

	if err != nil {
		return clientset, err
	}

	//fmt.Println("GetKubeClient: Clientset created")
	return clientset, nil
}

func StartVirtualKubelet() {
	if config.Cfg.VkubeServiceURL != "" {
		//fmt.Println("StartVirtualKubelet")
		parts := strings.Split(config.Cfg.DeviceName, ".")

		namelen := int(math.Min(55, float64(len(parts[0]))))
		config.Cfg.ShortDeviceName = config.Cfg.DeviceName[:namelen]

		InitVkubeletConfig()
		//err := StartVKubelet(config.shortDeviceName, config.DeviceIP, config.ServicePort, config.KubeletPort)
		vk := CreateVirtualKubelet()

		go func() {
			//fmt.Println("Creating mux and attaching routes to provider")
			mux := http.NewServeMux()
			vkubelet.AttachAllRoutes(*p, mux)
			http.ListenAndServe(":"+config.Cfg.KubeletPort, mux)
		}()

		go func() {

			node, err := k8sClient.CoreV1().Nodes().Get(config.Cfg.ShortDeviceName, metav1.GetOptions{}) //(*nodeLister).Get(config.Cfg.ShortDeviceName)
			for node == nil {
				node, err = k8sClient.CoreV1().Nodes().Get(config.Cfg.ShortDeviceName, metav1.GetOptions{}) //(*nodeLister).Get(config.Cfg.ShortDeviceName)
				time.Sleep(500 * time.Millisecond)
			}

			podCidr := node.Spec.PodCIDR
			SetupMasterNodeRoutes(config.Cfg.ShortDeviceName)
			if err != nil {
				fmt.Println(err.Error())
			} else {
				fmt.Println("Virtual kubelet started")
				fmt.Printf("Pod subnet %s", podCidr)
				CreateHostCNI(podCidr)
			}
		}()
		if err := vk.Run(rootContext); err != nil && errors.Cause(err) != context.Canceled {
			log.G(rootContext).Fatal(err)
		}
	}
}

//find a way around this call, because otherwise the paper needs correction
func SetupMasterNodeRoutes(shortDevName string) (string, error) {
	fullURL := fmt.Sprintf("%s/getPodCIDR?devName=%s", config.Cfg.VkubeServiceURL, shortDevName)
	fmt.Printf("Calling %s\n", fullURL)
	response, err := http.Get(fullURL)
	if err != nil {
		fmt.Println(err.Error())
		return "", err
	}

	data, _ := ioutil.ReadAll(response.Body)
	defer response.Body.Close()
	return string(data), err
}

func CreateHostCNI(cidrSubnet string) {
	//TODO put interface name in config
	defer func() {
		if err := recover(); err != nil {
			fmt.Println(err)
		}
	}()

	cmd := fmt.Sprintf("ip address show dev %s | grep -E -o '[0-9\\.]{7,15}/'", config.Cfg.Interface)
	tunaddr, _ := manager.ExecCmdBash(cmd)

	tunaddrlen := len(tunaddr)
	fmt.Printf("Got tun/tap addr %s len %d\n", tunaddr, tunaddrlen)
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
	cmd = fmt.Sprintf("sh -x ./startcni.sh %s %s %s %s", ipPts[0]+"."+ipPts[1]+".0.0", subnetPts[1], subnetIpPts[0]+"."+subnetIpPts[1]+"."+subnetIpPts[2]+".1", tunPts[0]+"."+tunPts[1]+"."+tunPts[2]+".1")

	fmt.Printf("Attempting CNI initialization %s", cmd)
	output, _ := manager.ExecCmdBash(cmd)
	fmt.Println(output)

	vkube.InitContainerNetworking(subnetPts[0], subnetPts[1])
}

var k8sClient *kubernetes.Clientset
var taint *corev1.Taint

//var secretLister *corev1listers.SecretLister
//var configMapLister *corev1listers.ConfigMapLister
var p *providers.Provider
var rm *manager.ResourceManager
var podInformer corev1informers.PodInformer
var rootContext, rootContextCancel = context.WithCancel(context.Background())
var logLevel = "INFO"
var kubeSharedInformerFactoryResync = 1 * time.Minute
var kubeNamespace = corev1.NamespaceAll

func InitVkubeletConfig() {
	provider := "edgeiot"
	k8sClient, _ = GetKubeClient()
	k8snil := k8sClient == nil
	fmt.Printf("k8sClient nil check %t\n", k8snil)
	operatingSystem := "Linux"

	fmt.Printf("InitVkubeletConfig %s %s\n", provider, operatingSystem)
	// Validate operating system.
	ok, _ := providers.ValidOperatingSystems[operatingSystem]
	if !ok {
		fmt.Printf("OS check %t\n", ok)
		//log.G(context.TODO()).WithField("OperatingSystem", operatingSystem).Fatalf("Operating system not supported. Valid options are: %s", strings.Join(providers.ValidOperatingSystems.Names(), " | "))
	}

	level, err := log.ParseLevel(logLevel)
	if err != nil {
		fmt.Printf("Log level %s not parsed correctly\n", logLevel)
		//log.G(context.TODO()).WithField("logLevel", logLevel).Fatal("log level is not supported")
	}

	logrus.SetLevel(level)

	logger := log.L.WithFields(logrus.Fields{
		"provider":        provider,
		"operatingSystem": operatingSystem,
		"node":            config.Cfg.ShortDeviceName,
		"namespace":       corev1.NamespaceAll,
	})
	log.L = logger

	//fmt.Printf("Creating taint with provider %s\n", provider)
	taint, err = vkube.GetTaint(provider, "", provider)
	if err != nil {
		logger.WithError(err).Fatal("Error setting up desired kubernetes node taint")
	}

	//fmt.Printf("Creating pod informer factory")
	// Create a shared informer factory for Kubernetes pods in the current namespace (if specified) and scheduled to the current node.
	podInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(k8sClient, kubeSharedInformerFactoryResync, kubeinformers.WithNamespace(kubeNamespace), kubeinformers.WithTweakListOptions(func(options *metav1.ListOptions) {
		options.FieldSelector = fields.OneTermEqualSelector("spec.nodeName", config.Cfg.ShortDeviceName).String()
	}))
	// Create a pod informer so we can pass its lister to the resource manager.
	podInformer = podInformerFactory.Core().V1().Pods()

	// Create another shared informer factory for Kubernetes secrets and configmaps (not subject to any selectors).
	//scmInformerFactory := kubeinformers.NewSharedInformerFactoryWithOptions(k8sClient, kubeSharedInformerFactoryResync)
	// Create a secret informer and a config map informer so we can pass their listers to the resource manager.
	//sLister := scmInformerFactory.Core().V1().Secrets().Lister()
	//secretLister = &sLister
	//cfgLister := scmInformerFactory.Core().V1().ConfigMaps().Lister()
	//configMapLister = &cfgLister

	/*lLister := scmInformerFactory.Coordination().V1beta1().Leases().Lister()
	lnsLister := lLister.Leases("kube-node-lease")
	lease, _ := lnsLister.Get(config.Cfg.ShortDeviceName)

	lease.Spec.*/
	vkube.K8sClient = k8sClient
	//vkube.SecretLister = secretLister
	//vkube.CfgMapLister = configMapLister
	//vkube.NodeLister = nodeLister

	// Create a new instance of the resource manager that uses the listers above for pods, secrets and config maps.
	rm, err = manager.NewResourceManager(podInformer.Lister(), k8sClient)
	if err != nil {
		fmt.Println("Error intializing resource manager")
		fmt.Println(err.Error())
		//logger.WithError(err).Fatal("Error initializing resource manager")
	}

	// Start the shared informer factory for pods.
	go podInformerFactory.Start(rootContext.Done())
	// Start the shared informer factory for secrets and configmaps.
	//go scmInformerFactory.Start(rootContext.Done())

	daemonPort := 8101

	initConfig := register.InitConfig{
		ConfigPath:      "",
		NodeName:        config.Cfg.ShortDeviceName,
		OperatingSystem: operatingSystem,
		ResourceManager: rm,
		DaemonPort:      int32(daemonPort),
		InternalIP:      config.Cfg.DeviceIP,
	}

	jsonStr, _ := json.Marshal(initConfig)
	fmt.Printf("Created provider config %s\n", string(jsonStr))

	prov, err := register.GetProvider(provider, initConfig)

	if err != nil {
		fmt.Println("Error initializing provider")
		fmt.Println(err.Error())
		//logger.WithError(err).Fatal("Error initializing provider")
	} else {
		p = &prov
	}
}
