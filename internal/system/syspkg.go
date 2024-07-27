package system

import (
	"github.com/bluet/syspkg"
	"log"
)

func InstallSysPkg(pkgNames []string) {
	// Initialize SysPkg with all available package managers on current system
	includeOptions := syspkg.IncludeOptions{
		AllAvailable: true,
	}
	syspkgManagers, err := syspkg.New(includeOptions)
	if err != nil {
		log.Printf("Error initializing SysPkg: %v\n", err)
		return
	}
	pkgManager := syspkgManagers.GetPackageManager("")

	// Install packages
	_, err = pkgManager.Install(pkgNames, nil)
	if err != nil {
		log.Printf("Error installing package %s: %v\n", pkgNames, err)
		return
	}
}
