package config

import (
	"droneOS/internal/input/sensor"
	"fmt"
	log "github.com/sirupsen/logrus"
	"path/filepath"
	"plugin"
)

func LoadSensorPlugins(c *Config) []func(c *Config, ch chan<- sensor.Event) {
	pluginDir := "./"

	// Find all .so files in the directory
	pluginFiles, err := filepath.Glob(filepath.Join(pluginDir, "plugin_*_so"))
	if err != nil {
		log.Fatalf("Error finding plugin files: %v", err)
	}

	// Load functions in the configured priority order
	functions := make([]func(c *Config, ch chan<- sensor.Event), 0)
	for _, pluginName := range c.SensorPriority {
		for _, pluginFile := range pluginFiles {
			if pluginFile == fmt.Sprintf("plugin_%s_so", pluginName) {
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
				mainFunc, ok := symMain.(func(c *Config, ch chan<- sensor.Event))
				if !ok {
					log.Fatalf("Main function in %s has incorrect signature\n", pluginFile)
					continue
				}
				functions = append(functions, mainFunc)
			}
		}
	}
	return functions
}

func LoadControlAlgorithmPlugins(c *Config) []func(c *Config) {
	pluginDir := "./"

	// Find all .so files in the directory
	pluginFiles, err := filepath.Glob(filepath.Join(pluginDir, "plugin_*_so"))
	if err != nil {
		log.Fatalf("Error finding plugin files: %v", err)
	}

	// Load functions in the configured priority order
	functions := make([]func(c *Config), 0)
	for _, pluginName := range c.ControlAlgorithmPriority {
		for _, pluginFile := range pluginFiles {
			if pluginFile == fmt.Sprintf("plugin_%s_so", pluginName) {
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
				mainFunc, ok := symMain.(func(c *Config))
				if !ok {
					log.Fatalf("Main function in %s has incorrect signature\n", pluginFile)
					continue
				}
				functions = append(functions, mainFunc)
			}
		}
	}
	return functions
}
