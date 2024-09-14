package config

import (
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"io"
	"os"
)

type Config struct {
	LogLevel string `yaml:"logLevel"`
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
