package ontology_test

import (
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/Financial-Times/aggregate-concept-transformer/ontology"
)

func TestOldConcept_ToSourceConcept(t *testing.T) {
	// OldConcept and Source Concept should have the same json representation
	jsonData := readFixture(t, "testdata/single-source.json")
	old := ontology.OldConcept{}
	source := ontology.SourceConcept{}

	err := json.Unmarshal(jsonData, &old)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(jsonData, &source)
	if err != nil {
		t.Fatal(err)
	}

	sourceTransfomed, err := old.ToSourceConcept()
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(source, sourceTransfomed) {
		diff := cmp.Diff(source, sourceTransfomed)
		t.Fatal(diff)
	}
}

func TestSourceConcept_ToOldConcept(t *testing.T) {
	jsonData := readFixture(t, "testdata/single-source.json")
	old := ontology.OldConcept{}
	source := ontology.SourceConcept{}

	err := json.Unmarshal(jsonData, &old)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(jsonData, &source)
	if err != nil {
		t.Fatal(err)
	}

	oldTransformed, err := source.ToOldConcept()
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(old, oldTransformed) {
		diff := cmp.Diff(old, oldTransformed)
		t.Fatal(diff)
	}
}

func TestOldConcordedConcept_ToConcordedConcept(t *testing.T) {
	jsonData := readFixture(t, "testdata/aggregate.json")
	old := ontology.OldConcordedConcept{}
	concorded := ontology.ConcordedConcept{}

	err := json.Unmarshal(jsonData, &old)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(jsonData, &concorded)
	if err != nil {
		t.Fatal(err)
	}

	concordedTransformed, err := old.ToConcordedConcept()
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(concorded, concordedTransformed) {
		diff := cmp.Diff(concorded, concordedTransformed)
		t.Fatal(diff)
	}
}

func TestConcordedConcept_ToOldConcordedConcept(t *testing.T) {
	jsonData := readFixture(t, "testdata/aggregate.json")
	old := ontology.OldConcordedConcept{}
	source := ontology.ConcordedConcept{}

	err := json.Unmarshal(jsonData, &old)
	if err != nil {
		t.Fatal(err)
	}
	err = json.Unmarshal(jsonData, &source)
	if err != nil {
		t.Fatal(err)
	}

	oldTransformed, err := source.ToOldConcordedConcept()
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
