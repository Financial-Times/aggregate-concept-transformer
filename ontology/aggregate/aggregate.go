package aggregate

import (
	"strings"

	"github.com/Financial-Times/aggregate-concept-transformer/ontology"
)

// CreateAggregateConcept creates NewAggregatedConcept by merging properties and relationships from primary and others NewConcept
// When merging the data from the primary NewConcept takes precedent.
// So if a property is present in both "primary" and one or more "other" NewConcept,
// the data from the primary will be in the NewAggregatedConcept
// Exception to this rule are relationships that are with MergingStrategy: AggregateStrategy.
// Those relationships will be collected from both primary and others NewConcept into NewAggregatedConcept
func CreateAggregateConcept(primary ontology.NewConcept, others []ontology.NewConcept) ontology.NewAggregatedConcept {
	var scopeNoteOptions = map[string][]string{}
	concordedConcept := ontology.NewAggregatedConcept{}
	concordedConcept.Fields = map[string]interface{}{} // initialise Fields to be able to safely access it later
	for _, src := range others {
		concordedConcept = mergeCanonicalInformation(concordedConcept, src, scopeNoteOptions)
	}

	concordedConcept = mergeCanonicalInformation(concordedConcept, primary, scopeNoteOptions)
	concordedConcept.Aliases = deduplicateAndSkipEmptyAliases(concordedConcept.Aliases)
	concordedConcept.ScopeNote = chooseScopeNote(concordedConcept, scopeNoteOptions)
	return concordedConcept
}

func chooseScopeNote(concept ontology.NewAggregatedConcept, scopeNoteOptions map[string][]string) string {
	if sn, ok := scopeNoteOptions[ontology.SmartlogicAuthority]; ok {
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

func buildScopeNoteOptions(scopeNotes map[string][]string, s ontology.NewConcept) {
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
func mergeCanonicalInformation(c ontology.NewAggregatedConcept, s ontology.NewConcept, scopeNoteOptions map[string][]string) ontology.NewAggregatedConcept {
	c.PrefUUID = s.UUID
	c.PrefLabel = s.PrefLabel
	c.Type = getMoreSpecificType(c.Type, s.Type)
	c.Aliases = append(c.Aliases, s.Aliases...)
	c.Aliases = append(c.Aliases, s.PrefLabel)

	for key, val := range s.Fields {
		if ontology.GetConfig().HasProperty(key) {
			c.Fields[key] = val
			continue
		}

		switch ontology.GetConfig().MergingStrategies[key] {
		case ontology.OverwriteStrategy:
			c.Fields[key] = val
		case ontology.AggregateStrategy:
			if _, has := c.Fields[key]; !has {
				c.Fields[key] = []interface{}{}
			}
			// TODO: test casting
			c.Fields[key] = append(c.Fields[key].([]interface{}), val.([]interface{})...)
		}
	}

	buildScopeNoteOptions(scopeNoteOptions, s)
	c.SourceRepresentations = append(c.SourceRepresentations, s)

	if s.FigiCode != "" {
		c.FigiCode = s.FigiCode
	}
	if s.IssuedBy != "" {
		c.IssuedBy = s.IssuedBy
	}
	c.IsDeprecated = s.IsDeprecated
	return c
}
