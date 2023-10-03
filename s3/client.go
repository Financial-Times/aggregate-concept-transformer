package s3

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	ontology "github.com/Financial-Times/cm-graph-ontology"
	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	"github.com/Financial-Times/go-logger"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type Client struct {
	s3         s3API
	bucketName string
}

type s3API interface {
	GetObjectWithContext(ctx aws.Context, input *s3.GetObjectInput, opts ...request.Option) (*s3.GetObjectOutput, error)
	HeadObjectWithContext(ctx aws.Context, input *s3.HeadObjectInput, opts ...request.Option) (*s3.HeadObjectOutput, error)
	HeadBucket(input *s3.HeadBucketInput) (*s3.HeadBucketOutput, error)
}

func NewClient(bucketName string, awsRegion string) (*Client, error) {
	hc := http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			MaxIdleConns:          20,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConnsPerHost:   20,
			TLSHandshakeTimeout:   3 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	sess, err := session.NewSession(
		&aws.Config{
			Region:     aws.String(awsRegion),
			MaxRetries: aws.Int(1),
			HTTPClient: &hc,
		})
	if err != nil {
		logger.WithError(err).Error("Unable to create an S3 client")
		return &Client{}, err
	}

	credValues, err := sess.Config.Credentials.Get()
	if err != nil {
		return &Client{}, fmt.Errorf("failed to obtain AWS credentials for values with error: %w, while creating s3 client", err)
	}
	logger.Infof("Obtaining AWS credentials by using [%s] as provider for s3 client", credValues.ProviderName)

	client := s3.New(sess)

	return &Client{
		s3:         client,
		bucketName: bucketName,
	}, err
}

func (c *Client) GetConceptAndTransactionID(ctx context.Context, publication string, UUID string) (bool, ontology.NewConcept, string, error) {
	key := getKey(UUID)
	if publication != "" {
		key = strings.Join([]string{publication, key}, "/")
	}

	getObjectParams := &s3.GetObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	}

	resp, err := c.s3.GetObjectWithContext(ctx, getObjectParams)
	if err != nil {
		e, ok := err.(awserr.Error)
		if ok && e.Code() == "NoSuchKey" {
			// NotFound rather than error, so no logging needed.
			return false, ontology.NewConcept{}, "", nil
		}
		logger.WithError(err).WithUUID(UUID).Error("Error retrieving concept from S3")
		return false, ontology.NewConcept{}, "", err
	}
	defer resp.Body.Close()

	getHeadersParams := &s3.HeadObjectInput{
		Bucket: aws.String(c.bucketName),
		Key:    aws.String(key),
	}
	ho, err := c.s3.HeadObjectWithContext(ctx, getHeadersParams)
	if err != nil {
		logger.WithError(err).WithUUID(UUID).Error("Cannot access S3 head object")
		return false, ontology.NewConcept{}, "", err
	}
	tid := ho.Metadata["Transaction_id"]

	var concept ontology.NewConcept
	if err = json.NewDecoder(resp.Body).Decode(&concept); err != nil {
		logger.WithError(err).WithUUID(UUID).Error("Cannot unmarshal object into a concept")
		return true, ontology.NewConcept{}, "", err
	}
	return true, concept, *tid, nil
}

func (c *Client) Healthcheck() fthealth.Check {
	return fthealth.Check{
		BusinessImpact:   "Editorial updates of concepts will not be written into UPP",
		Name:             "Check connectivity to S3 bucket",
		PanicGuide:       "https://runbooks.in.ft.com/aggregate-concept-transformer",
		Severity:         3,
		TechnicalSummary: `Cannot connect to S3 bucket. If this check fails, check that Amazon S3 is available`,
		Checker: func() (string, error) {
			params := &s3.HeadBucketInput{
				Bucket: aws.String(c.bucketName), // Required
			}
			_, err := c.s3.HeadBucket(params)
			if err != nil {
				logger.WithError(err).Error("Got error running S3 health check")
				return "", err
			}
			return "", err
		},
	}

}

func getKey(UUID string) string {
	return strings.Replace(UUID, "-", "/", -1)
}
