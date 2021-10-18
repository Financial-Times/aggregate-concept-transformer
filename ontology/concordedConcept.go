package ontology

import (
	"encoding/json"
)

// ConcordedConcept is the model of the aggregated concept that is send for storage in the knowledge graph.
type ConcordedConcept struct {
	RequiredConcordedFields
	AdditionalConcordedFields
}

type RequiredConcordedFields struct {
	// Required fields
	PrefUUID  string `json:"prefUUID,omitempty"`
	PrefLabel string `json:"prefLabel,omitempty"`
	Type      string `json:"type,omitempty"`
}

type AdditionalConcordedFields struct {
	Fields map[string]interface{} `json:"-"`
	// Additional fields
	Aliases           []string `json:"aliases,omitempty"`
	ParentUUIDs       []string `json:"parentUUIDs,omitempty"`
	BroaderUUIDs      []string `json:"broaderUUIDs,omitempty"`
	RelatedUUIDs      []string `json:"relatedUUIDs,omitempty"`
	SupersededByUUIDs []string `json:"supersededByUUIDs,omitempty"`
	ScopeNote         string   `json:"scopeNote,omitempty"`
	// Financial Instrument
	FigiCode string `json:"figiCode,omitempty"`
	IssuedBy string `json:"issuedBy,omitempty"`
	// Membership
	MembershipRoles  []MembershipRole `json:"membershipRoles,omitempty"`
	OrganisationUUID string           `json:"organisationUUID,omitempty"`
	PersonUUID       string           `json:"personUUID,omitempty"`
	// Organisation
	LeiCode                      string                        `json:"leiCode,omitempty"`
	PostalCode                   string                        `json:"postalCode,omitempty"`
	ProperName                   string                        `json:"properName,omitempty"`
	ShortName                    string                        `json:"shortName,omitempty"`
	YearFounded                  int                           `json:"yearFounded,omitempty"`
	IsDeprecated                 bool                          `json:"isDeprecated,omitempty"`
	NAICSIndustryClassifications []NAICSIndustryClassification `json:"naicsIndustryClassifications,omitempty"`
	// Location
	ISO31661 string `json:"iso31661,omitempty"`
	// IndustryClassification
	IndustryIdentifier string `json:"industryIdentifier,omitempty"`
	// Source representations
	SourceRepresentations []SourceConcept `json:"sourceRepresentations,omitempty"`
}

func (cc *ConcordedConcept) MarshalJSON() ([]byte, error) {
	req, err := mappify(cc.RequiredConcordedFields)
	if err != nil {
		return nil, err
	}
	add, err := mappify(cc.AdditionalConcordedFields)
	if err != nil {
		return nil, err
	}
	result := map[string]interface{}{}
	// TODO: ensure that fields are not overlapping
	for key, val := range cc.Fields {
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

func (cc *ConcordedConcept) UnmarshalJSON(bytes []byte) error {
	err := json.Unmarshal(bytes, &cc.RequiredConcordedFields)
	if err != nil {
		return err
	}
	err = json.Unmarshal(bytes, &cc.AdditionalConcordedFields)
	if err != nil {
		return err
	}
	fields := map[string]interface{}{}
	err = json.Unmarshal(bytes, &fields)
	if err != nil {
		return err
	}
	cc.Fields = map[string]interface{}{}
	for key := range GetConfig().Fields {
		val, has := fields[key]
		if !has {
			continue
		}
		cc.Fields[key] = val
	}

	for _, rel := range GetConfig().Relationships {
		val, has := fields[rel.ConceptField]
		if !has {
			continue
		}
		cc.Fields[rel.ConceptField] = val
	}
	return nil
}
