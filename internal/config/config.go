package config

import (
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"io"
	"os"
)

type BaseConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type Device struct {
	Name string `yaml:"name"`
	Pins []int  `yaml:"pins"`
}

type Radio struct {
	Name      string `yaml:"name"`
	AlwaysUse bool   `yaml:"alwaysUse"`
	Pins      []int  `yaml:"pins"`
	UsbId     string `yaml:"usbId"`
}

type DroneConfig struct {
	ID                       int      `yaml:"id"`
	Radio                    Radio    `yaml:"radio"`
	AlwaysUseRadio           bool     `yaml:"alwaysUseRadio"`
	Sensors                  []Device `yaml:"sensors"`
	Outputs                  []Device `yaml:"outputs"`
	ControlAlgorithmPriority []string `yaml:"controlAlgorithmPriority"`
}

type Config struct {
	Base  BaseConfig  `yaml:"base"`
	Drone DroneConfig `yaml:"drone"`
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
