package concept

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	fthealth "github.com/Financial-Times/go-fthealth/v1_1"
	logger "github.com/Financial-Times/go-logger"

	"github.com/Financial-Times/aggregate-concept-transformer/concordances"
	"github.com/Financial-Times/aggregate-concept-transformer/kinesis"
	"github.com/Financial-Times/aggregate-concept-transformer/ontology"
	"github.com/Financial-Times/aggregate-concept-transformer/s3"
	"github.com/Financial-Times/aggregate-concept-transformer/sqs"
)

const (
	thingsAPIEndpoint  = "/things"
	conceptsAPIEnpoint = "/concepts"
)

var irregularConceptTypePaths = map[string]string{
	"AlphavilleSeries":            "alphaville-series",
	"BoardRole":                   "membership-roles",
	"Dummy":                       "dummies",
	"Person":                      "people",
	"PublicCompany":               "organisations",
	"NAICSIndustryClassification": "industry-classifications",
}

type Service interface {
	ListenForNotifications(ctx context.Context, workerID int)
	ProcessMessage(ctx context.Context, UUID string, bookmark string) error
	GetConcordedConcept(ctx context.Context, UUID string, bookmark string) (ontology.ConcordedConcept, string, error)
	Healthchecks() []fthealth.Check
}

type systemHealth struct {
	sync.RWMutex
	healthy  bool
	shutdown bool
	feedback <-chan bool
	done     <-chan struct{}
}

func (r *systemHealth) isGood() bool {
	r.RLock()
	defer r.RUnlock()
	return r.healthy
}

func (r *systemHealth) isShuttingDown() bool {
	r.RLock()
	defer r.RUnlock()
	return r.shutdown
}

func (r *systemHealth) processChannel() {
	for {
		select {
		case st := <-r.feedback:
			r.Lock()
			if st != r.healthy {
				logger.Warnf("Changing healthy status to '%t'", st)
				r.healthy = st
			}
			r.Unlock()
		case <-r.done:
			r.Lock()
			logger.Warn("Changing shutdown status to 'true'")
			r.shutdown = true
			r.Unlock()
		}
	}
}

type AggregateService struct {
	s3                              s3.Client
	concordances                    concordances.Client
	conceptUpdatesSqs               sqs.Client
	eventsSqs                       sqs.Client
	kinesis                         kinesis.Client
	neoWriterAddress                string
	varnishPurgerAddress            string
	elasticsearchWriterAddress      string
	httpClient                      httpClient
	typesToPurgeFromPublicEndpoints []string
	health                          *systemHealth
	processTimeout                  time.Duration
	readOnly                        bool
}

func NewService(
	S3Client s3.Client,
	conceptUpdatesSQSClient sqs.Client,
	eventsSQSClient sqs.Client,
	concordancesClient concordances.Client,
	kinesisClient kinesis.Client,
	neoAddress string,
	elasticsearchAddress string,
	varnishPurgerAddress string,
	typesToPurgeFromPublicEndpoints []string,
	httpClient httpClient,
	feedback <-chan bool,
	done <-chan struct{},
	processTimeout time.Duration,
	readOnly bool,
) *AggregateService {
	health := &systemHealth{
		healthy:  false, // Set to false. Once health check passes app will read from SQS
		shutdown: false,
		feedback: feedback,
		done:     done,
	}
	go health.processChannel()

	return &AggregateService{
		s3:                              S3Client,
		concordances:                    concordancesClient,
		conceptUpdatesSqs:               conceptUpdatesSQSClient,
		eventsSqs:                       eventsSQSClient,
		kinesis:                         kinesisClient,
		neoWriterAddress:                neoAddress,
		elasticsearchWriterAddress:      elasticsearchAddress,
		varnishPurgerAddress:            varnishPurgerAddress,
		httpClient:                      httpClient,
		typesToPurgeFromPublicEndpoints: typesToPurgeFromPublicEndpoints,
		health:                          health,
		processTimeout:                  processTimeout,
		readOnly:                        readOnly,
	}
}

