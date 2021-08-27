package ontology_test

import (
	"encoding/json"
	"os"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"

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
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			sources := readSourcesFixture(t, test.Sources)
			expected := readAggregateFixture(t, test.Aggregate)
			actual := ontology.CreateAggregateConcept(sources)
			sortAliases(&expected)
			sortAliases(&actual)
			if !cmp.Equal(expected, actual) {
				diff := cmp.Diff(expected, actual)
				t.Fatal(diff)
			}
		})
	}
}

func sortAliases(concorded *ontology.ConcordedConcept) {
	sort.Strings(concorded.Aliases)
	for idx := 0; idx < len(concorded.SourceRepresentations); idx++ {
		sort.Strings(concorded.SourceRepresentations[idx].Aliases)
	}
}

func readSourcesFixture(t *testing.T, fixture string) []ontology.SourceConcept {
	t.Helper()
	f, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	result := []ontology.SourceConcept{}
	err = json.NewDecoder(f).Decode(&result)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func readAggregateFixture(t *testing.T, fixture string) ontology.ConcordedConcept {
	t.Helper()
	f, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	result := ontology.ConcordedConcept{}
	err = json.NewDecoder(f).Decode(&result)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
