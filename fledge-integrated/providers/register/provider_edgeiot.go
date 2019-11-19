// +build !no_web_provider

package register

import (
	"fledge/fledge-integrated/providers"
	"fledge/fledge-integrated/providers/edgeiot"
)

func init() {
	register("edgeiot", initEdge)
}

func initEdge(cfg InitConfig) (providers.Provider, error) {
	return edgeiot.NewBrokerProvider(cfg.NodeName, cfg.OperatingSystem, cfg.DaemonPort)
}
