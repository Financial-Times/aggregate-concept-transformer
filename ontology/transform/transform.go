package transform

import (
	"encoding/json"

	"github.com/Financial-Times/aggregate-concept-transformer/ontology"
)

func ToNewSourceConcept(old OldConcept) (ontology.NewConcept, error) {
	data, err := json.Marshal(&old)
	if err != nil {
		return ontology.NewConcept{}, err
	}
	result := ontology.NewConcept{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return ontology.NewConcept{}, err
	}
	return result, nil
}

func ToOldSourceConcept(new ontology.NewConcept) (OldConcept, error) {
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

func ToNewAggregateConcept(old OldAggregatedConcept) (ontology.NewAggregatedConcept, error) {
	data, err := json.Marshal(&old)
	if err != nil {
		return ontology.NewAggregatedConcept{}, err
	}
	result := ontology.NewAggregatedConcept{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return ontology.NewAggregatedConcept{}, err
	}
	return result, nil
}

func ToOldAggregateConcept(new ontology.NewAggregatedConcept) (OldAggregatedConcept, error) {
	data, err := json.Marshal(&new)
	if err != nil {
		return OldAggregatedConcept{}, err
	}
	result := OldAggregatedConcept{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return OldAggregatedConcept{}, err
	}
	return result, nil
}