func (s *AggregateService) ListenForNotifications(ctx context.Context, workerID int) {
	if s.readOnly {
		return
	}
	listenCtx, listenCancel := context.WithCancel(context.Background())
	defer listenCancel()
	for {
		select {
		case <-listenCtx.Done():
			logger.Infof("Stopping worker %d", workerID)
			return
		default:
			if s.health.isShuttingDown() {
				logger.Infof("Stopping worker %d", workerID)
				return
			}
			if !s.health.isGood() {
				continue
			}
			notifications := s.conceptUpdatesSqs.ListenAndServeQueue(listenCtx)
			nslen := len(notifications)
			if nslen <= 0 {
				continue
			}
			logger.Infof("Worker %d processing notifications", workerID)
			var wg sync.WaitGroup
			wg.Add(nslen)
			for _, n := range notifications {
				go func(ctx context.Context, reqWG *sync.WaitGroup, update sqs.ConceptUpdate) {
					defer reqWG.Done()
					err := s.processConceptUpdate(ctx, update)
					if err != nil {
						logger.WithError(err).WithUUID(n.UUID).Error("Error processing message.")
					}

				}(listenCtx, &wg, n)
			}
			wg.Wait()
		}
	}
}

func (s *AggregateService) processConceptUpdate(ctx context.Context, n sqs.ConceptUpdate) error {
	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, s.processTimeout)
	defer timeoutCancel()

	errCh := make(chan error)
	go func(ch chan<- error) {
		internalErr := s.ProcessMessage(timeoutCtx, n.UUID, n.Bookmark)
		if internalErr != nil {
			ch <- internalErr
			return
		}
		internalErr = s.conceptUpdatesSqs.RemoveMessageFromQueue(timeoutCtx, n.ReceiptHandle)
		if internalErr != nil {
			ch <- fmt.Errorf("error removing message from SQS: %w", internalErr)
			return
		}
		ch <- nil
	}(errCh)

	var err error
	select {
	case <-timeoutCtx.Done():
		err = timeoutCtx.Err()
	case err = <-errCh:
	}

	return err
}

