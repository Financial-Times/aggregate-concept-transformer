package ontology

import (
	"encoding/json"
	"sort"
	"strings"
)

const (
	SmartlogicAuthority      = "Smartlogic"
	ManagedLocationAuthority = "ManagedLocation"
)

func CreateAggregatedConcept(sources []SourceConcept) ConcordedConcept {
	var concordedConcept ConcordedConcept
	var scopeNoteOptions = map[string][]string{}
	for _, source := range sources {
		buildScopeNoteOptions(scopeNoteOptions, source)
		concordedConcept = mergeCanonicalInformation(concordedConcept, source)
	}
	concordedConcept.Aliases = deduplicateAndSkipEmptyAliases(concordedConcept.Aliases)
	concordedConcept.ScopeNote = chooseScopeNote(concordedConcept, scopeNoteOptions)
	return concordedConcept
}

func mappify(i interface{}) map[string]interface{} {
	data, _ := json.Marshal(i)
	result := map[string]interface{}{}
	json.Unmarshal(data, &result)
	return result
}
func unmappify(m map[string]interface{}) ConcordedConcept {
	data, _ := json.Marshal(m)
	result := ConcordedConcept{}
	json.Unmarshal(data, &result)
	return result
}

func mergeCanonicalInformation(c ConcordedConcept, sc SourceConcept) ConcordedConcept {
	specialFields := map[string]bool{
		"uuid":    true,
		"aliases": true,
	}
	sources := append(c.SourceRepresentations, sc)
	c.SourceRepresentations = nil // skip transforming sources to json
	if c.Properties == nil {
		c.Properties = map[string]interface{}{}
	}
	if c.Relationships == nil {
		c.Relationships = map[string]interface{}{}
	}
	for label, val := range sc {
		if specialFields[label] {
			continue
		}
		// Currently all properties are just copied from the source to aggregated
		// Should we add aggregate strategy for properties?
		if _, has := GetConfig().FieldToNeoProps[label]; has {
			c.Properties[label] = val
		}

		// Most relationships are just copied over and override the fields
		// Only MembershipRoles and NAICS are aggregated
		// looping over every relation config for every field is slow, maybe create on init map[conceptField]strategy.
		for _, rel := range GetConfig().Relationships {
			if rel.ConceptField != label {
				continue
			}
			switch rel.AggregateStrategy {
			case "aggregate":
				if _, has := c.Relationships[label]; !has {
					c.Relationships[label] = []interface{}{}
				}
				c.Relationships[label] = append(c.Relationships[label].([]interface{}), val.([]interface{})...)
			case "override":
				c.Relationships[label] = val
			}
		}
	}
	s := sc.ToOldSourceConcept()
	c.PrefUUID = s.UUID
	c.PrefLabel = s.PrefLabel
	c.Type = getMoreSpecificType(c.Type, s.Type)
	c.IsDeprecated = s.IsDeprecated
	c.SourceRepresentations = sources
	// []string
	c.Aliases = append(c.Aliases, s.Aliases...)
	c.Aliases = append(c.Aliases, s.PrefLabel)

	if len(s.TradeNames) > 0 {
		c.TradeNames = s.TradeNames
	}
	// string
	if s.Strapline != "" {
		c.Strapline = s.Strapline
	}
	if s.DescriptionXML != "" {
		c.DescriptionXML = s.DescriptionXML
	}
	if s.ImageURL != "" {
		c.ImageURL = s.ImageURL
	}
	if s.EmailAddress != "" {
		c.EmailAddress = s.EmailAddress
	}
	if s.FacebookPage != "" {
		c.FacebookPage = s.FacebookPage
	}
	if s.TwitterHandle != "" {
		c.TwitterHandle = s.TwitterHandle
	}
	if s.ShortLabel != "" {
		c.ShortLabel = s.ShortLabel
	}
	if s.ProperName != "" {
		c.ProperName = s.ProperName
	}
	if s.ShortName != "" {
		c.ShortName = s.ShortName
	}
	if s.CountryCode != "" {
		c.CountryCode = s.CountryCode
	}
	if s.CountryOfIncorporation != "" {
		c.CountryOfIncorporation = s.CountryOfIncorporation
	}
	if s.CountryOfOperations != "" {
		c.CountryOfOperations = s.CountryOfOperations
	}
	if s.PostalCode != "" {
		c.PostalCode = s.PostalCode
	}
	if s.LeiCode != "" {
		c.LeiCode = s.LeiCode
	}
	if s.Salutation != "" {
		c.Salutation = s.Salutation
	}
	if s.ISO31661 != "" {
		c.ISO31661 = s.ISO31661
	}
	if s.InceptionDate != "" {
		c.InceptionDate = s.InceptionDate
	}
	if s.TerminationDate != "" {
		c.TerminationDate = s.TerminationDate
	}
	if s.FigiCode != "" {
		c.FigiCode = s.FigiCode
	}
	if s.IndustryIdentifier != "" {
		c.IndustryIdentifier = s.IndustryIdentifier
	}

	// int
	if s.BirthYear > 0 {
		c.BirthYear = s.BirthYear
	}

	// relations
	if len(s.SupersededByUUIDs) > 0 {
		c.SupersededByUUIDs = s.SupersededByUUIDs
	}
	if len(s.BroaderUUIDs) > 0 {
		c.BroaderUUIDs = s.BroaderUUIDs
	}
	if len(s.RelatedUUIDs) > 0 {
		c.RelatedUUIDs = s.RelatedUUIDs
	}
	for _, mr := range s.MembershipRoles {
		c.MembershipRoles = append(c.MembershipRoles, MembershipRole{
			RoleUUID:        mr.RoleUUID,
			InceptionDate:   mr.InceptionDate,
			TerminationDate: mr.TerminationDate,
		})
	}

	if s.OrganisationUUID != "" {
		c.OrganisationUUID = s.OrganisationUUID
	}
	if s.PersonUUID != "" {
		c.PersonUUID = s.PersonUUID
	}
	if s.IssuedBy != "" {
		c.IssuedBy = s.IssuedBy
	}
	return c
}

