package sns

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/sns"
)

type MockPublishAPI func(ctx aws.Context, input *sns.PublishBatchInput, opts ...request.Option) (*sns.PublishBatchOutput, error)

func (m MockPublishAPI) PublishBatchWithContext(ctx aws.Context, input *sns.PublishBatchInput, opts ...request.Option) (*sns.PublishBatchOutput, error) {
	return m(ctx, input, opts...)
}

var (
	ErrNotFound      = awserr.New(sns.ErrCodeNotFoundException, "Topic does not exist", nil)
	ErrToManuEntries = awserr.New(sns.ErrCodeTooManyEntriesInBatchRequestException, "The batch request contains more entries than permissible", nil)

	joinederr = errors.Join(
		fmt.Errorf("publishing %s event failed: %s", "28090964-9997-4bc2-9638-7a11135aaff9_0", "some-aws-error-code"),
		fmt.Errorf("publishing %s event failed: %s", "34a571fb-d779-4610-a7ba-2e127676db4d_2", "some-aws-error-code"),
	)
)

func TestPublishEvents(t *testing.T) {
	tests := map[string]struct {
		getSNSSvc func(t *testing.T) PublishAPI
		events    []Event
		wanterr   error
	}{
		"Successfully": {
			getSNSSvc: func(t *testing.T) PublishAPI {
				return MockPublishAPI(func(ctx aws.Context, input *sns.PublishBatchInput, opts ...request.Option) (*sns.PublishBatchOutput, error) {
					return &sns.PublishBatchOutput{}, nil
				})
			},
			events: []Event{
				{},
			},
		},
		"Unsuccessfully": {
			getSNSSvc: func(t *testing.T) PublishAPI {
				return MockPublishAPI(func(ctx aws.Context, input *sns.PublishBatchInput, opts ...request.Option) (*sns.PublishBatchOutput, error) {
					// if topic arn is wrong
					return nil, ErrNotFound
				})
			},
			wanterr: ErrNotFound,
			events: []Event{
				{},
			},
		},
		"Unsuccessfully-Too-many-entries": {
			getSNSSvc: func(t *testing.T) PublishAPI {
				return MockPublishAPI(func(ctx aws.Context, input *sns.PublishBatchInput, opts ...request.Option) (*sns.PublishBatchOutput, error) {
					if len(input.PublishBatchRequestEntries) > 10 {
						return nil, ErrToManuEntries
					}

					return &sns.PublishBatchOutput{}, nil
				})
			},
			wanterr: ErrToManuEntries,
			events: []Event{
				{
					ConceptUUID: "28090964-9997-4bc2-9638-7a11135aaff9",
				},
				{
					ConceptUUID: "28090964-9997-4bc2-9638-7a11135aaff9",
				},
				{
					ConceptUUID: "28090964-9997-4bc2-9638-7a11135aaff9",
				},
				{
					ConceptUUID: "28090964-9997-4bc2-9638-7a11135aaff9",
				},
				{
					ConceptUUID: "28090964-9997-4bc2-9638-7a11135aaff9",
				},
				{
					ConceptUUID: "28090964-9997-4bc2-9638-7a11135aaff9",
				},
				{
					ConceptUUID: "28090964-9997-4bc2-9638-7a11135aaff9",
				},
				{
					ConceptUUID: "28090964-9997-4bc2-9638-7a11135aaff9",
				},
				{
					ConceptUUID: "28090964-9997-4bc2-9638-7a11135aaff9",
				},
				{
					ConceptUUID: "28090964-9997-4bc2-9638-7a11135aaff9",
				},
				{
					ConceptUUID: "28090964-9997-4bc2-9638-7a11135aaff9",
				},
			},
		},
		"PartialSuccess": {
			getSNSSvc: func(t *testing.T) PublishAPI {
				return MockPublishAPI(func(ctx aws.Context, input *sns.PublishBatchInput, opts ...request.Option) (*sns.PublishBatchOutput, error) {
					return &sns.PublishBatchOutput{
						Failed: []*sns.BatchResultErrorEntry{
							{
								Id:   aws.String("28090964-9997-4bc2-9638-7a11135aaff9_0"),
								Code: aws.String("some-aws-error-code"),
							},
							{
								Id:   aws.String("34a571fb-d779-4610-a7ba-2e127676db4d_2"),
								Code: aws.String("some-aws-error-code"),
							},
						},
					}, nil
				})
			},
			wanterr: joinederr,
			events: []Event{
				{
					ConceptType:   "Person",
					ConceptUUID:   "28090964-9997-4bc2-9638-7a11135aaff9",
					AggregateHash: "1234567890",
					EventDetails: struct {
						Type string
					}{
						Type: "Concept Updated",
					},
				},
				{
					ConceptType:   "Person",
					ConceptUUID:   "28090964-9997-4bc2-9638-7a11135aaf10",
					AggregateHash: "1234567890",
					EventDetails: struct {
						Type  string
						OldID string
						NewID string
					}{
						Type:  "Concordance Added",
						OldID: "34a571fb-d779-4610-a7ba-2e127676db4d",
						NewID: "28090964-9997-4bc2-9638-7a11135aaff9",
					},
				},
				{
					ConceptType:   "Person",
					ConceptUUID:   "34a571fb-d779-4610-a7ba-2e127676db4d",
					AggregateHash: "1234567890",
					EventDetails: struct {
						Type string
					}{
						Type: "Concept Updated",
					},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ta := "test-topic"
			client := &client{
				topicArn: &ta,
				sns:      test.getSNSSvc(t),
			}

			err := client.PublishEvents(context.TODO(), test.events)
			if err == nil && test.wanterr == nil {
				return
			}
			if err == nil {
				t.Fatalf("want: %s, got: nil", test.wanterr)
			}
			if test.wanterr == nil {
				t.Fatalf("did not expect err, got: %s", err)
			}

			if err.Error() != test.wanterr.Error() {
				t.Fatalf("got: %s, want: %s", err, test.wanterr)
			}
		})
	}
}
