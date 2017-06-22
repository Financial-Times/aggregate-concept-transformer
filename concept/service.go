package concept

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Financial-Times/aggregate-concept-transformer/dynamodb"
	"github.com/Financial-Times/aggregate-concept-transformer/s3"
	"github.com/Financial-Times/aggregate-concept-transformer/sqs"
	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	log "github.com/Sirupsen/logrus"
)

type Service interface {
	ListenForNotifications()
	ProcessMessage(notification sqs.Notification) error
	GetConcordedConcept(UUID string) (ConcordedConcept, string, error)
	Healthchecks() []fthealth.Check
}

type AggregateService struct {
	s3                         s3.Client
	db                         dynamodb.Client
	sqs                        sqs.Client
	neoWriterAddress           string
	elasticsearchWriterAddress string
	httpClient                 httpClient
}

func NewService(S3Client s3.Client, SQSClient sqs.Client, dynamoClient dynamodb.Client, neoAddress string, elasticsearchAddress string) Service {
	return &AggregateService{
		s3:                         S3Client,
		db:                         dynamoClient,
		sqs:                        SQSClient,
		neoWriterAddress:           neoAddress,
		elasticsearchWriterAddress: elasticsearchAddress,
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxIdleConnsPerHost: 128,
				Dial: (&net.Dialer{
					Timeout:   30 * time.Second,
					KeepAlive: 30 * time.Second,
				}).Dial,
			},
		},
	}
}

func (s *AggregateService) ListenForNotifications() {
	for {
		notifications := s.sqs.ListenAndServeQueue()
		if len(notifications) > 0 {
			var wg sync.WaitGroup
			wg.Add(len(notifications))
			for _, n := range notifications {
				go func(n sqs.Notification) {
					defer wg.Done()
					s.ProcessMessage(n)
				}(n)
			}
		}
	}
}

func (s *AggregateService) ProcessMessage(notification sqs.Notification) error {

	// Get the concorded concept
	concordedConcept, transactionID, err := s.GetConcordedConcept(notification.UUID)
	if err != nil {
		return err
	}

	// Write to Neo4j
	err = sendToWriter(s.httpClient, s.neoWriterAddress, resolveConceptType(concordedConcept.Type), concordedConcept.PrefUUID, concordedConcept, transactionID)
	if err != nil {
		return err
	}

	// Write to elastic search
	err = sendToWriter(s.httpClient, s.elasticsearchWriterAddress, resolveConceptType(concordedConcept.Type), concordedConcept.PrefUUID, concordedConcept, transactionID)
	if err != nil {
		return err
	}

	err = s.sqs.RemoveMessageFromQueue(notification.ReceiptHandle)
	if err != nil {
		return err
	}

	log.WithFields(log.Fields{
		"UUID":          concordedConcept.PrefUUID,
		"TransactionID": transactionID,
	}).Info("Finished processing update")

	return nil
}

func (s *AggregateService) GetConcordedConcept(UUID string) (ConcordedConcept, string, error) {
	concordedConcept := ConcordedConcept{}
	// Get concordance UUIDs.
	concordances, err := s.db.GetConcordance(UUID)
	if err != nil {
		log.WithField("UUID", UUID).Error("Can't get concordance from DynamoDB")
		return ConcordedConcept{}, "", err
	}

	// Get all concepts from S3.
	for _, UUID := range concordances.ConcordedIds {
		found, s3Concept, _, err := s.s3.GetConceptAndTransactionId(UUID)

		if err != nil {
			log.WithError(err).WithField("UUID", UUID).Error("Error getting concept from S3")
			return ConcordedConcept{}, "", err
		}
		if !found {
			return ConcordedConcept{}, "", errors.New("Not found")
		}

		//allConcepts = append(allConcepts, s3Concept)
		concordedConcept = mergeCanonicalInformation(concordedConcept, s3Concept)
	}

	found, primaryConcept, transactionID, err := s.s3.GetConceptAndTransactionId(concordances.UUID)
	if err != nil {
		return ConcordedConcept{}, "", err
	}
	if !found {
		return ConcordedConcept{}, "", errors.New("Not found")
	}

	// Aggregate concepts
	concordedConcept = mergeCanonicalInformation(concordedConcept, primaryConcept)
	return concordedConcept, transactionID, nil
}

func (s *AggregateService) Healthchecks() []fthealth.Check {
	return []fthealth.Check{
		s.s3.Healthcheck(),
		s.sqs.Healthcheck(),
		s.RWElasticsearchHealthCheck(),
		s.RWNeo4JHealthCheck(),
	}
}