func (s *AggregateService) ProcessMessage(ctx context.Context, UUID string, bookmark string) error {
	if s.readOnly {
		return errors.New("aggregate service is in read-only mode")
	}
	// Get the concorded concept
	concordedConcept, transactionID, err := s.GetConcordedConcept(ctx, UUID, bookmark)
	if err != nil {
		return err
	}
	if concordedConcept.PrefUUID != UUID {
		logger.WithTransactionID(transactionID).WithUUID(UUID).Infof("Requested concept %s is source node for canonical concept %s", UUID, concordedConcept.PrefUUID)
	}

	// Write to Neo4j
	logger.WithTransactionID(transactionID).WithUUID(concordedConcept.PrefUUID).Debug("Sending concept to Neo4j")
	conceptChanges, err := sendToWriter(ctx, s.httpClient, s.neoWriterAddress, resolveConceptType(concordedConcept.Type), concordedConcept.PrefUUID, concordedConcept, transactionID)
	if err != nil {
		return err
	}
	rawJson, err := json.Marshal(conceptChanges)
	if err != nil {
		logger.WithError(err).WithTransactionID(transactionID).WithUUID(concordedConcept.PrefUUID).Errorf("failed to marshall concept changes record: %v", conceptChanges)
		return err
	}
	var updateRecord sqs.ConceptChanges
	if err = json.Unmarshal(rawJson, &updateRecord); err != nil {
		logger.WithError(err).WithTransactionID(transactionID).WithUUID(concordedConcept.PrefUUID).Errorf("failed to unmarshall raw json into update record: %v", rawJson)
		return err
	}

	if len(updateRecord.ChangedRecords) < 1 {
		logger.WithTransactionID(transactionID).WithUUID(concordedConcept.PrefUUID).Info("concept was unchanged since last update, skipping!")
		return nil
	}
	logger.WithTransactionID(transactionID).WithUUID(concordedConcept.PrefUUID).Debug("concept successfully updated in neo4j")

	// Purge concept URLs in varnish
	// Always purge top level concept
	if err = sendToPurger(ctx, s.httpClient, s.varnishPurgerAddress, updateRecord.UpdatedIds, concordedConcept.Type, s.typesToPurgeFromPublicEndpoints, transactionID); err != nil {
		logger.WithTransactionID(transactionID).WithUUID(concordedConcept.PrefUUID).Errorf("Concept couldn't be purged from Varnish cache")
	}

	//optionally purge other affected concepts
	if concordedConcept.Type == "FinancialInstrument" {
		if err = sendToPurger(ctx, s.httpClient, s.varnishPurgerAddress, []string{concordedConcept.SourceRepresentations[0].IssuedBy}, "Organisation", s.typesToPurgeFromPublicEndpoints, transactionID); err != nil {
			logger.WithTransactionID(transactionID).WithUUID(concordedConcept.SourceRepresentations[0].IssuedBy).Errorf("Concept couldn't be purged from Varnish cache")
		}
	}

	if concordedConcept.Type == "Membership" {
		if err = sendToPurger(ctx, s.httpClient, s.varnishPurgerAddress, []string{concordedConcept.PersonUUID}, "Person", s.typesToPurgeFromPublicEndpoints, transactionID); err != nil {
			logger.WithTransactionID(transactionID).WithUUID(concordedConcept.PersonUUID).Errorf("Concept couldn't be purged from Varnish cache")
		}
	}

	// Write to Elasticsearch
	if isTypeAllowedInElastic(concordedConcept) {
		logger.WithTransactionID(transactionID).WithUUID(concordedConcept.PrefUUID).Debug("Writing concept to elastic search")
		if _, err = sendToWriter(ctx, s.httpClient, s.elasticsearchWriterAddress, resolveConceptType(concordedConcept.Type), concordedConcept.PrefUUID, concordedConcept, transactionID); err != nil {
			return err
		}
	}

	if err = s.eventsSqs.SendEvents(ctx, updateRecord.ChangedRecords); err != nil {
		logger.WithTransactionID(transactionID).WithUUID(concordedConcept.PersonUUID).Errorf("unable to send events: %v to Event Queue", updateRecord.ChangedRecords)
		return err
	}

	//Send notification to stream
	rawIDList, err := json.Marshal(conceptChanges.UpdatedIds)
	if err != nil {
		logger.WithError(err).WithTransactionID(transactionID).WithUUID(concordedConcept.PrefUUID).Errorf("failed to marshall concept changes record: %v", conceptChanges.UpdatedIds)
		return err
	}
	logger.WithTransactionID(transactionID).WithUUID(concordedConcept.PrefUUID).Debugf("sending notification of updated concepts to kinesis conceptsQueue: %v", conceptChanges)
	if err = s.kinesis.AddRecordToStream(ctx, rawIDList, concordedConcept.Type); err != nil {
		logger.WithError(err).WithTransactionID(transactionID).WithUUID(concordedConcept.PrefUUID).Errorf("Failed to update stream with notification record %v", conceptChanges)
		return err
	}
	logger.WithTransactionID(transactionID).WithUUID(concordedConcept.PrefUUID).Infof("Finished processing update of %s", UUID)

	return nil
}

func bucketConcordances(concordanceRecords []concordances.ConcordanceRecord) (map[string][]concordances.ConcordanceRecord, string, error) {
	if concordanceRecords == nil || len(concordanceRecords) == 0 {
		err := fmt.Errorf("no concordances provided")
		logger.WithError(err).Error("Error grouping concordance records")
		return nil, "", err
	}

	bucketedConcordances := map[string][]concordances.ConcordanceRecord{}
	for _, v := range concordanceRecords {
		bucketedConcordances[v.Authority] = append(bucketedConcordances[v.Authority], v)
	}

	var primaryAuthority string
	var err error
	slRecords, slFound := bucketedConcordances[ontology.SmartlogicAuthority]
	if slFound {
		if len(slRecords) == 1 {
			primaryAuthority = ontology.SmartlogicAuthority
		} else {
			err = fmt.Errorf("more than 1 primary authority")
		}
	}
	mlRecords, mlFound := bucketedConcordances[ontology.ManagedLocationAuthority]
	if mlFound {
		if len(mlRecords) == 1 {
			if primaryAuthority == "" {
				primaryAuthority = ontology.ManagedLocationAuthority
			}
		} else {
			err = fmt.Errorf("more than 1 ManagedLocation primary authority")
		}
	}
	if err != nil {
		logger.WithError(err).
			WithField("alert_tag", "AggregateConceptTransformerMultiplePrimaryAuthorities").
			WithField("primary_authorities", fmt.Sprintf("Smartlogic=%v, ManagedLocation=%v", slRecords, mlRecords)).
			Error("Error grouping concordance records")
		return nil, "", err
	}
	return bucketedConcordances, primaryAuthority, nil
}

