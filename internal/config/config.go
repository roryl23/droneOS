package config

import (
	"fmt"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"io"
	"os"
	"path/filepath"
	"plugin"
)

type BaseConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type DroneConfig struct {
	ID             int  `yaml:"id"`
	AlwaysUseRadio bool `yaml:"alwaysUseRadio"`
}

type Config struct {
	Base           BaseConfig  `yaml:"base"`
	Drone          DroneConfig `yaml:"drone"`
	PluginPriority []string    `yaml:"pluginPriority"`
}

func GetConfig(file string) Config {
	handle, err := os.Open(file)
	if err != nil {
		log.Fatalf("failed to open file: %s", err)
	}
	defer handle.Close()

	content, err := io.ReadAll(handle)
	if err != nil {
		log.Fatalf("failed to read file: %s", err)
	}

	c := Config{}
	err = yaml.Unmarshal(content, &c)
	if err != nil {
		log.Error(err)
	}

	return c
}

func LoadPlugins(c *Config) []func(c *Config) {
	pluginDir := "./"

	// Find all .so files in the directory
	pluginFiles, err := filepath.Glob(filepath.Join(pluginDir, "plugin_*_so"))
	if err != nil {
		log.Fatalf("Error finding plugin files: %v", err)
	}

	// Load functions in the configured priority order
	functions := make([]func(c *Config), 0)
	for _, pluginName := range c.PluginPriority {
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
