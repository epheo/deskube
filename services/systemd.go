package services

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/coreos/go-systemd/v22/dbus"
	"github.com/epheo/deskube/types"
)

func CreateAndEnableService(service types.Service) {

	serviceUnitName := fmt.Sprintf("%s.service", service.Name)

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
WantedBy=multi-user.target
`, service.ExecStart)

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
	err = conn.ReloadContext(context.TODO())
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
