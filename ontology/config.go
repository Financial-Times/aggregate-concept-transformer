package ontology

import (
	"embed"

	"gopkg.in/yaml.v2"
)

type RelationshipConfig struct {
	ConceptField      string   `yaml:"conceptField"`
	OneToOne          bool     `yaml:"oneToOne"`
	NeoCreate         bool     `yaml:"neoCreate"`
	Properties        []string `yaml:"properties"`
	ToNodeWithLabel   string   `yaml:"toNodeWithLabel"`
	AggregateStrategy string   `yaml:"aggregate_strategy"`
}

type Config struct {
	FieldToNeoProps map[string]string             `yaml:"fieldToNeoProps"`
	Relationships   map[string]RelationshipConfig `yaml:"relationships"`
}

var config Config

//go:embed config.yml
var f embed.FS

func init() {
	bytes, err := f.ReadFile("config.yml")
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
