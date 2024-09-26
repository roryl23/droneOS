package sensor

import (
	"droneOS/internal/config"
	"fmt"
	log "github.com/sirupsen/logrus"
	"path/filepath"
	"plugin"
)

type Event struct {
	Type string
}

type Sensor struct {
	Name string
	Main func(c *config.Config, eCh *chan Event)
}

func LoadPlugins(c *config.Config) []Sensor {
	pluginDir := "./"

	// Find all .so files in the directory
	pluginFiles, err := filepath.Glob(filepath.Join(pluginDir, "*so"))
	if err != nil {
		log.Fatalf("Error finding plugin files: %v", err)
	}

	// Load functions in the configured priority order
	functions := make([]Sensor, 0)
	for _, device := range c.Drone.Sensors {
		for _, pluginFile := range pluginFiles {
			if pluginFile == fmt.Sprintf("%s_so", device.Name) {
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
				mainFunc, ok := symMain.(func(c *config.Config, sCh *chan Event))
				if !ok {
					log.Fatalf("Main function in %s has incorrect signature\n", pluginFile)
					continue
				}
				functions = append(functions, Sensor{Name: device.Name, Main: mainFunc})
			}
		}
	}
	return functions
}
