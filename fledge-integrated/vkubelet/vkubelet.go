package vkubelet

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/util/clock"
	"time"

	//	"go.opencensus.io/trace"
	"fledge/fledge-integrated/config"
	"fledge/fledge-integrated/manager"
	"fledge/fledge-integrated/providers"
	coordv1beta1 "k8s.io/api/coordination/v1beta1"
	corev1 "k8s.io/api/core/v1"
	corev1informers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	podStatusReasonProviderFailed = "ProviderFailed"
)

// Server masquarades itself as a kubelet and allows for the virtual node to be backed by non-vm/node providers.
type Server struct {
	nodeName        string
	namespace       string
	k8sClient       *kubernetes.Clientset
	taint           *corev1.Taint
	provider        providers.Provider
	resourceManager *manager.ResourceManager
	podSyncWorkers  int
	podInformer     corev1informers.PodInformer
	leaseController *LeaseController
	lease           *coordv1beta1.Lease
}

// Config is used to configure a new server.
type Config struct {
	Client          *kubernetes.Clientset
	Namespace       string
	NodeName        string
	Provider        providers.Provider
	ResourceManager *manager.ResourceManager
	Taint           *corev1.Taint
	PodSyncWorkers  int
	PodInformer     corev1informers.PodInformer
}

// New creates a new virtual-kubelet server.
// This is the entrypoint to this package.
//
// This creates but does not start the server.
// You must call `Run` on the returned object to start the server.
func New(cfg Config) *Server {
	return &Server{
		namespace:       cfg.Namespace,
		nodeName:        cfg.NodeName,
		taint:           cfg.Taint,
		k8sClient:       cfg.Client,
		resourceManager: cfg.ResourceManager,
		provider:        cfg.Provider,
		podSyncWorkers:  cfg.PodSyncWorkers,
		podInformer:     cfg.PodInformer,
	}
}

// Run creates and starts an instance of the pod controller, blocking until it stops.
//
// Note that this does not setup the HTTP routes that are used to expose pod
// info to the Kubernetes API Server, such as logs, metrics, exec, etc.
// See `AttachPodRoutes` and `AttachMetricsRoutes` to set these up.
func (s *Server) Run(ctx context.Context) error {
	s.leaseController = NewController(clock.RealClock{}, s.k8sClient, s.nodeName, int32(config.Cfg.HeartbeatTime), s.onHeartbeatFailure)

	if err := s.registerNode(ctx); err != nil {
		return err
	}

	go s.providerSyncLoop(ctx)
	s.lease, _ = s.leaseController.BackoffEnsureLease()

	return NewPodController(s).Run(ctx, s.podSyncWorkers)
}

func (s *Server) onHeartbeatFailure() {

}

// providerSyncLoop syncronizes pod states from the provider back to kubernetes
func (s *Server) providerSyncLoop(ctx context.Context) {
	const updateTime = 5
	lastUpdate := 0
	lastLease := 0
	firstHb := true
	const sleepTime = updateTime * time.Second

	t := time.NewTimer(sleepTime)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			lastUpdate += updateTime
			lastLease += updateTime
			t.Stop()

			//ctx, span := trace.StartSpan(ctx, "syncActualState")
			if s.provider.NodeChanged() || lastUpdate >= config.Cfg.HeartbeatTime || firstHb {
				fmt.Println("Node changed or heartbeat time elapsed, sending node status to master node")
				s.updateNode(ctx)
				lastUpdate = 0
				firstHb = false
			}
			if lastLease >= 15 {
				fmt.Printf("%d lease time elapsed, refreshing lease\n", config.Cfg.HeartbeatTime)
				s.lease, _ = s.leaseController.BackoffEnsureLease()
				s.leaseController.RetryUpdateLease(s.lease)
				lastLease = 0
			}
			if s.provider.PodsChanged() {
				fmt.Println("Pods changed, sending pod statuses to master node")
				s.updatePodStatuses(ctx)
			}
			s.provider.ResetChanges()
			//span.End()

			// restart the timer
			t.Reset(sleepTime)
		}
	}
}
