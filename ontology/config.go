package ontology

import (
	"embed"
	"errors"
	"fmt"
	"math"

	"gopkg.in/yaml.v2"
)

type FieldConfig struct {
	NeoProp   string `yaml:"neoProp"`
	FieldType string `yaml:"type"`
}

type RelationshipConfig struct {
	ConceptField    string   `yaml:"conceptField"`
	OneToOne        bool     `yaml:"oneToOne"`
	NeoCreate       bool     `yaml:"neoCreate"`
	Properties      []string `yaml:"properties"`
	ToNodeWithLabel string   `yaml:"toNodeWithLabel"`
}

type Config struct {
	Fields        map[string]FieldConfig        `yaml:"fields"`
	Relationships map[string]RelationshipConfig `yaml:"relationships"`
	Authorities   []string                      `yaml:"authorities"`
}

var ErrUnknownProperty = errors.New("unknown concept property")
var ErrInvalidPropertyValue = errors.New("invalid property value")

func (cfg Config) ValidateProperties(props map[string]interface{}) error {
	for propName, propVal := range props {
		if !cfg.HasField(propName) {
			return fmt.Errorf("propName=%s: %w", propName, ErrUnknownProperty)
		}

		if !cfg.IsPropValueValid(propName, propVal) {
			return fmt.Errorf("propName=%s, value=%v: %w", propName, propVal, ErrInvalidPropertyValue)
		}
	}

	return nil
}

func (cfg Config) HasField(propName string) bool {
	_, has := cfg.Fields[propName]
	return has
}

func (cfg Config) IsPropValueValid(propName string, val interface{}) bool {
	fieldType := cfg.Fields[propName].FieldType
	switch fieldType {
	case "string":
		_, ok := val.(string)
		return ok
	case "[]string":
		_, ok := val.([]string)
		if ok {
			return true
		}

		vs, ok := val.([]interface{}) // []interface{}, for JSON arrays
		if !ok {
			return false
		}

		for _, v := range vs {
			_, ok := v.(string)
			if !ok {
				return false
			}
		}

		return true
	case "int":
		_, ok := val.(int)
		if ok {
			return true
		}

		floatVal, ok := val.(float64) // float64, for JSON numbers
		if !ok {
			return false
		}

		isWholeInteger := floatVal == math.Trunc(floatVal)
		return isWholeInteger
	default:
		return false
	}
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
