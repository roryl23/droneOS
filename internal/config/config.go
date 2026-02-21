package config

import (
	"io"
	"os"

	"github.com/rs/zerolog/log"
	"gopkg.in/yaml.v3"
)

type BaseConfig struct {
	Host       string `yaml:"host"`
	Port       int    `yaml:"port"`
	Controller string `yaml:"controller"`
	Radio      Radio  `yaml:"radio"`
}

type Pin struct {
	Name      string `yaml:"name,omitempty"`
	Scheme    string `yaml:"scheme,omitempty"`
	Number    int    `yaml:"number,omitempty"`
	Chip      string `yaml:"chip,omitempty"`
	Offset    int    `yaml:"offset,omitempty"`
	Direction string `yaml:"direction,omitempty"`
	ActiveLow *bool  `yaml:"activeLow,omitempty"`
	Bias      string `yaml:"bias,omitempty"`
	Drive     string `yaml:"drive,omitempty"`
}

type Device struct {
	Name   string         `yaml:"name"`
	Pins   []Pin          `yaml:"pins,omitempty"`
	Config map[string]any `yaml:"config,omitempty"`
}

type Radio struct {
	Name      string `yaml:"name"`
	AlwaysUse bool   `yaml:"alwaysUse"`
	Pins      []Pin  `yaml:"pins,omitempty"`
	UsbId     string `yaml:"usbId"`
}

type DroneConfig struct {
	ID                       int      `yaml:"id"`
	Radio                    Radio    `yaml:"radio"`
	AlwaysUseRadio           bool     `yaml:"alwaysUseRadio"`
	Sensors                  []Device `yaml:"sensors"`
	Outputs                  []Device `yaml:"outputs"`
	ControlAlgorithmPriority []string `yaml:"controlAlgorithmPriority"`
	GPIOLayout               string   `yaml:"gpioLayout"`
}

type Config struct {
	Base  BaseConfig  `yaml:"base"`
	Drone DroneConfig `yaml:"drone"`
}

func GetConfig(file string) Config {
	handle, err := os.Open(file)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open file")
	}
	defer handle.Close()

	content, err := io.ReadAll(handle)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to read file")
	}

	c := Config{}
	err = yaml.Unmarshal(content, &c)
	if err != nil {
		log.Error().Err(err)
	}

	return c
}
