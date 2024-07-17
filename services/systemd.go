package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/coreos/go-systemd/v22/activation"
	"github.com/coreos/go-systemd/v22/dbus"
)

const serviceUnitName = "example-service.service"

func systemd() {
	// Check if the program was started by systemd
	listeners, err := activation.Listeners()
	if err != nil {
		log.Fatal(err)
	}

	if len(listeners) > 0 {
		// Running as a systemd service
		runAsService()
	} else {
		// Not running as a systemd service, create and enable it
		createAndEnableService()
	}
}

func createAndEnableService() {
	conn, err := dbus.NewWithContext(context.TODO())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Define the service unit content
	serviceUnitContent := fmt.Sprintf(`[Unit]
Description=Example Go Service

[Service]
ExecStart=%s
Restart=always
User=nobody
Group=nogroup
Environment=HOME=/tmp

[Install]
WantedBy=default.target
`, os.Args[0])

	// Write the service unit content to a temporary file
	tmpFile, err := os.CreateTemp("", "example-service")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	_, err = tmpFile.WriteString(serviceUnitContent)
	if err != nil {
		log.Fatal(err)
	}

	// Reload systemd to pick up the new service unit
	err = conn.Reload()
	if err != nil {
		log.Fatal(err)
	}

	// Enable the service to start on boot
	_, _, err = conn.EnableUnitFilesContext(context.TODO(), []string{tmpFile.Name()}, false, true)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Systemd service %s created and enabled.\n", serviceUnitName)
}

func runAsService() {
	// Your actual service code goes here.
	// This is the code that will be executed when the program is started by systemd.
	fmt.Println("Hello, systemd! (Running as a service)")
	select {}
}
