# Control Room

Receives, validates, and stores event-driven data (heartbeats, status checks, users) from other microservices into Elasticsearch, visualized via Kibana.

## Stack

- Go 1.26
- RabbitMQ — `amqp091-go`
- Elasticsearch 9 — `go-elasticsearch/v9`
- Kibana 9.3.1
- Docker

## Requirements

- Docker Desktop running

## Run
```bash
# Start infrastructure
docker-compose up -d

# Start consumer
go run cmd/main.go
```

RabbitMQ UI: http://localhost:15672 (guest / guest)
Elasticsearch: http://localhost:9200
Kibana: http://localhost:5601

## Configuration

Secrets are managed via GitHub Secrets. The following secrets must be set in the repository settings:

| Secret | Description |
|---|---|
| `SERVER` | Hostname or IP of the deployment server |
| `PORT_APM` | APM server port |
| `PORT_ELASTICSEARCH` | Elasticsearch port |
| `PORT_KIBANA` | Kibana port |
| `PORT_LOGSTASH` | Logstash port |

## Kibana setup

1. Go to Stack Management > Data Views
2. Create data view `heartbeats`, timestamp field `@timestamp`
3. Create data view `users`, timestamp field `@timestamp`

