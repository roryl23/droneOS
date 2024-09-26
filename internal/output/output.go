package output

import (
	"droneOS/internal/config"
	"fmt"
	log "github.com/sirupsen/logrus"
	"path/filepath"
	"plugin"
	"time"
)

type Task struct {
	Priority int    // lower value means higher priority
	Index    int    // the index of the item in the heap
	Name     string // plugin
	Input    interface{}
}

type Output struct {
	Name string
	Main func(i interface{}) error
}

func LoadPlugins(c *config.Config) []Output {
	pluginDir := "./"

	// Find all .so files in the directory
	pluginFiles, err := filepath.Glob(filepath.Join(pluginDir, "*so"))
	if err != nil {
		log.Fatalf("Error finding plugin files: %v", err)
	}

	// Load functions in the configured priority order
	functions := make([]Output, 0)
	for _, device := range c.Drone.Outputs {
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
				mainFunc, ok := symMain.(func(i interface{}) error)
				if !ok {
					log.Fatalf("Main function in %s has incorrect signature\n", pluginFile)
					continue
				}
				functions = append(functions, Output{Name: device.Name, Main: mainFunc})
			}
		}
	}
	return functions
}

func Main(tq chan Task, plugins []Output) {
	for {
		task := <-tq
		for _, output := range plugins {
			if output.Name == task.Name {
				err := output.Main(task.Input)
				if err != nil {
					log.Error(err)
				}
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
}
