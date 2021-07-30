package ontology

import (
	"encoding/json"
)

type MembershipRole struct {
	RoleUUID        string `json:"membershipRoleUUID,omitempty"`
	InceptionDate   string `json:"inceptionDate,omitempty"`
	TerminationDate string `json:"terminationDate,omitempty"`
}

type NAICSIndustryClassification struct {
	UUID string `json:"uuid,omitempty"`
	Rank int    `json:"rank,omitempty"`
}

// ConcordedConcept is the data model of the concept send for serialization to the writer service
type ConcordedConcept struct {
	Properties    map[string]interface{} `json:"-"`
	Relationships map[string]interface{} `json:"-"`
	// Required fields
	PrefUUID  string `json:"prefUUID,omitempty"`
	PrefLabel string `json:"prefLabel,omitempty"`
	Type      string `json:"type,omitempty"`
	// Additional fields
	Aliases           []string `json:"aliases,omitempty"`
	ParentUUIDs       []string `json:"parentUUIDs,omitempty"`
	BroaderUUIDs      []string `json:"broaderUUIDs,omitempty"`
	RelatedUUIDs      []string `json:"relatedUUIDs,omitempty"`
	SupersededByUUIDs []string `json:"supersededByUUIDs,omitempty"`
	DescriptionXML    string   `json:"descriptionXML,omitempty"`
	ImageURL          string   `json:"_imageUrl,omitempty"`
	EmailAddress      string   `json:"emailAddress,omitempty"`
	FacebookPage      string   `json:"facebookPage,omitempty"`
	TwitterHandle     string   `json:"twitterHandle,omitempty"`
	ScopeNote         string   `json:"scopeNote,omitempty"`
	ShortLabel        string   `json:"shortLabel,omitempty"`
	// Brand
	Strapline string `json:"strapline,omitempty"`
	// Person
	Salutation string `json:"salutation,omitempty"`
	BirthYear  int    `json:"birthYear,omitempty"`
	// Financial Instrument
	FigiCode string `json:"figiCode,omitempty"`
	IssuedBy string `json:"issuedBy,omitempty"`
	// Membership
	InceptionDate    string           `json:"inceptionDate,omitempty"`
	MembershipRoles  []MembershipRole `json:"membershipRoles,omitempty"`
	OrganisationUUID string           `json:"organisationUUID,omitempty"`
	PersonUUID       string           `json:"personUUID,omitempty"`
	TerminationDate  string           `json:"terminationDate,omitempty"`
	// Organisation
	CountryCode                  string                        `json:"countryCode,omitempty"`
	CountryOfRisk                string                        `json:"countryOfRisk,omitempty"`
	CountryOfIncorporation       string                        `json:"countryOfIncorporation,omitempty"`
	CountryOfOperations          string                        `json:"countryOfOperations,omitempty"`
	FormerNames                  []string                      `json:"formerNames,omitempty"`
	TradeNames                   []string                      `json:"tradeNames,omitempty"`
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

func (cc ConcordedConcept) ToGeneric() (map[string]interface{}, error) {
	data, err := json.Marshal(cc)
	if err != nil {
		return nil, err
	}
	var m map[string]interface{}
	err = json.Unmarshal(data, &m)
	if err != nil {
		return nil, err
	}
	for k, v := range cc.Properties {
		m[k] = v
	}
	for k, v := range cc.Relationships {
		m[k] = v
	}
	return m, nil
}

type SourceConcept map[string]interface{}

func (sc SourceConcept) ToOldSourceConcept() OldSourceConcept {
	var result OldSourceConcept
	data, _ := json.Marshal(sc)
	json.Unmarshal(data, &result)
	return result
}

func (sc SourceConcept) GetStringProperty(field string) string {
	val, has := sc[field]
	if !has {
		return ""
	}
	str, ok := val.(string)
	if !ok {
		return ""
	}
	return str
}

// SourceConcept is the data model for concepts stored in the Normalized store
type OldSourceConcept struct {
	// Required fields
	UUID      string `json:"uuid,omitempty"`
	Type      string `json:"type,omitempty"`
	PrefLabel string `json:"prefLabel,omitempty"`
	Authority string `json:"authority,omitempty"`
	AuthValue string `json:"authorityValue,omitempty"`
	// Additional fields
	Aliases           []string `json:"aliases,omitempty"`
	ParentUUIDs       []string `json:"parentUUIDs,omitempty"`
	BroaderUUIDs      []string `json:"broaderUUIDs,omitempty"`
	RelatedUUIDs      []string `json:"relatedUUIDs,omitempty"`
	SupersededByUUIDs []string `json:"supersededByUUIDs,omitempty"`
	ImpliedByUUIDs    []string `json:"impliedByUUIDs,omitempty"`
	HasFocusUUIDs     []string `json:"hasFocusUUIDs,omitempty"`
	DescriptionXML    string   `json:"descriptionXML,omitempty"`
	ImageURL          string   `json:"_imageUrl,omitempty"`
	EmailAddress      string   `json:"emailAddress,omitempty"`
	FacebookPage      string   `json:"facebookPage,omitempty"`
	TwitterHandle     string   `json:"twitterHandle,omitempty"`
	ScopeNote         string   `json:"scopeNote,omitempty"`
	ShortLabel        string   `json:"shortLabel,omitempty"`
	// Brand
	Strapline string `json:"strapline,omitempty"`
	// Person
	Salutation string `json:"salutation,omitempty"`
	BirthYear  int    `json:"birthYear,omitempty"`
	// Financial Instrument
	FigiCode string `json:"figiCode,omitempty"`
	IssuedBy string `json:"issuedBy,omitempty"`
	// Membership
	InceptionDate    string           `json:"inceptionDate,omitempty"`
	MembershipRoles  []MembershipRole `json:"membershipRoles,omitempty"`
	OrganisationUUID string           `json:"organisationUUID,omitempty"`
	PersonUUID       string           `json:"personUUID,omitempty"`
	TerminationDate  string           `json:"terminationDate,omitempty"`
	// Organisation
	CountryCode                  string                        `json:"countryCode,omitempty"`
	CountryOfRisk                string                        `json:"countryOfRisk,omitempty"`
	CountryOfIncorporation       string                        `json:"countryOfIncorporation,omitempty"`
	CountryOfOperations          string                        `json:"countryOfOperations,omitempty"`
	CountryOfRiskUUID            string                        `json:"countryOfRiskUUID,omitempty"`
	CountryOfIncorporationUUID   string                        `json:"countryOfIncorporationUUID,omitempty"`
	CountryOfOperationsUUID      string                        `json:"countryOfOperationsUUID,omitempty"`
	FormerNames                  []string                      `json:"formerNames,omitempty"`
	TradeNames                   []string                      `json:"tradeNames,omitempty"`
	LeiCode                      string                        `json:"leiCode,omitempty"`
	ParentOrganisation           string                        `json:"parentOrganisation,omitempty"`
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
}
