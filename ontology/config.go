package ontology

import (
	"embed"

	"gopkg.in/yaml.v2"
)

type RelationshipConfig struct {
	ConceptField    string   `yaml:"conceptField"`
	OneToOne        bool     `yaml:"oneToOne"`
	NeoCreate       bool     `yaml:"neoCreate"`
	Properties      []string `yaml:"properties"`
	ToNodeWithLabel string   `yaml:"toNodeWithLabel"`
}

type Config struct {
	FieldToNeoProps map[string]string             `yaml:"fieldToNeoProps"`
	Relationships   map[string]RelationshipConfig `yaml:"relationships"`
}

var config Config

//go:embed config.yaml
var f embed.FS

func init() {
	bytes, err := f.ReadFile("config.yaml")
	if err != nil {
		panic(err)
	}

	err = yaml.Unmarshal(bytes, &config)
	if err != nil {
		panic(err)
	}
}

func GetConfig() Config {
	return config
}

func setGlobalConfig(cfg Config) {
	config = cfg
}
