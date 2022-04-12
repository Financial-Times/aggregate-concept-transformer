package ontology

import (
	"encoding/json"
	"fmt"
)

type Relationship struct {
	UUID       string                 `json:"uuid"`
	Label      string                 `json:"label"`
	Properties map[string]interface{} `json:"properties"`
}

type Relationships []Relationship

func (r Relationships) MarshalJSON() ([]byte, error) {
	result := map[string]interface{}{}
	for _, rel := range r {
		if rel.UUID == "" {
			continue
		}

		cfg, ok := GetConfig().Relationships[rel.Label]
		if !ok {
			continue
		}
		relAny := writeRelationshipsToAny(cfg, rel, result[cfg.ConceptField])
		result[cfg.ConceptField] = relAny
	}

	return json.Marshal(result)
}

func (r *Relationships) UnmarshalJSON(bytes []byte) error {
	oldMap := map[string]interface{}{}
	if err := json.Unmarshal(bytes, &oldMap); err != nil {
		return err
	}
	for label, cfg := range GetConfig().Relationships {
		if _, ok := oldMap[cfg.ConceptField]; !ok {
			continue
		}

		val := oldMap[cfg.ConceptField]

		rel, err := readRelationshipsFromAny(label, cfg, val)
		if err != nil {
			return err
		}
		*r = append(*r, rel...)
	}
	return nil
}

func writeRelationshipsToAny(cfg RelationshipConfig, rel Relationship, prev interface{}) interface{} {
	if cfg.OneToOne {
		return rel.UUID
	}

	if prev == nil {
		if len(cfg.Properties) == 0 {
			return []string{rel.UUID}
		}

		relProps := rel.Properties
		if relProps == nil {
			// relProps should never be nil, but if it is, ensure that we will not panic.
			relProps = map[string]interface{}{}
		}
		uuidKey := GetConfig().GetRelationshipUUIDKey(rel.Label)
		relProps[uuidKey] = rel.UUID

		return []map[string]interface{}{relProps}
	}

	if len(cfg.Properties) == 0 {
		relUUIDs := prev.([]string)
		relUUIDs = append(relUUIDs, rel.UUID)
		return relUUIDs
	}

	relProps := rel.Properties
	uuidKey := GetConfig().GetRelationshipUUIDKey(rel.Label)
	relProps[uuidKey] = rel.UUID

	rels := prev.([]map[string]interface{})
	rels = append(rels, relProps)
	return rels
}

func readRelationshipsFromAny(label string, cfg RelationshipConfig, val interface{}) (Relationships, error) {
	if cfg.OneToOne {
		uuid, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("failed to cast '%v' to string for relationship '%s' uuid", val, label)
		}
		return Relationships{{UUID: uuid, Label: label}}, nil
	}

	result := Relationships{}
	valSlice, ok := val.([]interface{})
	if !ok {
		return nil, fmt.Errorf("failed to cast '%v' to slice for relationships '%s' ", val, label)
	}

	if len(cfg.Properties) == 0 {
		for _, v := range valSlice {
			uuid, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("failed to cast '%v' to string for relationships '%s' uuid", v, label)
			}
			result = append(result, Relationship{UUID: uuid, Label: label})
		}
		return result, nil
	}

	for _, v := range valSlice {
		// extract uuid
		uuid, props, ok := extractUUIDAndProps(label, v)
		if !ok {
			continue // TODO: change to error?
		}
		props, err := readRelationshipProps(cfg, props)
		if err != nil {
			return nil, fmt.Errorf("failed to read relationship '%s' props: %w", label, err)
		}
		result = append(result, Relationship{UUID: uuid, Label: label, Properties: props})
	}
	return result, nil
}

func extractUUIDAndProps(label string, v interface{}) (string, map[string]interface{}, bool) {
	props, ok := v.(map[string]interface{})
	if !ok {
		return "", nil, false
	}
	uuidKey := GetConfig().GetRelationshipUUIDKey(label)
	uuidI, ok := props[uuidKey]
	if !ok {
		return "", nil, false
	}
	uuid, ok := uuidI.(string)
	if !ok {
		return "", nil, false
	}
	delete(props, uuidKey)
	return uuid, props, true
}

func readRelationshipProps(cfg RelationshipConfig, props map[string]interface{}) (map[string]interface{}, error) {
	result := map[string]interface{}{}
	for field, fieldType := range cfg.Properties {
		var v interface{}
		val, ok := props[field]
		if !ok {
			continue
		}

		switch fieldType {
		case "date":
			if v, ok = toString(val); !ok {
				return nil, InvalidPropValueError(field, v)
			}
		case "string":
			if v, ok = toString(val); !ok {
				return nil, InvalidPropValueError(field, v)
			}
		case "[]string":
			if v, ok = toStringSlice(val); !ok {
				return nil, InvalidPropValueError(field, v)
			}
		case "int":
			if v, ok = toInt(val); !ok {
				return nil, InvalidPropValueError(field, v)
			}
		default:
			return nil,
				fmt.Errorf("unsupported field type '%s' for prop '%s': %w", fieldType, field, ErrUnknownProperty)
		}
		result[field] = v
	}
	return result, nil
}

func toString(val interface{}) (string, bool) {
	str, ok := val.(string)
	return str, ok
}

func toInt(val interface{}) (int, bool) {
	switch v := val.(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

func toStringSlice(val interface{}) ([]string, bool) {
	if vs, ok := val.([]string); ok {
		return vs, ok
	}
	vs, ok := val.([]interface{})
	if !ok {
		return nil, false
	}
	var result []string
	for _, v := range vs {
		if str, ok := v.(string); ok {
			result = append(result, str)
		}
	}
	if len(result) != len(vs) {
		return nil, false
	}
	return result, true
}