func mergeCanonicalInformation(c ConcordedConcept, s s3.Concept) ConcordedConcept {
	c.PrefUUID = s.UUID
	c.PrefLabel = s.PrefLabel
	c.Type = s.Type
	c.Aliases = s.Aliases
	c.Strapline = s.Strapline
	c.DescriptionXML = s.DescriptionXML
	c.ImageURL = s.ImageURL
	c.ParentUUIDs = s.ParentUUIDs
	c.SourceRepresentations = append(c.SourceRepresentations, s)
	return c
}

func sendToWriter(client httpClient, baseUrl string, urlParam string, conceptUUID string, concept ConcordedConcept, tid string) error {

	body, err := json.Marshal(concept)
	if err != nil {
		return err
	}

	request, reqUrl, err := createWriteRequest(baseUrl, urlParam, strings.NewReader(string(body)), conceptUUID)
	if err != nil {
		return errors.New("Failed to create request to " + reqUrl + " with body " + string(body))
	}
	request.ContentLength = -1
	request.Header.Set("X-Request-Id", tid)

	resp, reqErr := client.Do(request)

	if reqErr != nil || resp.StatusCode != 200 {
		return errors.New("Request to " + reqUrl + " returned status: " + strconv.Itoa(resp.StatusCode) + "; skipping " + conceptUUID)
	}
	defer resp.Body.Close()

	return nil
}

func createWriteRequest(baseUrl string, urlParam string, msgBody io.Reader, uuid string) (*http.Request, string, error) {

	reqURL := strings.TrimRight(baseUrl, "/") + "/" + urlParam + "/" + uuid

	request, err := http.NewRequest("PUT", reqURL, msgBody)
	if err != nil {
		return nil, reqURL, fmt.Errorf("Failed to create request to %s with body %s", reqURL, msgBody)
	}
	return request, reqURL, err
}

//Turn stored singular type to plural form
func resolveConceptType(conceptType string) string {
	var messageType string
	switch conceptType {
	case "person":
		messageType = "people"
	case "alphavilleseries":
		messageType = "alphaville-series"
	case "specialreport":
		messageType = "special-reports"
	default:
		messageType = conceptType + "s"
	}
	return messageType
}

func (s *AggregateService) RWNeo4JHealthCheck() fthealth.Check {
	return fthealth.Check{
		BusinessImpact:   "Editorial updates of concepts will not be written into UPP",
		Name:             "Check connectivity to concept-rw-neo4j",
		PanicGuide:       "https://dewey.ft.com/aggregate-concept-transformer.html",
		Severity:         2,
		TechnicalSummary: `Cannot connect to concept writer neo4j. If this check fails, check health of concepts-rw-neo4j service`,
		Checker: func() (string, error) {
			urlToCheck := strings.TrimRight(s.neoWriterAddress, "/") + "/__gtg"
			resp, err := http.Get(urlToCheck)
			if err != nil {
				return "", fmt.Errorf("Error calling writer at %s : %v", urlToCheck, err)
			}
			resp.Body.Close()
			if resp != nil && resp.StatusCode != http.StatusOK {
				return "", fmt.Errorf("Writer %v returned status %d", urlToCheck, resp.StatusCode)
			}
			return "", nil
		},
	}
}

func (s *AggregateService) RWElasticsearchHealthCheck() fthealth.Check {
	return fthealth.Check{
		BusinessImpact:   "Editorial updates of concepts will not be written into UPP",
		Name:             "Check connectivity to concept-rw-elasticsearch",
		PanicGuide:       "https://dewey.ft.com/aggregate-concept-transformer.html",
		Severity:         2,
		TechnicalSummary: `Cannot connect to elasticsearch concept writer. If this check fails, check health of concept-rw-elasticsearch service`,
		Checker: func() (string, error) {
			urlToCheck := strings.TrimRight(s.elasticsearchWriterAddress, "/") + "/__gtg"
			resp, err := http.Get(urlToCheck)
			if err != nil {
				return "", fmt.Errorf("Error calling writer at %s : %v", urlToCheck, err)
			}
			resp.Body.Close()
			if resp != nil && resp.StatusCode != http.StatusOK {
				return "", fmt.Errorf("Writer %v returned status %d", urlToCheck, resp.StatusCode)
			}
			return "", nil
		},
	}
}