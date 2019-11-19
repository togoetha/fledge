package main

import (
	"encoding/json"
	"os"
)

type Config struct {
	Runtime         string `json:"runtime"`
	DeviceName      string `json:"deviceName"`
	shortDeviceName string
	DeviceIP        string `json:"deviceIP"`
	ServicePort     string `json:"servicePort"`
	KubeletPort     string `json:"kubeletPort"`
	VkubeServiceURL string `json:"vkubeServiceURL"`
	IgnoreKubeProxy string `json:"ignoreKubeProxy"`
}

func LoadConfig(filename string) (*Config, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(file)
	var config = &Config{}
	err = decoder.Decode(config)
	if err != nil {
		return nil, err
	}
	return config, nil
}
