package main

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

var baseSubnetIP int
var maxSubnetIP int
var subnetMask int
var gatewayIP string

var usedAddresses map[int]string

func InitContainerNetworking(nodeSubnet string, subMask string) {
	subnetMask, _ = strconv.Atoi(subMask)
	baseSubnetIP, _ = IPStringToInt(nodeSubnet)
	maxSubnetIP = baseSubnetIP + int(math.Pow(2, float64(subnetMask)))
	gatewayIP, _ = IPIntToString(baseSubnetIP + 1)
	usedAddresses = make(map[int]string)
}

func RequestIP(namespace string, pod string) (string, error) {
	freeIP := baseSubnetIP + 2
	podName := namespace + "_" + pod
	_, taken := usedAddresses[freeIP]
	for taken {
		freeIP++
	}
	if freeIP < maxSubnetIP {
		ip, _ := IPIntToString(freeIP)
		usedAddresses[freeIP] = podName
		return ip, nil
	} else {
		return "", errors.New("Out of IP addresses")
	}
}

func FreeIP(namespace string, pod string) {
	var foundIp int = 0
	podName := namespace + "_" + pod
	for ip, cName := range usedAddresses {
		if cName == podName {
			foundIp = ip
		}
	}
	if foundIp > 0 {
		delete(usedAddresses, foundIp)
	}
}

func IPIntToString(ip int) (string, error) {
	var ipStr string = ""

	for ip > 0 {
		ipPart := ip % 256
		ipStr = strconv.Itoa(ipPart) + "." + ipStr
		ip = (ip - ipPart) / 256
	}

	return ipStr[0 : len(ipStr)-1], nil
}

func IPStringToInt(ipStr string) (int, error) {
	parts := strings.Split(ipStr, ".")
	var ip int = 0
	for i := 0; i < len(parts); i++ {
		p, _ := strconv.Atoi(parts[i])
		ip += p << uint(24-i*8)
	}
	return ip, nil
}
