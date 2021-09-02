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
	}

	for name, test := range tests {
		test := test
		t.Run(name, func(t *testing.T) {
			sources := readSourcesFixture(t, test.Sources)
			expected := readAggregateFixture(t, test.Aggregate)
			actual := CreateAggregateConcept(sources)
			sortAliases(&expected)
			sortAliases(&actual)
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
	cfg.FieldToNeoProps = map[string]string{
		"test": "test",
	}
	setGlobalConfig(cfg)

	sources := readSourcesFixture(t, test.Sources)
	expected := readAggregateFixture(t, test.Aggregate)
	actual := CreateAggregateConcept(sources)
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
