package concept

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	"github.com/stretchr/testify/assert"

	ontology "github.com/Financial-Times/cm-graph-ontology/v2"
	"github.com/Financial-Times/cm-graph-ontology/v2/transform"

	"github.com/Financial-Times/aggregate-concept-transformer/sqs"
)

func TestHandlers(t *testing.T) {
	testCases := map[string]struct {
		method         string
		url            string
		requestBody    string
		resultCode     int
		resultJSONBody map[string]interface{}
		resultTextBody string
		err            error
		concepts       map[string]transform.OldAggregatedConcept
		notifications  []sqs.ConceptUpdate
		healthchecks   []fthealth.Check
		cancelContext  bool
	}{
		"Get Concept - Success": {
			method:     "GET",
			url:        "/concept/f7fd05ea-9999-47c0-9be9-c99dd84d0097",
			resultCode: 200,
			resultJSONBody: map[string]interface{}{
				"prefUUID":  "f7fd05ea-9999-47c0-9be9-c99dd84d0097",
				"type":      "TestConcept",
				"prefLabel": "TestConcept",
			},
			concepts: map[string]transform.OldAggregatedConcept{
				"f7fd05ea-9999-47c0-9be9-c99dd84d0097": {
					PrefUUID:  "f7fd05ea-9999-47c0-9be9-c99dd84d0097",
					Type:      "TestConcept",
					PrefLabel: "TestConcept",
				},
			},
		},
		"Get External Concept - Success": {
			method:     "GET",
			url:        "/concept/f7fd05ea-9999-47c0-9be9-c99dd84d0097?publication=8e6c705e-1132-42a2-8db0-c295e29e8658",
			resultCode: 200,
			resultJSONBody: map[string]interface{}{
				"prefUUID":  "f7fd05ea-9999-47c0-9be9-c99dd84d0097",
				"prefLabel": "TestConcept",
				"type":      "TestConcept",
			},
			concepts: map[string]transform.OldAggregatedConcept{
				"8e6c705e-1132-42a2-8db0-c295e29e8658-f7fd05ea-9999-47c0-9be9-c99dd84d0097": {
					PrefUUID:  "f7fd05ea-9999-47c0-9be9-c99dd84d0097",
					PrefLabel: "TestConcept",
					Type:      "TestConcept",
				},
			},
		},
		"Get External Concept - Not Found": {
			method:     "GET",
			url:        "/concept/f7fd05ea-9999-47c0-9be9-c99dd84d0097?publication=8e6c705e-1132-42a2-8db0-c295e29e8658",
			resultCode: 500,
			resultJSONBody: map[string]interface{}{
				"message": "Canonical concept not found in S3",
			},
			err: errors.New("Canonical concept not found in S3"),
		},
		"Get Concept - Not Found": {
			method:     "GET",
			url:        "/concept/f7fd05ea-9999-47c0-9be9-c99dd84d0097",
			resultCode: 500,
			resultJSONBody: map[string]interface{}{
				"message": "Canonical concept not found in S3",
			},
			err: errors.New("Canonical concept not found in S3"),
		},
		"Send Concept - Success": {
			method:     "POST",
			url:        "/concept/f7fd05ea-9999-47c0-9be9-c99dd84d0097/send",
			resultCode: 200,
			resultJSONBody: map[string]interface{}{
				"message": "Concept f7fd05ea-9999-47c0-9be9-c99dd84d0097 updated successfully.",
			},
			concepts: map[string]transform.OldAggregatedConcept{
				"f7fd05ea-9999-47c0-9be9-c99dd84d0097": {
					PrefUUID:  "f7fd05ea-9999-47c0-9be9-c99dd84d0097",
					Type:      "TestConcept",
					PrefLabel: "TestConcept",
				},
			},
		},
		"Send Concept - Failure": {
			method:     "POST",
			url:        "/concept/f7fd05ea-9999-47c0-9be9-c99dd84d0097/send",
			resultCode: 500,
			resultJSONBody: map[string]interface{}{
				"message": "could not process the concept",
			},
			err: errors.New("could not process the concept"),
		},
		"GTG - Success": {
			method:         "GET",
			url:            "/__gtg",
			resultCode:     200,
			resultTextBody: "OK",
		},
		"GTG - Failure": {
			method:         "GET",
			url:            "/__gtg",
			resultCode:     503,
			resultTextBody: "GTG fail error",
			healthchecks: []fthealth.Check{
				{
					Checker: func() (string, error) {
						return "", errors.New("GTG fail error")
					},
				},
			},
		},
		"Get Concept - Context cancelled": {
			method:     "GET",
			url:        "/concept/f7fd05ea-9999-47c0-9be9-c99dd84d0097",
			resultCode: 500,
			resultJSONBody: map[string]interface{}{
				"message": "context canceled",
			},
			cancelContext: true,
		},
		"Send Concept - Context cancelled": {
			method:     "POST",
			url:        "/concept/f7fd05ea-9999-47c0-9be9-c99dd84d0097/send",
			resultCode: 500,
			resultJSONBody: map[string]interface{}{
				"message": "context canceled",
			},
			cancelContext: true,
		},
	}

	for testName, d := range testCases {
		t.Run(testName, func(t *testing.T) {
			fb := make(chan bool)
			mockService := NewMockService(d.concepts, d.notifications, d.healthchecks, d.err)
			handler := NewHandler(mockService, time.Second*1)
			sm := handler.RegisterHandlers(NewHealthService(mockService, "system-code", "app-name", 8080, "description"), true, fb)

			ctx, cancel := context.WithCancel(context.Background())
			if d.cancelContext {
				cancel()
			} else {
				defer cancel()
			}

			req, _ := http.NewRequestWithContext(ctx, d.method, d.url, bytes.NewBufferString(d.requestBody))
			rr := httptest.NewRecorder()

			sm.ServeHTTP(rr, req)

			assert.Equal(t, d.resultCode, rr.Code, testName)
			if d.resultTextBody != "" {
				b, err := io.ReadAll(rr.Body)
				assert.NoError(t, err)
				actual := string(b)
				assert.Equal(t, d.resultTextBody, actual, testName)
				return
			}
			if d.resultJSONBody != nil {
				actual := map[string]interface{}{}
				err := json.NewDecoder(rr.Body).Decode(&actual)
				assert.NoError(t, err)
				assert.Equal(t, d.resultJSONBody, actual, testName)
				return
			}
		})
	}
}