func (s *AggregateService) GetConcordedConcept(ctx context.Context, UUID string, bookmark string) (ontology.ConcordedConcept, string, error) {
	type concordedData struct {
		Concept       ontology.ConcordedConcept
		TransactionID string
		Err           error
	}
	ch := make(chan concordedData)

	go func() {
		concept, tranID, err := s.getConcordedConcept(ctx, UUID, bookmark)
		ch <- concordedData{Concept: concept, TransactionID: tranID, Err: err}
	}()
	select {
	case data := <-ch:
		return data.Concept, data.TransactionID, data.Err
	case <-ctx.Done():
		return ontology.ConcordedConcept{}, "", ctx.Err()
	}
}

func (s *AggregateService) getConcordedConcept(ctx context.Context, UUID string, bookmark string) (ontology.ConcordedConcept, string, error) {
	var transactionID string
	var err error
	sources := []ontology.Concept{}

	concordedRecords, err := s.concordances.GetConcordance(ctx, UUID, bookmark)
	if err != nil {
		return ontology.ConcordedConcept{}, "", err
	}
	logger.WithField("UUID", UUID).Debugf("Returned concordance record: %v", concordedRecords)

	bucketedConcordances, primaryAuthority, err := bucketConcordances(concordedRecords)
	if err != nil {
		return ontology.ConcordedConcept{}, "", err
	}

	// Get all concepts from S3
	for authority, concordanceRecords := range bucketedConcordances {
		if authority == primaryAuthority {
			continue
		}
		for _, conc := range concordanceRecords {
			var found bool
			var sourceConcept ontology.Concept
			found, sourceConcept, transactionID, err = s.s3.GetConceptAndTransactionID(ctx, conc.UUID)
			if err != nil {
				return ontology.ConcordedConcept{}, "", err
			}

			if !found {
				//we should let the concorded concept to be written as a "Thing"
				logger.WithField("UUID", UUID).Warn(fmt.Sprintf("Source concept %s not found in S3", conc))
				sourceConcept.Authority = authority
				sourceConcept.AuthValue = conc.AuthorityValue
				sourceConcept.UUID = conc.UUID
				sourceConcept.Type = "Thing"
			}
			sources = append(sources, sourceConcept)

		}
	}

	if primaryAuthority != "" {
		canonicalConcept := bucketedConcordances[primaryAuthority][0]
		var found bool
		var primaryConcept ontology.Concept
		found, primaryConcept, transactionID, err = s.s3.GetConceptAndTransactionID(ctx, canonicalConcept.UUID)
		if err != nil {
			return ontology.ConcordedConcept{}, "", err
		} else if !found {
			err = fmt.Errorf("canonical concept %s not found in S3", canonicalConcept.UUID)
			logger.WithField("UUID", UUID).Error(err.Error())
			return ontology.ConcordedConcept{}, "", err
		}
		sources = append(sources, primaryConcept)
	}

	concordedConcept := ontology.CreateAggregateConcept(sources)

	return concordedConcept, transactionID, nil
}

func (s *AggregateService) Healthchecks() []fthealth.Check {
	checks := []fthealth.Check{
		s.s3.Healthcheck(),
		s.concordances.Healthcheck(),
	}
	if !s.readOnly {
		checks = append(checks, s.conceptUpdatesSqs.Healthcheck())
		checks = append(checks, s.RWElasticsearchHealthCheck())
		checks = append(checks, s.RWNeo4JHealthCheck())
		checks = append(checks, s.VarnishPurgerHealthCheck())
		checks = append(checks, s.kinesis.Healthcheck())
	}
	return checks
}

