# Control Room

Receives, validates, and stores event-driven data (heartbeats, status checks, users) from other microservices into Elasticsearch, visualized via Kibana.

## Stack

- Go 1.26
- RabbitMQ (`amqp091-go`)
- Elasticsearch 9 (`go-elasticsearch/v9`)
- Kibana 9.3.1
- Go MCP SDK

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
   - `checkins`

## IoT RabbitMQ Consumer Documentation

The Controlroom service also consumes IoT badge scanner check-in events from RabbitMQ. These events are handled by the `internal/checkin` package and indexed into Elasticsearch for later analytics and MCP queries.

- Consumer package: `internal/checkin`
- Elasticsearch index: `checkins`
- Message format: XML
- Expected root element: `<CheckIn>`
- Required fields:
  - `<id>`: string identifier for the badge/user
  - `<timestamp>`: RFC3339 datetime

Example payload:

```xml
<CheckIn>
  <id>12345</id>
  <timestamp>2026-05-19T14:36:33</timestamp>
</CheckIn>
```

Schema definition (XSD):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<xs:schema xmlns:xs="http://www.w3.org/2001/XMLSchema"
           elementFormDefault="qualified">

    <xs:element name="CheckIn">
        <xs:complexType>
            <xs:sequence>
                <xs:element name="id" type="xs:string"/>
                <xs:element name="timestamp" type="xs:dateTime"/>
            </xs:sequence>
        </xs:complexType>
    </xs:element>

</xs:schema>
```

This fits the existing Controlroom architecture by providing a lightweight IoT event source that is parsed from XML and stored as structured documents in Elasticsearch. The check-in events can then be queried or summarized through the same monitoring and MCP tooling used by the rest of the service.

## MCP Integration

Controlroom exposes an MCP server for intelligent querying and GitHub-based tooling. The MCP server is initialized in `cmd/controlroom/main.go` by calling `mcp.SetupMCP(client)` concurrently with the RabbitMQ session.

- MCP package: `internal/mcp`
- GitHub helper package: `internal/cr_github`
- Server name: `controlroom-mcp`
- Server version: `1.0.0`

### Required environment variables

- `MCP_PORT` - port where the MCP HTTP stream server listens
- `ORG` - GitHub organization used for repo discovery
- `GITHUB_TOKEN` - personal access token for authenticated GitHub API calls

### Supported MCP tools

- `error_analysis`: query `controlroom-logs` by Lucene query string
- `heartbeat_status`: read recent heartbeat events from `heartbeats`
- `statuscheck_summary`: read statuscheck events from `statuscheck`
- `fetch_logs`: fetch ERROR/WARN logs for a service from `controlroom-logs`
- `fetch_recent_deploys`: fetch recent GitHub Actions deploy runs for a service repo
- `fetch_recent_commits`: fetch recent GitHub commits for a service repo
- `fetch_blob`: retrieve file content by Git blob SHA
- `request_changes`: create a pull request on GitHub

### GitHub service mapping

The MCP GitHub tools map service names to repositories by fetching all repos for the configured `ORG` and matching on the lowercase repo name. The helper code currently documents the following service keys:

- `kassa`
- `crm`
- `controlroom`
- `frontend`
- `mailing`
- `facturatie`
- `planning`
- `iot`
- `mcp-master`

If the repository for `iot` or `mcp-master` exists in the configured GitHub org, MCP can use it for deploy, commit, and blob operations.

## Testing

```bash
go test ./tests/...
```

Running tests individually requires you to pass helper.go as a second file argument because it contains helper functions that the tests depend on.

## Hosted

[control-room.integration-project-2026-groep-2.my.be](https://control-room.integration-project-2026-groep-2.my.be)


## Developers

- Marwan Makouh
- Thomas Heusdens
- Steven Deloof
- Abdellah El Morabit


# Misc.

We developed an extra package for the fun of it that generates go structs based on a given XSD.
AI generation has been used for the file writing but the parsing and lexing is hand written.
This simplified  our workflow to just drag and dropping XSD's developped by other teams in the `pkg/xml` folder and
running the meta program.

(won't do this anymore)
We're hoping on expanding this package in the future to generate code based on exchange and queue declarations.


## Extra

Recently we've added an mcp integration for querying information out of elastic and giving cool AI summaries.


