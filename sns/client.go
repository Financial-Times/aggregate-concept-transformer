package sns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
)

type PublishAPI interface {
	PublishBatchWithContext(aws.Context, *sns.PublishBatchInput, ...request.Option) (*sns.PublishBatchOutput, error)
}

type Client interface {
	PublishEvents(context.Context, []Event) error
}

type client struct {
	sns      PublishAPI
	topicArn *string
}

func NewClient(topicArn string) (Client, error) {
	tarn, err := arn.Parse(topicArn)
	if err != nil {
		return nil, fmt.Errorf("parsing topic arn: %w", err)
	}

	cfg := aws.NewConfig().WithRegion(tarn.Region)
	sess, err := session.NewSession(cfg)
	if err != nil {
		return nil, fmt.Errorf("creating new aws session: %w", err)
	}

	snsSvc := sns.New(sess)

	return &client{
		sns:      snsSvc,
		topicArn: &topicArn,
	}, nil
}

func (c *client) PublishEvents(ctx context.Context, events []Event) error {
	entries := []*sns.PublishBatchRequestEntry{}

	for i, ev := range events {
		evData, err := json.Marshal(ev)
		if err != nil {
			return fmt.Errorf("marshaling concept %q: %w", ev.ConceptUUID, err)
		}

		entry := &sns.PublishBatchRequestEntry{
			Id:      aws.String(ev.ConceptUUID + "_" + strconv.Itoa(i)),
			Message: aws.String(string(evData)),
		}

		entries = append(entries, entry)
	}

	output, err := c.sns.PublishBatchWithContext(ctx, &sns.PublishBatchInput{
		TopicArn:                   c.topicArn,
		PublishBatchRequestEntries: entries,
	})
	if err != nil {
		return err
	}

	errs := []error{}
	for _, o := range output.Failed {
		err := fmt.Errorf("publishing %s event failed: %s", *o.Id, *o.Code)
		errs = append(errs, err)
	}

	return errors.Join(errs...)
}