func sendToPurger(ctx context.Context, client httpClient, baseURL string, conceptUUIDs []string, conceptType string, conceptTypesWithPublicEndpoints []string, tid string) error {

	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(baseURL, "/")+"/purge", nil)
	if err != nil {
		return err
	}

	queryParams := req.URL.Query()
	for _, cUUID := range conceptUUIDs {
		queryParams.Add("target", thingsAPIEndpoint+"/"+cUUID)
		queryParams.Add("target", conceptsAPIEnpoint+"/"+cUUID)
	}

	if contains(conceptType, conceptTypesWithPublicEndpoints) {
		urlParam := resolveConceptType(conceptType)
		for _, cUUID := range conceptUUIDs {
			queryParams.Add("target", "/"+urlParam+"/"+cUUID)
		}
	}

	req.URL.RawQuery = queryParams.Encode()

	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("request was not successful, status code: %v", resp.StatusCode)
	}
	logger.WithTransactionID(tid).Debugf("Concepts with ids %s successfully purged from varnish cache", conceptUUIDs)

	return err
}

func contains(element string, types []string) bool {
	for _, t := range types {
		if element == t {
			return true
		}
	}
	return false
}

func sendToWriter(ctx context.Context, client httpClient, baseURL string, urlParam string, conceptUUID string, concept ontology.ConcordedConcept, tid string) (sqs.ConceptChanges, error) {
	updatedConcepts := sqs.ConceptChanges{}
	body, err := json.Marshal(concept)
	if err != nil {
		return updatedConcepts, err
	}

	request, reqURL, err := createWriteRequest(ctx, baseURL, urlParam, strings.NewReader(string(body)), conceptUUID)
	if err != nil {
		err = errors.New("Failed to create request to " + reqURL + " with body " + string(body))
		logger.WithTransactionID(tid).WithUUID(conceptUUID).Error(err)
		return updatedConcepts, err
	}
	request.ContentLength = -1
	request.Header.Set("X-Request-Id", tid)
	resp, err := client.Do(request)
	if err != nil {
		logger.WithError(err).WithTransactionID(tid).WithUUID(conceptUUID).Errorf("Request to %s returned error", reqURL)
		return updatedConcepts, err
	}

	defer resp.Body.Close()

	if strings.Contains(baseURL, "neo4j") && int(resp.StatusCode/100) == 2 {
		dec := json.NewDecoder(resp.Body)
		if err = dec.Decode(&updatedConcepts); err != nil {
			logger.WithError(err).WithTransactionID(tid).WithUUID(conceptUUID).Error("Error whilst decoding response from writer")
			return updatedConcepts, err
		}
	}

	if resp.StatusCode == 404 && strings.Contains(baseURL, "elastic") {
		logger.WithTransactionID(tid).WithUUID(conceptUUID).Debugf("Elastic search rw cannot handle concept: %s, because it has an unsupported type %s; skipping record", conceptUUID, concept.Type)
		return updatedConcepts, nil
	}
	if resp.StatusCode != 200 && resp.StatusCode != 304 {
		err := errors.New("Request to " + reqURL + " returned status: " + strconv.Itoa(resp.StatusCode) + "; skipping " + conceptUUID)
		logger.WithTransactionID(tid).WithUUID(conceptUUID).Errorf("Request to %s returned status: %d", reqURL, resp.StatusCode)
		return updatedConcepts, err
	}

	return updatedConcepts, nil
}

func createWriteRequest(ctx context.Context, baseURL string, urlParam string, msgBody io.Reader, uuid string) (*http.Request, string, error) {

	reqURL := strings.TrimRight(baseURL, "/") + "/" + urlParam + "/" + uuid

	request, err := http.NewRequestWithContext(ctx, "PUT", reqURL, msgBody)
	if err != nil {
		return nil, reqURL, fmt.Errorf("failed to create request to %s with body %s", reqURL, msgBody)
	}
	return request, reqURL, err
}

//Turn stored singular type to plural form
func resolveConceptType(conceptType string) string {
	if ipath, ok := irregularConceptTypePaths[conceptType]; ok && ipath != "" {
		return ipath
	}

	return toSnakeCase(conceptType) + "s"
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func toSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}-${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}-${2}")
	return strings.ToLower(snake)
}

