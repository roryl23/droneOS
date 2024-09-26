package control

import (
	"droneOS/internal/config"
	"droneOS/internal/input/sensor"
	"droneOS/internal/output"
	"fmt"
	log "github.com/sirupsen/logrus"
	"path/filepath"
	"plugin"
)

type ControlAlgorithm struct {
	Name string
	Main func(c *config.Config, priority int, sCh *chan sensor.Event, pq *output.Queue)
}

func LoadPlugins(c *config.Config) []ControlAlgorithm {
	pluginDir := "./"

	// Find all .so files in the directory
	pluginFiles, err := filepath.Glob(filepath.Join(pluginDir, "ca_*so"))
	if err != nil {
		log.Fatalf("Error finding plugin files: %v", err)
	}

	// Load functions in the configured priority order
	functions := make([]ControlAlgorithm, 0)
	for _, pluginName := range c.Drone.ControlAlgorithmPriority {
		for _, pluginFile := range pluginFiles {
			if pluginFile == fmt.Sprintf("%s_so", pluginName) {
				p, err := plugin.Open(pluginFile)
				if err != nil {
					log.Fatalf("Error loading plugin %s: %v\n", pluginFile, err)
					continue
				}
				// Look up the Main function
				symMain, err := p.Lookup("Main")
				if err != nil {
					log.Fatalf("Main function not found in %s: %v\n", pluginFile, err)
					continue
				}
				// Assert that loaded symbol is a function with the correct signature
				mainFunc, ok := symMain.(func(c *config.Config, p int, sCh *chan sensor.Event, pq *output.Queue))
				if !ok {
					log.Fatalf("Main function in %s has incorrect signature\n", pluginFile)
					continue
				}
				functions = append(functions, ControlAlgorithm{Name: pluginName, Main: mainFunc})
			}
		}
	}
	return functions
}
