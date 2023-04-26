# Aggregate Concept Transformer (aggregate-concept-transformer)

[![CircleCI](https://dl.circleci.com/status-badge/img/gh/Financial-Times/aggregate-concept-transformer/tree/master.svg?style=svg)](https://dl.circleci.com/status-badge/redirect/gh/Financial-Times/aggregate-concept-transformer/tree/master)
[![Go Report Card](https://goreportcard.com/badge/github.com/Financial-Times/aggregate-concept-transformer)](https://goreportcard.com/report/github.com/Financial-Times/aggregate-concept-transformer)
[![Coverage Status](https://coveralls.io/repos/github/Financial-Times/aggregate-concept-transformer/badge.svg?branch=master)](https://coveralls.io/github/Financial-Times/aggregate-concept-transformer?branch=master)

A service which gets notified via SQS of updates to source concepts in an Amazon S3 bucket. It then returns all UUIDs with concordance to said concept, requests each in turn from S3, builds the concorded JSON model and sends the updated concept JSON to both Neo4j and Elasticsearch. After the concept has successfully been written in Neo4j, the varnish-purger is called to invalidate the cache for the given concept. Finally it sends a notification of all updated concepts IDs to a Kinesis stream, and to a SNS topic.

## Installation

```shell
go get github.com/Financial-Times/aggregate-concept-transformer
cd $GOPATH/src/github.com/Financial-Times/aggregate-concept-transformer
go build -mod=readonly
```

## Running locally

```text
Usage: aggregate-concept-transformer [OPTIONS]

Aggregate and concord concepts in UPP.

Options:
  --app-system-code                   System Code of the application (env $APP_SYSTEM_CODE) (default "aggregate-concept-transformer")
  --app-name                          Application name (env $APP_NAME) (default "Aggregate Concept Transformer")
  --port                              Port to listen on (env $APP_PORT) (default 8080)
  --bucketName                        Bucket to read concepts from. (env $BUCKET_NAME)
  --bucketRegion                      AWS Region in which the S3 bucket is located (env $BUCKET_REGION) (default "eu-west-1")
  --conceptUpdatesQueueURL            Url of AWS SQS queue to listen for concept updates (env $CONCEPTS_QUEUE_URL)
  --sqsRegion                         AWS Region in which the SQS queue is located (env $SQS_REGION)
  --sqsEndpoint                       SQS queue endpoint (for local debugging only) (env $SQS_ENDPOINT)
  --messagesToProcess                 Maximum number or messages to concurrently read off of queue and process (env $MAX_MESSAGES) (default 10)
  --visibilityTimeout                 Duration(seconds) that messages will be ignored by subsequent requests after initial response (env $VISIBILITY_TIMEOUT) (default 30)
  --http-timeout                      Duration(seconds) to wait before timing out a request (env $HTTP_TIMEOUT) (default 15)
  --waitTime                          Duration(seconds) to wait on queue for messages until returning. Will be shorter if messages arrive (env $WAIT_TIME) (default 20)
  --neo4jWriterAddress                Address for the Neo4J Concept Writer (env $NEO_WRITER_ADDRESS) (default "http://localhost:8081/")
  --concordancesReaderAddress         Address for the Neo4J Concept Writer (env $CONCORDANCES_RW_ADDRESS) (default "http://localhost:8082/")
  --elasticsearchWriterAddress        Address for the Elasticsearch Concept Writer (env $ES_WRITER_ADDRESS) (default "http://localhost:8083/")
  --varnishPurgerAddress              Address for the Varnish Purger application (env $VARNISH_PURGER_ADDRESS) (default "http://localhost:8084/")
  --typesToPurgeFromPublicEndpoints   Concept types that need purging from specific public endpoints (other than /things) (env $TYPES_TO_PURGE_FROM_PUBLIC_ENDPOINTS) (default ["Person", "Brand", "Organisation", "PublicCompany"])
  --crossAccountRoleARN               ARN for cross account role (env $CROSS_ACCOUNT_ARN)
  --kinesisStreamName                 AWS Kinesis stream name (env $KINESIS_STREAM_NAME)
  --kinesisRegion                     AWS region the Kinesis stream is located (env $KINESIS_REGION) (default "eu-west-1")
  --requestLoggingOn                  Whether to log HTTP requests or not (env $REQUEST_LOGGING_ON) (default true)
  --logLevel                          App log level (env $LOG_LEVEL) (default "info")
  --read-only                         Start service in ready only mode (env $READ_ONLY)
  --conceptUpdatesSNSTopicArn         SNS Topic ARN in which concept updates are published (env $CONCEPT_UPDATES_SNS_ARN)
```

### Setup AWS credentials

The app assumes that you have correctly set up your AWS credentials by either using the `~/.aws/credentials` file:

```text
[default]
aws_access_key_id = AKID1234567890
aws_ secret_access_key = MY-SECRET-KEY
```

or the default AWS environment variables otherwise requests will return 401 Unauthorised

```shell
export AWS_ACCESS_KEY_ID=AKID1234567890
export AWS_SECRET_ACCESS_KEY=MY-SECRET-KEY
```

### Setup dependencies

Start a local emulation of the SQS server (most probably you don't want to connect to the real SQS queue because this may "steal" the messages from the running service in the cluster):

```shell
docker run --rm -p 4100:4100 --volume=`pwd`/goaws.yaml:/conf/goaws.yaml pafortin/goaws Local

export CONCEPTS_QUEUE_URL=http://localhost:4100/queue/concepts
export SQS_REGION=local
export SQS_ENDPOINT=http://localhost:4100
```

Setup all the necessary environment variables using the settings on Dev:

```shell
# You have to be logged to the Dev cluster before you can continue
# Get all necessary settings from the cluster and write them to a file
kubectl set env deploy aggregate-concept-transformer --list --resolve=true | grep "BUCKET_NAME\|KINESIS_STREAM_NAME\|KINESIS_REGION\|CROSS_ACCOUNT_ARN\|CONCEPT_UPDATES_SNS_ARN" > env_vars

# Export all variables from the file
set -a ; source env_vars ; set +a

# Delete the file now that it is no longer necessary
rm env_vars
```

Port-forward the necessary services:

```shell
kubectl port-forward svc/concordances-rw-neo4j 8082:8080

# The following are not needed if you will only test the transformation logic.
kubectl port-forward svc/concepts-rw-neo4j 8081:8080
kubectl port-forward svc/concept-rw-elasticsearch 8083:8080
kubectl port-forward svc/varnish-purger 8084:8080
```

### Run

Once all the above is completed you can simply run the application

```shell
./aggregate-concept-transformer
```

## Build and deployment

* Built by Docker Hub when merged to master: [coco/aggregate-concept-transformer](https://hub.docker.com/r/coco/aggregate-concept-transformer/)
* CI provided by CircleCI: [aggregate-concept-transformer](https://circleci.com/gh/Financial-Times/aggregate-concept-transformer)
* Code test coverage provided by Coveralls: [aggregate-concept-transformer](https://coveralls.io/github/Financial-Times/aggregate-concept-transformer)

## Aggregation

This service aggregates a number of source concepts into a single canonical view.  At present, the logic is as follows:

* All concorded/secondary concepts are merged together without any ordering.  These will always be from TME or Factset at the moment.
* The primary concept is then merged, overwriting the fields from the secondary concepts.  This is a Smartlogic concept.
* Aliases are the exception - they are merged between all concepts and de-duplicated.

## Endpoints

See [swagger.yml](api/swagger.yml).

## Admin Endpoints

* Healthchecks: `http://localhost:8080/__health`
* Good to go: `http://localhost:8080/__gtg`
* Build info: `http://localhost:8080/__build-info`

## Documentation

* Runbook: [Runbook](https://runbooks.in.ft.com/aggregate-concept-transformer)
