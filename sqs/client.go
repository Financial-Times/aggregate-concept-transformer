package sqs

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	"github.com/Financial-Times/go-logger"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sqs"
)

var keyMatcher = regexp.MustCompile("[0-9a-f]{8}/[0-9a-f]{4}/[0-9a-f]{4}/[0-9a-f]{4}/[0-9a-f]{12}")

type Client interface {
	ListenAndServeQueue(ctx context.Context) []ConceptUpdate
	RemoveMessageFromQueue(ctx context.Context, receiptHandle *string) error
	Healthcheck() fthealth.Check
}

type NotificationClient struct {
	sqs          *sqs.SQS
	listenParams sqs.ReceiveMessageInput
	queueUrl     string
}

func NewClient(awsRegion, queueURL, endpoint string, messagesToProcess, visibilityTimeout, waitTime int) (Client, error) {
	listenParams := sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(queueURL),
		MaxNumberOfMessages: aws.Int64(int64(messagesToProcess)),
		VisibilityTimeout:   aws.Int64(int64(visibilityTimeout)),
		WaitTimeSeconds:     aws.Int64(int64(waitTime)),
	}

	conf := &aws.Config{
		Region:     aws.String(awsRegion),
		MaxRetries: aws.Int(3),
	}
	if endpoint != "" {
		conf.Endpoint = aws.String(endpoint)
	}
	sess, err := session.NewSession(conf)
	if err != nil {
		logger.WithError(err).Error("Unable to create an SQS client")
		return &NotificationClient{}, err
	}
	credValues, err := sess.Config.Credentials.Get()
	if err != nil {
		return &NotificationClient{}, fmt.Errorf("failed to obtain AWS credentials for values with error: %w, while creating sqs client", err)
	}
	logger.Infof("Obtaining AWS credentials by using [%s] as provider for sqs client", credValues.ProviderName)

	client := sqs.New(sess)
	return &NotificationClient{
		sqs:          client,
		listenParams: listenParams,
		queueUrl:     queueURL,
	}, err
}

func (c *NotificationClient) ListenAndServeQueue(ctx context.Context) []ConceptUpdate {
	messages, err := c.sqs.ReceiveMessageWithContext(ctx, &c.listenParams)
	if err != nil {
		logger.WithError(err).Error("Error whilst listening for messages")
	}
	return getNotificationsFromMessages(messages.Messages)
}

func (c *NotificationClient) RemoveMessageFromQueue(ctx context.Context, receiptHandle *string) error {
	deleteParams := sqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.queueUrl),
		ReceiptHandle: receiptHandle,
	}
	if _, err := c.sqs.DeleteMessageWithContext(ctx, &deleteParams); err != nil {
		logger.WithError(err).Error("Error deleting message from SQS")
		return err
	}
	return nil
}

func getNotificationsFromMessages(messages []*sqs.Message) []ConceptUpdate {

	notifications := []ConceptUpdate{}

	for _, message := range messages {
		var err error
		receiptHandle := message.ReceiptHandle
		messageBody := Body{}
		if err = json.Unmarshal([]byte(*message.Body), &messageBody); err != nil {
			logger.WithError(err).Error("Failed to unmarshal SQS message")
			continue
		}

		msgRecord := Message{}
		if err = json.Unmarshal([]byte(messageBody.Message), &msgRecord); err != nil {
			logger.WithError(err).Error("Failed to unmarshal S3 notification")
			continue
		}

		if msgRecord.Records == nil {
			logger.Error("Cannot map message to expected JSON format - skipping")
			continue
		}
		key := msgRecord.Records[0].S3.Object.Key
		matches := keyMatcher.FindAllString(key, 2)

		if matches == nil {
			logger.WithField("key", key).Error("no valid UUID matches in the key")
			continue
		}

		bookmark := msgRecord.Records[0].Bookmark
		notifications = append(notifications, ConceptUpdate{
			UUID:          strings.Replace(key, "/", "-", -1),
			Bookmark:      bookmark, //no need to verify via regex, because neo4j might change the pattern..
			ReceiptHandle: receiptHandle,
		})
	}

	return notifications
}

func (c *NotificationClient) Healthcheck() fthealth.Check {
	return fthealth.Check{
		BusinessImpact:   "Editorial updates of concepts will not be written into UPP",
		Name:             "Check connectivity to SQS queue",
		PanicGuide:       "https://runbooks.in.ft.com/aggregate-concept-transformer",
		Severity:         3,
		TechnicalSummary: `Cannot connect to SQS queue. If this check fails, check that Amazon SQS is available`,
		Checker: func() (string, error) {
			params := &sqs.GetQueueAttributesInput{
				QueueUrl:       aws.String(c.queueUrl),
				AttributeNames: []*string{aws.String("ApproximateNumberOfMessages")},
			}
			if _, err := c.sqs.GetQueueAttributes(params); err != nil {
				logger.WithError(err).Error("Got error running SQS health check")
				return "", err
			}
			return "", nil
		},
	}
}
