package ontology

import (
	"encoding/json"
	"io/ioutil"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCreateAggregatedConcept(t *testing.T) {
	tests := map[string]struct {
		Sources    string
		Aggregated string
	}{
		"valid": {
			Sources:    "testdata/valid-sources.json",
			Aggregated: "testdata/valid-aggregated.json",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			sources := readSources(t, test.Sources)
			aggregated := readAggregated(t, test.Aggregated)
			result := CreateAggregatedConcept(sources)
			if !cmp.Equal(aggregated, result) {
				diff := cmp.Diff(aggregated, result)
				t.Fatal(diff)
			}
		})
	}
}

func readFile(t *testing.T, filename string) []byte {
	t.Helper()
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func readSources(t *testing.T, filename string) []SourceConcept {
	data := readFile(t, filename)
	var result []SourceConcept
	err := json.Unmarshal(data, &result)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func readAggregated(t *testing.T, filename string) ConcordedConcept {
	data := readFile(t, filename)
	var result ConcordedConcept
	err := json.Unmarshal(data, &result)
	if err != nil {
		t.Fatal(err)
	}
	return result
}
