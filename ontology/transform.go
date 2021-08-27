package ontology

import "encoding/json"

func (sc SourceConcept) ToOldConcept() (OldConcept, error) {
	data, err := json.Marshal(sc)
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

func (sc OldConcept) ToSourceConcept() (SourceConcept, error) {
	data, err := json.Marshal(sc)
	if err != nil {
		return SourceConcept{}, err
	}
	result := SourceConcept{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return SourceConcept{}, err
	}
	return result, nil
}

func (cc ConcordedConcept) ToOldConcordedConcept() (OldConcordedConcept, error) {
	data, err := json.Marshal(cc)
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

func (cc OldConcordedConcept) ToConcordedConcept() (ConcordedConcept, error) {
	data, err := json.Marshal(cc)
	if err != nil {
		return ConcordedConcept{}, err
	}
	result := ConcordedConcept{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return ConcordedConcept{}, err
	}
	return result, nil
}
