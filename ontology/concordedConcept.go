package ontology

import (
	"encoding/json"
)

// NewAggregatedConcept is the model of the aggregated concept that is send for storage in the knowledge graph.
type NewAggregatedConcept struct {
	RequiredConcordedFields
	AdditionalConcordedFields
}

type RequiredConcordedFields struct {
	// Required fields
	PrefUUID       string `json:"prefUUID,omitempty"`
	PrefLabel      string `json:"prefLabel,omitempty"`
	Type           string `json:"type,omitempty"`
	AggregatedHash string `json:"aggregateHash,omitempty"`
	// Source representations
	SourceRepresentations []NewConcept `json:"sourceRepresentations,omitempty"`
}

type AdditionalConcordedFields struct {
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

func (cc *NewAggregatedConcept) MarshalJSON() ([]byte, error) {
	req, err := mappify(cc.RequiredConcordedFields)
	if err != nil {
		return nil, err
	}
	add, err := mappify(cc.AdditionalConcordedFields)
	if err != nil {
		return nil, err
	}
	rels, err := mappify(&cc.Relationships)
	if err != nil {
		return nil, err
	}
	result := map[string]interface{}{}
	// TODO: ensure that fields are not overlapping
	for key, val := range cc.Properties {
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

func (cc *NewAggregatedConcept) UnmarshalJSON(bytes []byte) error {
	err := json.Unmarshal(bytes, &cc.RequiredConcordedFields)
	if err != nil {
		return err
	}
	err = json.Unmarshal(bytes, &cc.AdditionalConcordedFields)
	if err != nil {
		return err
	}
	err = json.Unmarshal(bytes, &cc.Relationships)
	if err != nil {
		return err
	}
	fields := map[string]interface{}{}
	err = json.Unmarshal(bytes, &fields)
	if err != nil {
		return err
	}
	cc.Properties = map[string]interface{}{}
	for key := range GetConfig().Properties {
		val, has := fields[key]
		if !has {
			continue
		}
		cc.Properties[key] = val
	}
	return nil
}
