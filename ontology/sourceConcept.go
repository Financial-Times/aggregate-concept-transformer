package ontology

import (
	"encoding/json"
)

type NewConcept struct {
	RequiredSourceFields
	AdditionalSourceFields
}

type RequiredSourceFields struct {
	UUID              string `json:"uuid"`
	Type              string `json:"type"`
	PrefLabel         string `json:"prefLabel"`
	Authority         string `json:"authority"`
	AuthorityValue    string `json:"authorityValue"`
	LastModifiedEpoch int    `json:"lastModifiedEpoch,omitempty"`
	Hash              string `json:"hash,omitempty"`
}

type AdditionalSourceFields struct {
	Properties    map[string]interface{} `json:"-"`
	Relationships Relationships          `json:"-"`
	// Additional fields
	Aliases   []string `json:"aliases,omitempty"`
	ScopeNote string   `json:"scopeNote,omitempty"`
	// Financial Instrument
	FigiCode string `json:"figiCode,omitempty"`
	IssuedBy string `json:"issuedBy,omitempty"`
	// Organisation
	IsDeprecated bool `json:"isDeprecated,omitempty"`
}

func (sc *NewConcept) MarshalJSON() ([]byte, error) {
	req, err := mappify(sc.RequiredSourceFields)
	if err != nil {
		return nil, err
	}
	add, err := mappify(sc.AdditionalSourceFields)
	if err != nil {
		return nil, err
	}
	rels, err := mappify(&sc.Relationships)
	if err != nil {
		return nil, err
	}
	result := map[string]interface{}{}
	// TODO: ensure that fields are not overlapping
	for key, val := range sc.Properties {
		// serialize only fields defined in the config
		if !GetConfig().HasProperty(key) {
			continue
		}
		result[key] = val
	}

	for key, val := range rels {
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

func (sc *NewConcept) UnmarshalJSON(bytes []byte) error {
	err := json.Unmarshal(bytes, &sc.RequiredSourceFields)
	if err != nil {
		return err
	}
	err = json.Unmarshal(bytes, &sc.AdditionalSourceFields)
	if err != nil {
		return err
	}
	err = json.Unmarshal(bytes, &sc.Relationships)
	if err != nil {
		return err
	}
	fields := map[string]interface{}{}
	err = json.Unmarshal(bytes, &fields)
	if err != nil {
		return err
	}
	sc.Properties = map[string]interface{}{}

	for key := range GetConfig().Properties {
		val, has := fields[key]
		if !has {
			continue
		}
		sc.Properties[key] = val
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
