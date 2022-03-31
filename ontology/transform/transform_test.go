package transform_test

import (
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/Financial-Times/aggregate-concept-transformer/ontology"
	"github.com/Financial-Times/aggregate-concept-transformer/ontology/transform"
)

func TestToNewSourceConcept(t *testing.T) {
	// OldConcept and Source Concept should have the same json representation
	jsonData := readFixture(t, "testdata/single-source.json")
	old := transform.OldConcept{}
	source := ontology.NewConcept{}

	err := json.Unmarshal(jsonData, &old)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(jsonData, &source)
	if err != nil {
		t.Fatal(err)
	}

	sourceTransfomed, err := transform.ToNewSourceConcept(old)
	if err != nil {
		t.Fatal(err)
	}
	opts := cmp.Options{
		cmpopts.SortSlices(func(l, r ontology.Relationship) bool {
			return strings.Compare(l.Label, r.Label) > 0
		}),
	}
	if !cmp.Equal(source, sourceTransfomed, opts) {
		diff := cmp.Diff(source, sourceTransfomed, opts)
		t.Fatal(diff)
	}
}

func TestToOldSourceConcept(t *testing.T) {
	jsonData := readFixture(t, "testdata/single-source.json")
	old := transform.OldConcept{}
	source := ontology.NewConcept{}

	err := json.Unmarshal(jsonData, &old)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(jsonData, &source)
	if err != nil {
		t.Fatal(err)
	}

	oldTransformed, err := transform.ToOldSourceConcept(source)
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(old, oldTransformed) {
		diff := cmp.Diff(old, oldTransformed)
		t.Fatal(diff)
	}
}

func TestToNewAggregateConcept(t *testing.T) {
	jsonData := readFixture(t, "testdata/aggregate.json")
	old := transform.OldAggregatedConcept{}
	concorded := ontology.NewAggregatedConcept{}

	err := json.Unmarshal(jsonData, &old)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(jsonData, &concorded)
	if err != nil {
		t.Fatal(err)
	}

	concordedTransformed, err := transform.ToNewAggregateConcept(old)
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(concorded, concordedTransformed) {
		diff := cmp.Diff(concorded, concordedTransformed)
		t.Fatal(diff)
	}
}

func TestToOldAggregateConcept(t *testing.T) {
	jsonData := readFixture(t, "testdata/aggregate.json")
	old := transform.OldAggregatedConcept{}
	source := ontology.NewAggregatedConcept{}

	err := json.Unmarshal(jsonData, &old)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(jsonData, &source)
	if err != nil {
		t.Fatal(err)
	}

	oldTransformed, err := transform.ToOldAggregateConcept(source)
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(old, oldTransformed) {
		diff := cmp.Diff(old, oldTransformed)
		t.Fatal(diff)
	}
}

func readFixture(t *testing.T, fixture string) []byte {
	t.Helper()
	fixtureFile, err := os.Open(fixture)
	if err != nil {
		t.Fatal(err)
	}
	defer fixtureFile.Close()
	data, err := io.ReadAll(fixtureFile)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
