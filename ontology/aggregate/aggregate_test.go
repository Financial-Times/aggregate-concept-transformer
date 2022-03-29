package aggregate

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/Financial-Times/aggregate-concept-transformer/ontology"
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
		"source-only-relationships": {
			Sources:   "testdata/source-only-relationships-sources.json",
			Aggregate: "testdata/source-only-relationships-aggregate.json",
		},
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			sources := readSourcesFixture(t, test.Sources)
			expected := readAggregateFixture(t, test.Aggregate)
			primary := sources[len(sources)-1]
			actual := CreateAggregateConcept(primary, sources[:len(sources)-1])
			compareAggregateConcepts(t, expected, actual)
		})
	}
}

func TestCreateAggregateConcept_Properties(t *testing.T) {
	tests := map[string]struct {
		Primary ontology.NewConcept
		Sources []ontology.NewConcept
	}{
		"Properties": {
			Primary: ontology.NewConcept{AdditionalSourceFields: ontology.AdditionalSourceFields{Properties: map[string]interface{}{
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
			Sources: []ontology.NewConcept{
				{AdditionalSourceFields: ontology.AdditionalSourceFields{Properties: map[string]interface{}{
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
			expected := ontology.NewAggregatedConcept{
				RequiredConcordedFields: ontology.RequiredConcordedFields{
					SourceRepresentations: sources,
				},
				AdditionalConcordedFields: ontology.AdditionalConcordedFields{
					Properties: test.Primary.Properties,
				},
			}
			compareAggregateConcepts(t, expected, actual)
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
	backup := ontology.GetConfig()
	defer ontology.SetGlobalConfig(backup)

	cfg := backup
	cfg.Properties = map[string]ontology.PropertyConfig{
		"test": {NeoProp: "test"},
	}
	cfg.Relationships = map[string]ontology.RelationshipConfig{
		"relOverride": {
			ConceptField: "relOverride",
			Strategy:     ontology.OverwriteStrategy,
			OneToOne:     true,
		},
		"relAggregate": {
			ConceptField: "relAggregate",
			Strategy:     ontology.AggregateStrategy,
		},
	}
	ontology.SetGlobalConfig(cfg)

	sources := readSourcesFixture(t, test.Sources)
	expected := readAggregateFixture(t, test.Aggregate)
	primary := sources[len(sources)-1]
	actual := CreateAggregateConcept(primary, sources[:len(sources)-1])
	compareAggregateConcepts(t, expected, actual)
}

func compareAggregateConcepts(t *testing.T, expected, actual ontology.NewAggregatedConcept) {
	t.Helper()
	sortAliases(&expected)
	sortAliases(&actual)
	opts := cmp.Options{
		cmpopts.SortSlices(func(l, r ontology.Relationship) bool {
			return strings.Compare(l.Label, r.Label) > 0
		}),
	}
	if !cmp.Equal(expected, actual, opts) {
		diff := cmp.Diff(expected, actual, opts)
		t.Fatal(diff)
	}
}

func sortAliases(concorded *ontology.NewAggregatedConcept) {
	sort.Strings(concorded.Aliases)
	for idx := 0; idx < len(concorded.SourceRepresentations); idx++ {
		sort.Strings(concorded.SourceRepresentations[idx].Aliases)
	}
}

func readSourcesFixture(t *testing.T, fixture string) []ontology.NewConcept {
	t.Helper()
	f, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	result := []ontology.NewConcept{}
	err = json.NewDecoder(f).Decode(&result)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func readAggregateFixture(t *testing.T, fixture string) ontology.NewAggregatedConcept {
	t.Helper()
	f, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	result := ontology.NewAggregatedConcept{}
	err = json.NewDecoder(f).Decode(&result)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