func (s *AggregateService) RWNeo4JHealthCheck() fthealth.Check {
	return fthealth.Check{
		BusinessImpact:   "Editorial updates of concepts will not be written into UPP",
		Name:             "Check connectivity to concept-rw-neo4j",
		PanicGuide:       "https://runbooks.in.ft.com/aggregate-concept-transformer",
		Severity:         3,
		TechnicalSummary: `Cannot connect to concept writer neo4j. If this check fails, check health of concepts-rw-neo4j service`,
		Checker: func() (string, error) {
			urlToCheck := strings.TrimRight(s.neoWriterAddress, "/") + "/__gtg"
			req, err := http.NewRequest("GET", urlToCheck, nil)
			if err != nil {
				return "", err
			}
			resp, err := s.httpClient.Do(req)
			if err != nil {
				return "", fmt.Errorf("error calling writer at %s : %v", urlToCheck, err)
			}
			resp.Body.Close()
			if resp != nil && resp.StatusCode != http.StatusOK {
				return "", fmt.Errorf("writer %v returned status %d", urlToCheck, resp.StatusCode)
			}
			return "", nil
		},
	}
}

func (s *AggregateService) VarnishPurgerHealthCheck() fthealth.Check {
	return fthealth.Check{
		BusinessImpact:   "Editorial updates of concepts won't be immediately refreshed in the cache",
		Name:             "Check connectivity to varnish purger",
		PanicGuide:       "https://runbooks.in.ft.com/aggregate-concept-transformer",
		Severity:         3,
		TechnicalSummary: `Cannot connect to varnish purger. If this check fails, check health of varnish-purger service`,
		Checker: func() (string, error) {
			urlToCheck := strings.TrimRight(s.varnishPurgerAddress, "/") + "/__gtg"
			req, err := http.NewRequest("GET", urlToCheck, nil)
			if err != nil {
				return "", err
			}
			resp, err := s.httpClient.Do(req)
			if err != nil {
				return "", fmt.Errorf("error calling purger at %s : %v", urlToCheck, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return "", fmt.Errorf("purger %v returned status %d", urlToCheck, resp.StatusCode)
			}
			return "", nil
		},
	}
}

func (s *AggregateService) RWElasticsearchHealthCheck() fthealth.Check {
	return fthealth.Check{
		BusinessImpact:   "Editorial updates of concepts will not be written into UPP",
		Name:             "Check connectivity to concept-rw-elasticsearch",
		PanicGuide:       "https://runbooks.in.ft.com/aggregate-concept-transformer",
		Severity:         3,
		TechnicalSummary: `Cannot connect to elasticsearch concept writer. If this check fails, check health of concept-rw-elasticsearch service`,
		Checker: func() (string, error) {
			urlToCheck := strings.TrimRight(s.elasticsearchWriterAddress, "/bulk") + "/__gtg"
			req, err := http.NewRequest("GET", urlToCheck, nil)
			if err != nil {
				return "", err
			}
			resp, err := s.httpClient.Do(req)
			if err != nil {
				return "", fmt.Errorf("error calling writer at %s : %v", urlToCheck, err)
			}
			resp.Body.Close()
			if resp != nil && resp.StatusCode != http.StatusOK {
				return "", fmt.Errorf("writer %v returned status %d", urlToCheck, resp.StatusCode)
			}
			return "", nil
		},
	}
}

func isTypeAllowedInElastic(concordedConcept ontology.ConcordedConcept) bool {
	switch concordedConcept.Type {
	case "FinancialInstrument": //, "MembershipRole", "BoardRole":
		return false
	case "MembershipRole":
		return false
	case "BoardRole":
		return false
	case "Membership":
		for _, sr := range concordedConcept.SourceRepresentations {
			//Allow smartlogic curated memberships through to elasticsearch as we will use them to discover authors
			if sr.Authority == ontology.SmartlogicAuthority {
				return true
			}
		}
		return false
	case "IndustryClassification", "NAICSIndustryClassification":
		return false
	}

	return true
}
