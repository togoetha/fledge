package main

import (
	"github.com/gorilla/mux"

	"net/http"
)

type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
	Queries     []string
}

type Routes []Route

func NewRouter() *mux.Router {

	router := mux.NewRouter().StrictSlash(true)
	for _, route := range routes {
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(route.HandlerFunc)
		//Queries(route.Queries)
	}

	return router
}

var routes = Routes{
	Route{
		Name:        "startVirtualKubelet",
		Method:      "GET",
		Pattern:     "/startVirtualKubelet",
		HandlerFunc: StartVirtualKubelet,
		Queries:     []string{"devName", "{devName}", "serviceIP", "{serviceIP}", "servicePort", "{servicePort}", "kubeletPort", "{kubeletPort}"},
	},
	Route{
		Name:        "stopVirtualKubelet",
		Method:      "GET",
		Pattern:     "/stopVirtualKubelet",
		HandlerFunc: StopVirtualKubelet,
		Queries:     []string{"devName", "{devName}"},
	},
	Route{
		Name:        "buildVPNClient",
		Method:      "GET",
		Pattern:     "/getVpnClient",
		HandlerFunc: BuildVPNClient,
		Queries:     []string{"devName", "{devName}"},
	},
	Route{
		Name:        "getPodCIDR",
		Method:      "GET",
		Pattern:     "/getPodCIDR",
		HandlerFunc: GetVkubeletPodCIDR,
		Queries:     []string{"devName", "{devName}"},
	},
	Route{
		Name:        "getSecret",
		Method:      "GET",
		Pattern:     "/getSecret",
		HandlerFunc: GetSecret,
		Queries:     []string{"secretName", "{secretName}"},
	},
	Route{
		Name:        "getConfigMap",
		Method:      "GET",
		Pattern:     "/getConfigMap",
		HandlerFunc: GetConfigMap,
		Queries:     []string{"mapName", "{mapName}"},
	},
}
