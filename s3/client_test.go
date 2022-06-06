package s3

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/google/go-cmp/cmp"

	"github.com/Financial-Times/cm-graph-ontology/transform"
)

func TestClient_GetConceptAndTransactionID(t *testing.T) {
	testBucket := "testBucket"
	testKey := "testKey"
	testTID := "tid_test"
	testConceptFixture := "testdata/test-concept.json"
	expected := readOldConceptFixture(t, testConceptFixture)

	client := &Client{
		s3: &mockS3API{
			t:                  t,
			testBucket:         testBucket,
			testKey:            testKey,
			testTID:            testTID,
			testConceptFixture: testConceptFixture,
		},
		bucketName: testBucket,
	}

	has, concept, tid, err := client.GetConceptAndTransactionID(context.Background(), testKey)
	if err != nil {
		t.Fatal(err)
	}
	if !has {
		t.Error("expected s3 to have the concept")
	}
	if tid != testTID {
		t.Errorf("expect tid %v, got %v", testTID, tid)
	}

	actual, err := transform.ToOldSourceConcept(concept)
	if err != nil {
		t.Fatal(err)
	}

	if !cmp.Equal(expected, actual) {
		diff := cmp.Diff(expected, concept)
		t.Errorf("concept mismatch: %s", diff)
	}
}

func readOldConceptFixture(t *testing.T, filename string) transform.OldConcept {
	t.Helper()
	f, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	result := transform.OldConcept{}
	err = json.NewDecoder(f).Decode(&result)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

type mockS3API struct {
	t                  *testing.T
	testBucket         string
	testKey            string
	testTID            string
	testConceptFixture string
}

func (m *mockS3API) GetObjectWithContext(ctx aws.Context, input *s3.GetObjectInput, opts ...request.Option) (*s3.GetObjectOutput, error) {
	m.t.Helper()
	if input.Bucket == nil {
		m.t.Fatal("expect bucket to not be nil")
	}
	if e, a := m.testBucket, *input.Bucket; e != a {
		m.t.Errorf("expect bucket %v, got %v", e, a)
	}
	if input.Key == nil {
		m.t.Fatal("expect key to not be nil")
	}
	if e, a := m.testKey, *input.Key; e != a {
		m.t.Errorf("expect key %v, got %v", e, a)
	}

	conceptFile, err := os.Open(m.testConceptFixture)
	if err != nil {
		m.t.Fatal(err)
	}

	return &s3.GetObjectOutput{
		Body: conceptFile,
	}, nil
}

func (m *mockS3API) HeadObjectWithContext(ctx aws.Context, input *s3.HeadObjectInput, opts ...request.Option) (*s3.HeadObjectOutput, error) {
	m.t.Helper()
	if input.Bucket == nil {
		m.t.Fatal("expect bucket to not be nil")
	}
	if e, a := m.testBucket, *input.Bucket; e != a {
		m.t.Errorf("expect bucket %v, got %v", e, a)
	}
	if input.Key == nil {
		m.t.Fatal("expect key to not be nil")
	}
	if e, a := m.testKey, *input.Key; e != a {
		m.t.Errorf("expect key %v, got %v", e, a)
	}
	return &s3.HeadObjectOutput{
		Metadata: map[string]*string{
			"Transaction_id": aws.String(m.testTID),
		},
	}, nil
}

func (m *mockS3API) HeadBucket(input *s3.HeadBucketInput) (*s3.HeadBucketOutput, error) {
	panic("implement me")
}
