package main

import (
	"github.com/gorilla/mux"
)

func VkubeletRouter() *mux.Router {

	router := mux.NewRouter().StrictSlash(true)
	for _, route := range vkroutes {
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(route.HandlerFunc)
		//Queries(route.Queries)
	}

	return router
}

var vkroutes = Routes{
	Route{
		Name:        "createPod",
		Method:      "POST",
		Pattern:     "/createPod",
		HandlerFunc: CreatePod,
		Queries:     []string{},
	},
	Route{
		Name:        "updatePod",
		Method:      "PUT",
		Pattern:     "/updatePod",
		HandlerFunc: UpdatePod,
		Queries:     []string{},
	},
	Route{
		Name:        "deletePod",
		Method:      "DELETE",
		Pattern:     "/deletePod",
		HandlerFunc: DeletePod,
		Queries:     []string{},
	},
	Route{
		Name:        "getPod",
		Method:      "GET",
		Pattern:     "/getPod",
		HandlerFunc: GetPod,
		Queries:     []string{},
	},
	Route{
		Name:        "getContainerLogs",
		Method:      "GET",
		Pattern:     "/getContainerLogs",
		HandlerFunc: GetContainerLogs,
		Queries:     []string{},
	},
	Route{
		Name:        "getPodStatus",
		Method:      "GET",
		Pattern:     "/getPodStatus",
		HandlerFunc: GetPodStatus,
		Queries:     []string{"namespace", "{namespace}", "name", "{name}"},
	},
	Route{
		Name:        "getPods",
		Method:      "GET",
		Pattern:     "/getPods",
		HandlerFunc: GetPods,
		Queries:     []string{},
	},
	Route{
		Name:        "capacity",
		Method:      "GET",
		Pattern:     "/capacity",
		HandlerFunc: Capacity,
		Queries:     []string{},
	},
	Route{
		Name:        "nodeConditions",
		Method:      "GET",
		Pattern:     "/nodeConditions",
		HandlerFunc: NodeConditions,
		Queries:     []string{},
	},
	Route{
		Name:        "nodeAddresses",
		Method:      "GET",
		Pattern:     "/nodeAddresses",
		HandlerFunc: NodeAddresses,
		Queries:     []string{},
	},
	Route{
		Name:        "shutDown",
		Method:      "GET",
		Pattern:     "/shutDown",
		HandlerFunc: ShutDown,
		Queries:     []string{},
	},
}
