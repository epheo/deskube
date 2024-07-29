package net

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"time"
)

func GetIpAddress() string {
	// Find the main network interface dynamically
	mainInterface, err := findMainInterface()
	if err != nil {
		fmt.Println("Error finding main interface:", err)
		return ""
	}
	fmt.Println("Main interface is", mainInterface.Name)

	// Get the IP address of the main network interface
	ipAddress, subnet, err := getIPv4AddressAndSubnet(mainInterface)
	if err != nil {
		fmt.Println("Error getting IP address:", err)
		return ""
	}
	fmt.Printf("IP address is %s and subnet is %s\n", ipAddress, subnet)

	return ipAddress
}

func findMainInterface() (*net.Interface, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue // Interface is down or loopback
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue // Error getting addresses
		}
		for _, addr := range addrs {
			if _, ok := addr.(*net.IPNet); ok {
				return &iface, nil // Found a valid interface with an IP address
			}
		}
	}
	return nil, fmt.Errorf("no suitable interface found")
}

func getIPv4AddressAndSubnet(iface *net.Interface) (string, string, error) {
	addrs, err := iface.Addrs()
	if err != nil {
		return "", "", err
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
			ipAddress := ipNet.IP.String()
			subnetMask := ipNet.Mask.String()
			return ipAddress, subnetMask, nil
		}
	}
	return "", "", fmt.Errorf("no IPv4 address found for interface %s", iface.Name)
}

func WaitForEndpoint(url string, hostHeader string, timeout time.Duration, interval time.Duration) bool {

	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Host", hostHeader)

	start := time.Now()
	for {
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("Request failed: %v", err)
		} else {
			if resp.StatusCode == http.StatusOK {
				log.Println("Received 200 OK")
				return true
			}
			resp.Body.Close()
		}

		if time.Since(start) > timeout {
			log.Println("Timeout reached, exiting")
			break
		}

		time.Sleep(interval)
	}

	log.Println("Failed to receive 200 OK within the timeout period")
	return false
}
