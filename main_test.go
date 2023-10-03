package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Financial-Times/go-logger"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/Financial-Times/aggregate-concept-transformer/concept"
	"github.com/Financial-Times/aggregate-concept-transformer/concordances"
	"github.com/Financial-Times/aggregate-concept-transformer/sns"
	"github.com/Financial-Times/aggregate-concept-transformer/sqs"

	ontology "github.com/Financial-Times/cm-graph-ontology"
	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
)

// service integration test

func TestAggregateService_GetConceptHandler(t *testing.T) {
	logger.InitLogger("aggregate-concept-transformer-testing", "panic")

	expected := readAggregateConceptFixture(t, "testdata/aggregated-concept.json")
	sources := readSourceConceptFixture(t, "testdata/source-concepts.json")

	// generic http server that will handle all aggregate service requests
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.Contains(request.RequestURI, "concordances") {
			http.NotFound(writer, request)
			return
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// make sure that s3 will return the correct sources based on the setup data
	s3Concepts := map[string]ontology.NewConcept{}
	for _, s := range sources {
		s3Concepts[s.UUID] = s
	}
	s3 := s3Mock{
		concepts: s3Concepts,
	}
	externalS3Mock := s3Mock{
		concepts: s3Concepts,
	}
	// sqs and kinesis are currently not used in this test so no specifics
	sqsClient := &sqsMock{}
	snsClient := &snsMock{}
	ksClient := &kinesisMock{}
	concordancesClient, err := concordances.NewClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	timeout := time.Second * 30
	feedback := make(chan bool)
	done := make(chan struct{})
	defer close(feedback)
	defer close(done)

	service := concept.NewService(s3, externalS3Mock, sqsClient, snsClient, concordancesClient, ksClient, server.URL+"/neo4j", server.URL+"/elastic", server.URL+"/varnish", []string{""}, server.Client(), feedback, done, timeout, true)
	handler := concept.NewHandler(service, timeout)

	m := handler.RegisterHandlers(concept.NewHealthService(service, "", "", 8080, ""), false, feedback)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/concept/"+expected.PrefUUID, nil)

	m.ServeHTTP(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatal(http.StatusText(resp.StatusCode))
	}

	var actual ontology.NewAggregatedConcept
	err = json.NewDecoder(resp.Body).Decode(&actual)
	if err != nil {
		t.Fatal(err)
	}

	compareAggregateConcepts(t, expected, actual)
}

func compareAggregateConcepts(t *testing.T, expected, actual ontology.NewAggregatedConcept) {
	t.Helper()
	opts := cmp.Options{
		cmpopts.SortSlices(func(l, r ontology.Relationship) bool {
			return strings.Compare(l.Label, r.Label) > 0
		}),
		cmpopts.SortSlices(func(l, r string) bool {
			return strings.Compare(l, r) > 0
		}),
	}
	if !cmp.Equal(expected, actual, opts) {
		diff := cmp.Diff(expected, actual, opts)
		t.Fatal(diff)
	}
}

// great place to use generics for readSourceConceptFixture and readAggregateConceptFixture
func readSourceConceptFixture(t *testing.T, filename string) []ontology.NewConcept {
	t.Helper()

	f, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var result []ontology.NewConcept
	err = json.NewDecoder(f).Decode(&result)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func readAggregateConceptFixture(t *testing.T, filename string) ontology.NewAggregatedConcept {
	t.Helper()

	f, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var result ontology.NewAggregatedConcept
	err = json.NewDecoder(f).Decode(&result)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

type s3Mock struct {
	concepts map[string]ontology.NewConcept
}

func (s s3Mock) GetConceptAndTransactionID(ctx context.Context, UUID string) (bool, ontology.NewConcept, string, error) {
	concept, ok := s.concepts[UUID]
	if !ok {
		return false, ontology.NewConcept{}, "", errors.New("not found")
	}
	return true, concept, "tid_test", nil
}

func (s s3Mock) Healthcheck() fthealth.Check {
	return fthealth.Check{}
}

type sqsMock struct {
}

func (s sqsMock) ListenAndServeQueue(ctx context.Context) []sqs.ConceptUpdate {
	//TODO implement me
	panic("implement me")
}

func (s sqsMock) RemoveMessageFromQueue(ctx context.Context, receiptHandle *string) error {
	//TODO implement me
	panic("implement me")
}

func (s sqsMock) Healthcheck() fthealth.Check {
	return fthealth.Check{}
}

type snsMock struct {
}

func (s snsMock) PublishEvents(ctx context.Context, events []sns.Event) error {
	//TODO implement me
	panic("implement me")
}

type kinesisMock struct {
}

func (k kinesisMock) AddRecordToStream(ctx context.Context, updatedConcept []byte, conceptType string) error {
	//TODO implement me
	panic("implement me")
}

func (k kinesisMock) Healthcheck() fthealth.Check {
	return fthealth.Check{}
}
