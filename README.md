# Control Room

Receives, validates, and stores event-driven data (heartbeats, status checks, users) from other microservices into Elasticsearch, visualized via Kibana.

## Stack

- Go 1.26
- RabbitMQ (`amqp091-go`)
- Elasticsearch 9 (`go-elasticsearch/v9`)
- Kibana 9.3.1

## Quick Start

```bash
docker compose up -d --build
```

### Local Testing

After the initial docker compose:

```bash
cd tests/deployments_test
docker compose up -d --build
```

## Access Points

- **RabbitMQ UI**: http://localhost:15672
- **Elasticsearch**: http://localhost:9200
- **Kibana**: http://localhost:5601

## Kibana Configuration

1. Go to **Stack Management** → **Data Views**
2. Create the following data views with `@timestamp` as the timestamp field:
   - `heartbeats`
   - `statuscheck`
   - `users`

## Testing

```bash
go test ./tests/...
```

## Hosted

[control-room.integration-project-2026-groep-2.my.be](https://control-room.integration-project-2026-groep-2.my.be)


## Developers

Marwan Makouh
Thomas Heusdens
Steven Deloof
Abdellah El Morabit


# Misc.

We developed an extra package for the fun of it that generates go structs based on a given XSD.
AI generation has been used for the file writing but the parsing and lexing is hand written.
This simplified  our workflow to just drag and dropping XSD's developped by other teams in the `pkg/xml` folder and
running the meta program.

We're hoping on expanding this package in the future to generate code based on exchange and queue declarations.