func deduplicateAndSkipEmptyAliases(aliases []string) []string {
	aMap := map[string]bool{}
	var outAliases []string
	for _, v := range aliases {
		if v == "" {
			continue
		}
		aMap[v] = true
	}
	for a := range aMap {
		outAliases = append(outAliases, a)
	}
	sort.Strings(outAliases)
	return outAliases
}

func getMoreSpecificType(existingType string, newType string) string {

	// Thing type shouldn't wipe things.
	if newType == "Thing" && existingType != "" {
		return existingType
	}

	// If we've already called it a PublicCompany, keep that information.
	if existingType == "PublicCompany" && (newType == "Organisation" || newType == "Company") {
		return existingType
	}
	return newType
}

func buildScopeNoteOptions(scopeNotes map[string][]string, s SourceConcept) {
	var newScopeNote string
	authority := s.GetStringProperty("authority")
	if authority == "TME" {
		newScopeNote = s.GetStringProperty("prefLabel")
	} else {
		newScopeNote = s.GetStringProperty("scopeNote")
	}
	if newScopeNote != "" {
		scopeNotes[authority] = append(scopeNotes[authority], newScopeNote)
	}
}

func chooseScopeNote(concept ConcordedConcept, scopeNoteOptions map[string][]string) string {
	if sn, ok := scopeNoteOptions[SmartlogicAuthority]; ok {
		return strings.Join(removeMatchingEntries(sn, concept.PrefLabel), " | ")
	}
	if sn, ok := scopeNoteOptions["Wikidata"]; ok {
		return strings.Join(removeMatchingEntries(sn, concept.PrefLabel), " | ")
	}
	if sn, ok := scopeNoteOptions["TME"]; ok {
		if concept.Type == "Location" {
			return strings.Join(removeMatchingEntries(sn, concept.PrefLabel), " | ")
		}
	}
	return ""
}

func removeMatchingEntries(slice []string, matcher string) []string {
	var newSlice []string
	for _, k := range slice {
		if k != matcher {
			newSlice = append(newSlice, k)
		}
	}
	return newSlice
}
