package concept

import (
	"context"
	"strings"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	"github.com/stretchr/testify/mock"

	ontology "github.com/Financial-Times/cm-graph-ontology/v2"
	"github.com/Financial-Times/cm-graph-ontology/v2/transform"
)

type mockS3Client struct {
	mock.Mock
	concepts map[string]struct {
		transactionID string
		concept       transform.OldConcept
	}
	err         error
	callsMocked bool
}

func (s *mockS3Client) GetConceptAndTransactionID(ctx context.Context, publication string, UUID string) (bool, ontology.SourceConcept, string, error) {
	if s.callsMocked {
		s.Called(UUID)
	}

	key := UUID
	if publication != "" {
		key = strings.Join([]string{publication, UUID}, "/")
	}

	c, ok := s.concepts[key]
	if !ok {
		return false, ontology.SourceConcept{}, "", s.err
	}

	concept, err := transform.ToNewSourceConcept(c.concept)
	if err != nil {
		return false, ontology.SourceConcept{}, "", err
	}
	return true, concept, c.transactionID, s.err
}
func (s *mockS3Client) Healthcheck() fthealth.Check {
	return fthealth.Check{
		Checker: func() (string, error) {
			return "", nil
		},
	}
}
