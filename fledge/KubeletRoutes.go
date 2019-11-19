package main

import (
	"github.com/gorilla/mux"
)

func KubeletRouter() *mux.Router {

	router := mux.NewRouter().StrictSlash(true)
	for _, route := range kubroutes {
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(route.HandlerFunc)
		//Queries(route.Queries)
	}

	return router
}

var kubroutes = Routes{
	Route{
		Name:        "statsSummary",
		Method:      "GET",
		Pattern:     "/stats/summary/",
		HandlerFunc: StatsSummary,
		Queries:     []string{},
	},
}
