package ontology

import (
	"encoding/json"
	"os"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCreateAggregateConcept(t *testing.T) {
	tests := map[string]struct {
		Sources   string
		Aggregate string
	}{
		"bulgaria": {
			Sources:   "testdata/sources.json",
			Aggregate: "testdata/aggregate.json",
		},
		"simple-relationships-overwrite": {
			Sources:   "testdata/simple-relationships-sources.json",
			Aggregate: "testdata/simple-relationships-aggregate.json",
		},
		"complex-relationships": {
			Sources:   "testdata/complex-relationships-sources.json",
			Aggregate: "testdata/complex-relationships-aggregate.json",
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			sources := readSourcesFixture(t, test.Sources)
			expected := readAggregateFixture(t, test.Aggregate)
			primary := sources[len(sources)-1]
			actual := CreateAggregateConcept(primary, sources[:len(sources)-1])
			sortAliases(&expected)
			sortAliases(&actual)
			if !cmp.Equal(expected, actual) {
				diff := cmp.Diff(expected, actual)
				t.Fatal(diff)
			}
		})
	}
}

func TestCreateAggregateConcept_Properties(t *testing.T) {
	tests := map[string]struct {
		Primary SourceConcept
		Sources []SourceConcept
	}{
		"Properties": {
			Primary: SourceConcept{AdditionalSourceFields: AdditionalSourceFields{Fields: map[string]interface{}{
				"descriptionXML":         "primary description",
				"_imageUrl":              "primary image",
				"emailAddress":           "primary emailAddress",
				"facebookPage":           "primary facebookPage",
				"twitterHandle":          "primary twitterHandle",
				"shortLabel":             "primary shortLabel",
				"strapline":              "primary strapline",
				"salutation":             "primary salutation",
				"birthYear":              1,
				"inceptionDate":          "primary inceptionDate",
				"terminationDate":        "primary terminationDate",
				"countryCode":            "primary countryCode",
				"countryOfRisk":          "primary countryOfRisk",
				"countryOfIncorporation": "primary countryOfIncorporation",
				"countryOfOperations":    "primary countryOfOperations",
				"formerNames":            []string{"primary formerNames"},
				"tradeNames":             []string{"primary tradeNames"},
				"leiCode":                "primary leiCode",
				"postalCode":             "primary postalCode",
				"properName":             "primary properName",
				"shortName":              "primary shortName",
				"yearFounded":            1,
				"iso31661":               "primary iso31661",
				"industryIdentifier":     "primary industryIdentifier",
			}},
			},
			Sources: []SourceConcept{
				{AdditionalSourceFields: AdditionalSourceFields{Fields: map[string]interface{}{
					"descriptionXML":         "secondary description",
					"_imageUrl":              "secondary image",
					"emailAddress":           "secondary emailAddress",
					"facebookPage":           "secondary facebookPage",
					"twitterHandle":          "secondary twitterHandle",
					"shortLabel":             "secondary shortLabel",
					"strapline":              "secondary strapline",
					"salutation":             "secondary salutation",
					"birthYear":              2,
					"inceptionDate":          "secondary inceptionDate",
					"terminationDate":        "secondary terminationDate",
					"countryCode":            "secondary countryCode",
					"countryOfRisk":          "secondary countryOfRisk",
					"countryOfIncorporation": "secondary countryOfIncorporation",
					"countryOfOperations":    "secondary countryOfOperations",
					"formerNames":            []string{"secondary formerNames"},
					"tradeNames":             []string{"secondary tradeNames"},
					"leiCode":                "secondary leiCode",
					"postalCode":             "secondary postalCode",
					"properName":             "secondary properName",
					"shortName":              "secondary shortName",
					"yearFounded":            2,
					"iso31661":               "secondary iso31661",
					"industryIdentifier":     "secondary industryIdentifier",
				}}},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual := CreateAggregateConcept(test.Primary, test.Sources)
			sources := test.Sources
			sources = append(sources, test.Primary)
			expected := ConcordedConcept{
				AdditionalConcordedFields: AdditionalConcordedFields{
					Fields:                test.Primary.Fields,
					SourceRepresentations: sources,
				},
			}
			if !cmp.Equal(expected, actual) {
				diff := cmp.Diff(expected, actual)
				t.Fatal(diff)
			}
		})
	}
}

func TestCreateAggregateConcept_WithDummyConfig(t *testing.T) {
	// WARNING: don't run this test parallel with others. It changes the global config.
	test := struct {
		Sources   string
		Aggregate string
	}{
		Sources:   "testdata/dummy-config/sources.json",
		Aggregate: "testdata/dummy-config/aggregate.json",
	}

	// backup config before modifying it.
	backup := GetConfig()
	defer setGlobalConfig(backup)

	cfg := backup
	cfg.Fields = map[string]FieldConfig{
		"test": {NeoProp: "test"},
	}
	cfg.Relationships = map[string]RelationshipConfig{
		"relOverride": {
			ConceptField: "relOverride",
			Strategy:     OverwriteStrategy,
		},
		"relAggregate": {
			ConceptField: "relAggregate",
			Strategy:     AggregateStrategy,
		},
	}
	setGlobalConfig(cfg)

	sources := readSourcesFixture(t, test.Sources)
	expected := readAggregateFixture(t, test.Aggregate)
	primary := sources[len(sources)-1]
	actual := CreateAggregateConcept(primary, sources[:len(sources)-1])
	sortAliases(&expected)
	sortAliases(&actual)
	if !cmp.Equal(expected, actual) {
		diff := cmp.Diff(expected, actual)
		t.Fatal(diff)
	}
}

func sortAliases(concorded *ConcordedConcept) {
	sort.Strings(concorded.Aliases)
	for idx := 0; idx < len(concorded.SourceRepresentations); idx++ {
		sort.Strings(concorded.SourceRepresentations[idx].Aliases)
	}
}

func readSourcesFixture(t *testing.T, fixture string) []SourceConcept {
	t.Helper()
	f, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	result := []SourceConcept{}
	err = json.NewDecoder(f).Decode(&result)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func readAggregateFixture(t *testing.T, fixture string) ConcordedConcept {
	t.Helper()
	f, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	result := ConcordedConcept{}
	err = json.NewDecoder(f).Decode(&result)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
