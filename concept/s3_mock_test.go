package concept

import (
	"context"

	ontology "github.com/Financial-Times/cm-graph-ontology"
	"github.com/Financial-Times/cm-graph-ontology/transform"
	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	"github.com/stretchr/testify/mock"
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

func (s *mockS3Client) GetConceptAndTransactionID(ctx context.Context, UUID string) (bool, ontology.NewConcept, string, error) {
	if s.callsMocked {
		s.Called(UUID)
	}

	c, ok := s.concepts[UUID]
	if !ok {
		return false, ontology.NewConcept{}, "", s.err
	}

	concept, err := transform.ToNewSourceConcept(c.concept)
	if err != nil {
		return false, ontology.NewConcept{}, "", err
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
