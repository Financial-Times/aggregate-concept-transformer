package concept

import (
	"context"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	"github.com/stretchr/testify/mock"

	"github.com/Financial-Times/aggregate-concept-transformer/ontology"
)

type mockS3Client struct {
	mock.Mock
	concepts map[string]struct {
		transactionID string
		concept       ontology.Concept
	}
	err         error
	callsMocked bool
}

func (s *mockS3Client) GetConceptAndTransactionID(ctx context.Context, UUID string) (bool, ontology.Concept, string, error) {
	if s.callsMocked {
		s.Called(UUID)
	}
	if c, ok := s.concepts[UUID]; ok {
		return true, c.concept, c.transactionID, s.err
	}
	return false, ontology.Concept{}, "", s.err
}
func (s *mockS3Client) Healthcheck() fthealth.Check {
	return fthealth.Check{
		Checker: func() (string, error) {
			return "", nil
		},
	}
}
