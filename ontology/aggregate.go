package ontology

import (
	"strings"
)

const (
	SmartlogicAuthority      = "Smartlogic"
	ManagedLocationAuthority = "ManagedLocation"
)

// CreateAggregateConcept creates ConcordedConcept by merging properties and relationships from primary and others SourceConcept
// When merging the data from the primary SourceConcept takes precedent.
// So if a property is present in both "primary" and one or more "other" SourceConcept,
// the data from the primary will be in the ConcordedConcept
// Exception to this rule are relationships that are with MergingStrategy: AggregateStrategy.
// Those relationships will be collected from both primary and others SourceConcept into ConcordedConcept
func CreateAggregateConcept(primary SourceConcept, others []SourceConcept) ConcordedConcept {
	var scopeNoteOptions = map[string][]string{}
	concordedConcept := ConcordedConcept{}
	concordedConcept.Fields = map[string]interface{}{} // initialise Fields to be able to safely access it later
	for _, src := range others {
		concordedConcept = mergeCanonicalInformation(concordedConcept, src, scopeNoteOptions)
	}

	concordedConcept = mergeCanonicalInformation(concordedConcept, primary, scopeNoteOptions)
	concordedConcept.Aliases = deduplicateAndSkipEmptyAliases(concordedConcept.Aliases)
	concordedConcept.ScopeNote = chooseScopeNote(concordedConcept, scopeNoteOptions)
	return concordedConcept
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
	if s.Authority == "TME" {
		newScopeNote = s.PrefLabel
	} else {
		newScopeNote = s.ScopeNote
	}
	if newScopeNote != "" {
		scopeNotes[s.Authority] = append(scopeNotes[s.Authority], newScopeNote)
	}
}

// nolint:gocognit // in the process of simplifying this function
func mergeCanonicalInformation(c ConcordedConcept, s SourceConcept, scopeNoteOptions map[string][]string) ConcordedConcept {
	c.PrefUUID = s.UUID
	c.PrefLabel = s.PrefLabel
	c.Type = getMoreSpecificType(c.Type, s.Type)
	c.Aliases = append(c.Aliases, s.Aliases...)
	c.Aliases = append(c.Aliases, s.PrefLabel)

	for key, val := range s.Fields {
		if GetConfig().HasField(key) {
			c.Fields[key] = val
			continue
		}

		switch GetConfig().MergingStrategies[key] {
		case OverwriteStrategy:
			c.Fields[key] = val
		case AggregateStrategy:
			if _, has := c.Fields[key]; !has {
				c.Fields[key] = []interface{}{}
			}
			// TODO: test casting
			c.Fields[key] = append(c.Fields[key].([]interface{}), val.([]interface{})...)
		}
	}

	buildScopeNoteOptions(scopeNoteOptions, s)
	if len(s.SupersededByUUIDs) > 0 {
		c.SupersededByUUIDs = s.SupersededByUUIDs
	}
	if len(s.ParentUUIDs) > 0 {
		c.ParentUUIDs = s.ParentUUIDs
	}
	if len(s.BroaderUUIDs) > 0 {
		c.BroaderUUIDs = s.BroaderUUIDs
	}
	if len(s.RelatedUUIDs) > 0 {
		c.RelatedUUIDs = s.RelatedUUIDs
	}
	c.SourceRepresentations = append(c.SourceRepresentations, s)
	if s.ProperName != "" {
		c.ProperName = s.ProperName
	}
	if s.ShortName != "" {
		c.ShortName = s.ShortName
	}
	if len(s.TradeNames) > 0 {
		c.TradeNames = s.TradeNames
	}
	if len(s.FormerNames) > 0 {
		c.FormerNames = s.FormerNames
	}
	if s.CountryCode != "" {
		c.CountryCode = s.CountryCode
	}
	if s.CountryOfRisk != "" {
		c.CountryOfRisk = s.CountryOfRisk
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
	if s.YearFounded > 0 {
		c.YearFounded = s.YearFounded
	}
	if s.LeiCode != "" {
		c.LeiCode = s.LeiCode
	}
	if s.ISO31661 != "" {
		c.ISO31661 = s.ISO31661
	}

	for _, mr := range s.MembershipRoles {
		c.MembershipRoles = append(c.MembershipRoles, MembershipRole{
			RoleUUID:        mr.RoleUUID,
			InceptionDate:   mr.InceptionDate,
			TerminationDate: mr.TerminationDate,
		})
	}

	for _, ic := range s.NAICSIndustryClassifications {
		c.NAICSIndustryClassifications = append(c.NAICSIndustryClassifications, NAICSIndustryClassification{
			UUID: ic.UUID,
			Rank: ic.Rank,
		})
	}

	if s.OrganisationUUID != "" {
		c.OrganisationUUID = s.OrganisationUUID
	}
	if s.PersonUUID != "" {
		c.PersonUUID = s.PersonUUID
	}
	if s.FigiCode != "" {
		c.FigiCode = s.FigiCode
	}
	if s.IssuedBy != "" {
		c.IssuedBy = s.IssuedBy
	}
	if s.IndustryIdentifier != "" {
		c.IndustryIdentifier = s.IndustryIdentifier
	}
	c.IsDeprecated = s.IsDeprecated
	return c
}
