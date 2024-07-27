package system

import (
	"context"
	"log"
	"strings"

	"github.com/coreos/go-systemd/v22/dbus"
)

func EnableStartService(servicesNames []string) {

	conn, err := dbus.NewWithContext(context.TODO())
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// Reload systemd to pick up the new service unit
	err = conn.ReloadContext(context.TODO())
	if err != nil {
		log.Fatal(err)
	}

	// Enable the service to start on boot
	_, _, err = conn.EnableUnitFilesContext(context.TODO(), servicesNames, false, true)
	if err != nil {
		log.Fatal(err)
	}

	// Start the service
	for _, serviceName := range servicesNames {
		tokens := strings.Split(serviceName, "/")
		unitName := tokens[len(tokens)-1]
		_, err = conn.StartUnitContext(context.TODO(), unitName, "replace", nil)
		if err != nil {
			log.Fatal(err)
		}
	}

	log.Printf("Systemd service %s created and enabled.\n", servicesNames)
}
