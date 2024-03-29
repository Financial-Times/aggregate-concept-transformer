package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	cli "github.com/jawher/mow.cli"
	log "github.com/sirupsen/logrus"

	logger "github.com/Financial-Times/go-logger"

	"github.com/Financial-Times/aggregate-concept-transformer/concept"
	"github.com/Financial-Times/aggregate-concept-transformer/concordances"
	"github.com/Financial-Times/aggregate-concept-transformer/kinesis"
	"github.com/Financial-Times/aggregate-concept-transformer/s3"
	"github.com/Financial-Times/aggregate-concept-transformer/sns"
	"github.com/Financial-Times/aggregate-concept-transformer/sqs"
)

const appDescription = "Service to aggregate concepts from different sources and produce a canonical view."

func main() {
	app := cli.App("aggregate-concept-service", "Aggregate and concord concepts in UPP")

	appSystemCode := app.String(cli.StringOpt{
		Name:   "app-system-code",
		Value:  "aggregate-concept-transformer",
		Desc:   "System Code of the application",
		EnvVar: "APP_SYSTEM_CODE",
	})
	appName := app.String(cli.StringOpt{
		Name:   "app-name",
		Value:  "Aggregate Concept Transformer",
		Desc:   "Application name",
		EnvVar: "APP_NAME",
	})
	port := app.Int(cli.IntOpt{
		Name:   "port",
		Value:  8080,
		Desc:   "Port to listen on",
		EnvVar: "APP_PORT",
	})
	bucketName := app.String(cli.StringOpt{
		Name:   "bucketName",
		Desc:   "Bucket to read concepts from.",
		EnvVar: "BUCKET_NAME",
	})
	bucketRegion := app.String(cli.StringOpt{
		Name:   "bucketRegion",
		Desc:   "AWS Region in which the S3 bucket is located",
		Value:  "eu-west-1",
		EnvVar: "BUCKET_REGION",
	})
	externalBucketName := app.String(cli.StringOpt{
		Name:   "externalBucketName",
		Desc:   "Bucket to read external concepts from.",
		EnvVar: "EXTERNAL_BUCKET_NAME",
	})
	externalBucketRegion := app.String(cli.StringOpt{
		Name:   "externalBucketRegion",
		Desc:   "AWS Region in which the external S3 bucket is located",
		Value:  "eu-west-1",
		EnvVar: "EXTERNAL_BUCKET_REGION",
	})
	conceptUpdatesQueueURL := app.String(cli.StringOpt{
		Name:   "conceptUpdatesQueueURL",
		Desc:   "Url of AWS SQS queue to listen for concept updates",
		EnvVar: "CONCEPTS_QUEUE_URL",
	})
	sqsRegion := app.String(cli.StringOpt{
		Name:   "sqsRegion",
		Desc:   "AWS Region in which the SQS queue is located",
		EnvVar: "SQS_REGION",
	})
	sqsEndpoint := app.String(cli.StringOpt{
		Name:   "sqsEndpoint",
		Desc:   "SQS queue endpoint (for local debugging only)",
		EnvVar: "SQS_ENDPOINT",
	})
	messagesToProcess := app.Int(cli.IntOpt{
		Name:   "messagesToProcess",
		Value:  10,
		Desc:   "Maximum number or messages to concurrently read off of queue and process",
		EnvVar: "MAX_MESSAGES",
	})
	visibilityTimeout := app.Int(cli.IntOpt{
		Name:   "visibilityTimeout",
		Value:  30,
		Desc:   "Duration(seconds) that messages will be ignored by subsequent requests after initial response",
		EnvVar: "VISIBILITY_TIMEOUT",
	})
	httpTimeout := app.Int(cli.IntOpt{
		Name:   "http-timeout",
		Value:  15,
		Desc:   "Duration(seconds) to wait before timing out a request",
		EnvVar: "HTTP_TIMEOUT",
	})
	waitTime := app.Int(cli.IntOpt{
		Name:   "waitTime",
		Value:  20,
		Desc:   "Duration(seconds) to wait on queue for messages until returning. Will be shorter if messages arrive",
		EnvVar: "WAIT_TIME",
	})
	neoWriterAddress := app.String(cli.StringOpt{
		Name:   "neo4jWriterAddress",
		Value:  "http://localhost:8081/",
		Desc:   "Address for the Neo4J Concept Writer",
		EnvVar: "NEO_WRITER_ADDRESS",
	})
	concordancesReaderAddress := app.String(cli.StringOpt{
		Name:   "concordancesReaderAddress",
		Value:  "http://localhost:8082/",
		Desc:   "Address for the Neo4J Concept Writer",
		EnvVar: "CONCORDANCES_RW_ADDRESS",
	})
	elasticsearchWriterAddress := app.String(cli.StringOpt{
		Name:   "elasticsearchWriterAddress",
		Value:  "http://localhost:8083/",
		Desc:   "Address for the Elasticsearch Concept Writer",
		EnvVar: "ES_WRITER_ADDRESS",
	})
	varnishPurgerAddress := app.String(cli.StringOpt{
		Name:   "varnishPurgerAddress",
		Value:  "http://localhost:8084/",
		Desc:   "Address for the Varnish Purger application",
		EnvVar: "VARNISH_PURGER_ADDRESS",
	})
	typesToPurgeFromPublicEndpoints := app.Strings(cli.StringsOpt{
		Name:   "typesToPurgeFromPublicEndpoints",
		Value:  []string{"Person", "Brand", "Organisation", "PublicCompany"},
		Desc:   "Concept types that need purging from specific public endpoints (other than /things)",
		EnvVar: "TYPES_TO_PURGE_FROM_PUBLIC_ENDPOINTS",
	})
	crossAccountRoleARN := app.String(cli.StringOpt{
		Name:      "crossAccountRoleARN",
		HideValue: true,
		Desc:      "ARN for cross account role",
		EnvVar:    "CROSS_ACCOUNT_ARN",
	})
	kinesisStreamName := app.String(cli.StringOpt{
		Name:   "kinesisStreamName",
		Desc:   "AWS Kinesis stream name",
		EnvVar: "KINESIS_STREAM_NAME",
	})
	kinesisRegion := app.String(cli.StringOpt{
		Name:   "kinesisRegion",
		Value:  "eu-west-1",
		Desc:   "AWS region the Kinesis stream is located",
		EnvVar: "KINESIS_REGION",
	})
	requestLoggingOn := app.Bool(cli.BoolOpt{
		Name:   "requestLoggingOn",
		Value:  true,
		Desc:   "Whether to log HTTP requests or not",
		EnvVar: "REQUEST_LOGGING_ON",
	})
	logLevel := app.String(cli.StringOpt{
		Name:   "logLevel",
		Value:  "info",
		Desc:   "App log level",
		EnvVar: "LOG_LEVEL",
	})
	isReadOnly := app.Bool(cli.BoolOpt{
		Name:   "read-only",
		Desc:   "Start service in ready only mode",
		EnvVar: "READ_ONLY",
		Value:  false,
	})
	conceptUpdatesSNSTopicArn := app.String(cli.StringOpt{
		Name:   "conceptUpdatesSNSTopicArn",
		Value:  "",
		Desc:   "SNS Topic ARN in which concept updates are published",
		EnvVar: "CONCEPT_UPDATES_SNS_ARN",
	})

	app.Before = func() {
		logger.InitLogger(*appSystemCode, *logLevel)

		logger.WithFields(log.Fields{
			"ES_WRITER_ADDRESS":       *elasticsearchWriterAddress,
			"CONCORDANCES_RW_ADDRESS": *concordancesReaderAddress,
			"NEO_WRITER_ADDRESS":      *neoWriterAddress,
			"VARNISH_PURGER_ADDRESS":  *varnishPurgerAddress,
			"EXTERNAL_BUCKET_REGION":  *externalBucketRegion,
			"EXTERNAL_BUCKET_NAME":    *externalBucketName,
			"BUCKET_REGION":           *bucketRegion,
			"BUCKET_NAME":             *bucketName,
			"SQS_REGION":              *sqsRegion,
			"CONCEPTS_QUEUE_URL":      *conceptUpdatesQueueURL,
			"LOG_LEVEL":               *logLevel,
			"KINESIS_STREAM_NAME":     *kinesisStreamName,
			"CONCEPT_UPDATES_SNS_ARN": *conceptUpdatesSNSTopicArn,
		}).Info("Starting app with arguments")

		if *bucketName == "" {
			logger.Fatal("S3 bucket name not set")
		}
		if *bucketRegion == "" {
			logger.Fatal("AWS bucket region not set")
		}
		if *concordancesReaderAddress == "" {
			logger.Fatal("Concordances reader address not set")
		}

		if !*isReadOnly {
			if *conceptUpdatesQueueURL == "" {
				logger.Fatal("Concept update SQS queue URL not set")
			}

			if *sqsRegion == "" {
				logger.Fatal("AWS SQS region not set")
			}

			if *kinesisStreamName == "" {
				logger.Fatal("Kinesis stream name not set")
			}
		}
	}

	app.Action = func() {
		s3Client, err := s3.NewClient(*bucketName, *bucketRegion)
		if err != nil {
			logger.WithError(err).Fatal("Error creating S3 client for concept-normalised-store")
		}

		externalS3Client, err := s3.NewClient(*externalBucketName, *externalBucketRegion)
		if err != nil {
			logger.WithError(err).Fatal("Error creating S3 client for external-concept-normalised-store")
		}

		concordancesClient, err := concordances.NewClient(*concordancesReaderAddress)
		if err != nil {
			logger.WithError(err).Fatal("Error creating Concordances client")
		}

		var conceptUpdatesSqsClient sqs.Client
		var eventsSNS sns.Client
		var kinesisClient kinesis.Client

		if !*isReadOnly {
			conceptUpdatesSqsClient, err = sqs.NewClient(*sqsRegion, *conceptUpdatesQueueURL, *sqsEndpoint, *messagesToProcess, *visibilityTimeout, *waitTime)
			if err != nil {
				logger.WithError(err).Fatal("Error creating concept updates SQS client")
			}

			eventsSNS, err = sns.NewClient(*conceptUpdatesSNSTopicArn)
			if err != nil {
				logger.WithError(err).Fatal("Error creating concept events SNS client")
			}

			kinesisClient, err = kinesis.NewClient(*kinesisStreamName, *kinesisRegion, *crossAccountRoleARN)
			if err != nil {
				logger.WithError(err).Fatal("Error creating Kinesis client")
			}
		}

		feedback := make(chan bool)
		done := make(chan struct{})

		maxWorkers := runtime.GOMAXPROCS(0) + 1
		if *isReadOnly {
			maxWorkers = 0
		}
		requestTimeout := time.Second * time.Duration(*httpTimeout)
		svc := concept.NewService(
			s3Client,
			externalS3Client,
			conceptUpdatesSqsClient,
			eventsSNS,
			concordancesClient,
			kinesisClient,
			*neoWriterAddress,
			*elasticsearchWriterAddress,
			*varnishPurgerAddress,
			*typesToPurgeFromPublicEndpoints,
			defaultHTTPClient(maxWorkers),
			feedback,
			done,
			requestTimeout,
			*isReadOnly)

		handler := concept.NewHandler(svc, requestTimeout)
		hs := concept.NewHealthService(svc, *appSystemCode, *appName, *port, appDescription)

		serveMux := handler.RegisterHandlers(hs, *requestLoggingOn, feedback)

		logger.Infof("Running %d ListenForNotifications", maxWorkers)
		var listenForNotificationsWG sync.WaitGroup
		listenForNotificationsWG.Add(maxWorkers)

		workerCtx, workerCancel := context.WithCancel(context.Background())

		for i := 0; i < maxWorkers; i++ {
			go func(workerId int) {
				logger.Infof("Starting ListenForNotifications worker %d", workerId)
				svc.ListenForNotifications(workerCtx, workerId)
				listenForNotificationsWG.Done()
			}(i)
		}

		logger.Infof("Listening on port %v", *port)
		srv := &http.Server{
			Addr: fmt.Sprintf(":%d", *port),
			// Good practice to set timeouts to avoid Slowloris attacks.
			WriteTimeout: time.Second * 15,
			ReadTimeout:  time.Second * 15,
			IdleTimeout:  time.Second * 60,
			Handler:      serveMux,
		}

		// Run our server in a goroutine so that it doesn't block.
		go func() {
			if err := srv.ListenAndServe(); err != http.ErrServerClosed {
				logger.Fatalf("Unexpected error during server shutdown: %v", err)
			}
		}()

		c := make(chan os.Signal, 1)
		// We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
		// or SIGTERM (Ctrl+/).
		// SIGKILL or SIGQUIT will not be caught.
		signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)

		// Block until we receive our signal.
		<-c
		logger.Info("Interruption signal received, shutting down")
		// Send done signal to service
		workerCancel()
		done <- struct{}{}
		logger.Info("Waiting for workers to stop")
		listenForNotificationsWG.Wait()
		// Create a deadline to wait for.
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*waitTime)*time.Second)
		defer cancel()

		// Doesn't block if no connections, but will otherwise wait
		// until the timeout deadline.
		logger.Info("Shutting down HTTP server")
		srv.Shutdown(ctx)
		logger.Info("Exiting application")
		cli.Exit(0)
	}
	app.Run(os.Args)
}

func defaultHTTPClient(maxWorkers int) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   90 * time.Second,
				KeepAlive: 60 * time.Second,
				DualStack: true,
			}).DialContext,
			MaxIdleConns:          128,
			MaxIdleConnsPerHost:   maxWorkers + 1,   // one more than needed
			IdleConnTimeout:       90 * time.Second, // from DefaultTransport
			TLSHandshakeTimeout:   10 * time.Second, // from DefaultTransport
			ExpectContinueTimeout: 1 * time.Second,  // from DefaultTransport
		},
	}
}
