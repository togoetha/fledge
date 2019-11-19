package main

import (
	"log"
	"net/http"
	"os"
)

var kubernetesHost string
var kubernetesPort string
var defaultPodFile string

func main() {

	argsWithoutProg := os.Args[1:]
	kubernetesHost = argsWithoutProg[0]
	kubernetesPort = argsWithoutProg[1]
	defaultPodFile = argsWithoutProg[2]

	router := NewRouter()

	log.Fatal(http.ListenAndServe(":8180", router))
}