type MockService struct {
	notifications []sqs.ConceptUpdate
	concepts      map[string]transform.OldAggregatedConcept
	m             sync.RWMutex
	healthchecks  []fthealth.Check
	err           error
}

func NewMockService(concepts map[string]transform.OldAggregatedConcept, notifications []sqs.ConceptUpdate, healthchecks []fthealth.Check, err error) *MockService {
	return &MockService{
		concepts:      concepts,
		notifications: notifications,
		healthchecks:  healthchecks,
		err:           err,
	}
}

func (s *MockService) ProcessMessage(ctx context.Context, UUID string, bookmark string) error {
	if _, _, err := s.GetConcordedConcept(ctx, UUID, bookmark); err != nil {
		return err
	}
	return nil
}

func (s *MockService) GetConcordedConcept(ctx context.Context, UUID string, bookmark string) (ontology.CanonicalConcept, string, error) {
	if s.err != nil {
		return ontology.CanonicalConcept{}, "", s.err
	}
	c, ok := s.concepts[UUID]
	if !ok {
		return ontology.CanonicalConcept{}, "", errors.New("concept not found")
	}
	newConcept, err := transform.ToCanonicalConcept(c)
	if err != nil {
		return ontology.CanonicalConcept{}, "", err
	}
	return newConcept, "tid", nil
}

func (s *MockService) Healthchecks() []fthealth.Check {
	if s.healthchecks != nil {
		return s.healthchecks
	}
	return []fthealth.Check{}
}
