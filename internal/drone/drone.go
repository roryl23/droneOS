package drone

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"path/filepath"
	"plugin"
)

func Main() {
	pluginDir := "./"

	// Find all .so files in the directory
	pluginFiles, err := filepath.Glob(filepath.Join(pluginDir, "plugin_*so"))
	if err != nil {
		log.Fatalf("Error finding plugin files: %v", err)
	}

	// Iterate over each plugin file
	for _, pluginFile := range pluginFiles {
		p, err := plugin.Open(pluginFile)
		if err != nil {
			fmt.Printf("Error loading plugin %s: %v\n", pluginFile, err)
			continue
		}

		// Look up the Main function
		symMain, err := p.Lookup("Main")
		if err != nil {
			fmt.Printf("Main function not found in %s: %v\n", pluginFile, err)
			continue
		}

		// Assert that loaded symbol is a function with the correct signature
		mainFunc, ok := symMain.(func())
		if !ok {
			fmt.Printf("Main function in %s has incorrect signature\n", pluginFile)
			continue
		}

		// Call the Main function
		fmt.Printf("Calling Main from %s:\n", pluginFile)
		mainFunc()
		fmt.Println()
	}
}
