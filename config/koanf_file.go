package config

import (
	"fmt"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

func loadYAMLWithKoanf(path string, cfg *Config) error {
	if cfg == nil {
		return nil
	}

	k := koanf.New(".")
	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		return fmt.Errorf("parsing config file %s: %w", path, err)
	}

	if err := unmarshalConfigWithKoanf(k, cfg); err != nil {
		return fmt.Errorf("parsing config file %s: %w", path, err)
	}
	return nil
}

func unmarshalConfigWithKoanf(k *koanf.Koanf, cfg *Config) error {
	if cfg == nil {
		return nil
	}
	if err := k.UnmarshalWithConf("", cfg, koanf.UnmarshalConf{Tag: "yaml"}); err != nil {
		return fmt.Errorf("unmarshalling config: %w", err)
	}
	return nil
}
