package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	v1 "k8s.io/api/core/v1"
)

func StartVKubelet(shortDevName string, deviceIP string, servicePort string, kubeletPort string) error {
	fullURL := fmt.Sprintf("%s/startVirtualKubelet?devName=%s&serviceIP=%s&servicePort=%s&kubeletPort=%s", config.VkubeServiceURL, shortDevName, deviceIP, servicePort, kubeletPort)
	fmt.Printf("Calling %s\n", fullURL)
	_, err := http.Get(fullURL)

	return err
}

func FetchPodCIDR(shortDevName string) (string, error) {
	fullURL := fmt.Sprintf("%s/getPodCIDR?devName=%s", config.VkubeServiceURL, shortDevName)
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

func FetchSecret(name string) (v1.Secret, error) {
	fullURL := fmt.Sprintf("%s/getSecret?secretName=%s", config.VkubeServiceURL, name)
	fmt.Printf("Calling %s\n", fullURL)
	response, err := http.Get(fullURL)
	if err != nil {
		fmt.Println(err.Error())
		return v1.Secret{}, err
	}

	decoder := json.NewDecoder(response.Body)
	var secret v1.Secret
	err = decoder.Decode(&secret)
	if err != nil {
		fmt.Println(err.Error())
	}
	defer response.Body.Close()
	return secret, nil
}

func FetchConfigMap(name string) (v1.ConfigMap, error) {
	fullURL := fmt.Sprintf("%s/getConfigMap?mapName=%s", config.VkubeServiceURL, name)
	fmt.Printf("Calling %s\n", fullURL)
	response, err := http.Get(fullURL)
	if err != nil {
		fmt.Println(err.Error())
		return v1.ConfigMap{}, err
	}

	decoder := json.NewDecoder(response.Body)
	var cfg v1.ConfigMap
	err = decoder.Decode(&cfg)
	if err != nil {
		fmt.Println(err.Error())
	}
	defer response.Body.Close()
	return cfg, nil
}

func StopVirtualKubelet(devName string) error {
	fullURL := fmt.Sprintf("%s/stopVirtualKubelet?devName=%s", config.VkubeServiceURL, devName)
	fmt.Printf("Calling %s\n", fullURL)
	response, err := http.Get(fullURL)

	if err != nil {
		fmt.Println(err.Error())
	}
	_, err = ioutil.ReadAll(response.Body)
	defer response.Body.Close()

	return nil
}
