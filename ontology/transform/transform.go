package transform

import (
	"encoding/json"

	"github.com/Financial-Times/aggregate-concept-transformer/ontology"
)

func ToNewSourceConcept(old OldConcept) (ontology.SourceConcept, error) {
	data, err := json.Marshal(&old)
	if err != nil {
		return ontology.SourceConcept{}, err
	}
	result := ontology.SourceConcept{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return ontology.SourceConcept{}, err
	}
	return result, nil
}

func ToOldSourceConcept(new ontology.SourceConcept) (OldConcept, error) {
	data, err := json.Marshal(&new)
	if err != nil {
		return OldConcept{}, err
	}
	result := OldConcept{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return OldConcept{}, err
	}
	return result, nil
}

func ToNewAggregateConcept(old OldConcordedConcept) (ontology.ConcordedConcept, error) {
	data, err := json.Marshal(&old)
	if err != nil {
		return ontology.ConcordedConcept{}, err
	}
	result := ontology.ConcordedConcept{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return ontology.ConcordedConcept{}, err
	}
	return result, nil
}

func ToOldAggregateConcept(new ontology.ConcordedConcept) (OldConcordedConcept, error) {
	data, err := json.Marshal(&new)
	if err != nil {
		return OldConcordedConcept{}, err
	}
	result := OldConcordedConcept{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return OldConcordedConcept{}, err
	}
	return result, nil
}
