package ontology

import (
	"encoding/json"
)

type SourceConcept struct {
	RequiredSourceFields
	AdditionalSourceFields
}

type RequiredSourceFields struct {
	UUID      string `json:"uuid,omitempty"`
	Type      string `json:"type,omitempty"`
	PrefLabel string `json:"prefLabel,omitempty"`
	Authority string `json:"authority,omitempty"`
	AuthValue string `json:"authorityValue,omitempty"`
}

type AdditionalSourceFields struct {
	Fields map[string]interface{} `json:"-"`
	// Additional fields
	Aliases   []string `json:"aliases,omitempty"`
	ScopeNote string   `json:"scopeNote,omitempty"`
	// Financial Instrument
	FigiCode string `json:"figiCode,omitempty"`
	IssuedBy string `json:"issuedBy,omitempty"`
	// Organisation
	IsDeprecated bool `json:"isDeprecated,omitempty"`
}

func (sc *SourceConcept) MarshalJSON() ([]byte, error) {
	req, err := mappify(sc.RequiredSourceFields)
	if err != nil {
		return nil, err
	}
	add, err := mappify(sc.AdditionalSourceFields)
	if err != nil {
		return nil, err
	}
	result := map[string]interface{}{}
	// TODO: ensure that fields are not overlapping
	for key, val := range sc.Fields {
		// serialize only fields defined in the config
		if !GetConfig().HasField(key) && !GetConfig().HasRelationship(key) {
			continue
		}
		result[key] = val
	}
	for key, val := range add {
		result[key] = val
	}
	for key, val := range req {
		result[key] = val
	}
	return json.Marshal(result)
}

func (sc *SourceConcept) UnmarshalJSON(bytes []byte) error {
	err := json.Unmarshal(bytes, &sc.RequiredSourceFields)
	if err != nil {
		return err
	}
	err = json.Unmarshal(bytes, &sc.AdditionalSourceFields)
	if err != nil {
		return err
	}
	fields := map[string]interface{}{}
	err = json.Unmarshal(bytes, &fields)
	if err != nil {
		return err
	}
	sc.Fields = map[string]interface{}{}
	for key := range GetConfig().Fields {
		val, has := fields[key]
		if !has {
			continue
		}
		sc.Fields[key] = val
	}
	for _, rel := range GetConfig().Relationships {
		val, has := fields[rel.ConceptField]
		if !has {
			continue
		}
		sc.Fields[rel.ConceptField] = val
	}
	return nil
}

func mappify(source interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(source)
	if err != nil {
		return nil, err
	}
	result := map[string]interface{}{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}
