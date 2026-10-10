# chora-profile-conjurer

## About

chora-profile-conjurer is a Go ADK service with a single `conjurer` agent. It reads a learner's free-text bio and completed course titles, then returns taxonomy-constrained interest tags and a per-category proficiency map as JSON. The result is used by the C+ profiler and by the profile-tag fallback used for duel matchmaking.

## Quick start

Prerequisites:

- Go 1.26.6
- Access to a running Chora model gateway
- `CHORA_GATEWAY_TENANT_ID` and `CHORA_GATEWAY_GCID` for the process-level gateway identity

Clone and start the web-mode service:

```sh
git clone https://github.com/apollo-chora/chora-profile-conjurer.git
cd chora-profile-conjurer

export CHORA_GATEWAY_TENANT_ID="<tenant-uuid>"
export CHORA_GATEWAY_GCID="<user-gcid>"

go run ./cmd/profile_conjurer web -port 8080 agentengine
```

The gateway defaults to `gateway.chora.site:443`. For a plaintext local gateway, set `CHORA_GATEWAY_ENDPOINT` and `CHORA_GATEWAY_INSECURE=1`.

To build the binary instead:

```sh
go build -o profile_conjurer ./cmd/profile_conjurer
./profile_conjurer web -port 8080 agentengine
```

## Usage

The service runs through the ADK launcher in `web` mode and listens on port `8080` by default. The repository's caller contract creates a session through the `:query` endpoint with `class_method: async_create_session`, supplying the session state used to compose the prompt:

```text
state:
  tenant_id: "<tenant-uuid>"
  user_gcid: "<user-gcid>"
  bio: "<free-text bio>"
  course_titles_json: "[\"Intro to Physics\", \"Calculus I\"]"
  mana_tier: "standard"
```

The agent recomposes its instruction on every turn from the session state. `course_titles_json` is expected to be a JSON array of completed course titles.

The response is expected to be JSON only:

```json
{
  "tags": {
    "science": ["astronomy", "physics"],
    "mathematics": ["calculus"]
  },
  "proficiency": {
    "per_category": {
      "science": "beginner",
      "mathematics": "beginner"
    }
  }
}
```

Interest categories are `programming`, `mathematics`, `science`, `humanities`, `arts`, and `languages`. Tags are free-form slugs within those categories. Proficiency levels are `beginner`, `intermediate`, or `advanced`, stored only under `proficiency.per_category`.

Configuration:

| Variable | Default | Purpose |
| --- | --- | --- |
| `PROFILE_CONJURER_MODEL` | `longcat-2.5-preview` | Override the primary model |
| `CHORA_GATEWAY_ENDPOINT` | `gateway.chora.site:443` | Model-gateway gRPC target |
| `CHORA_GATEWAY_AUDIENCE` | `https://gateway.chora.site` | ID-token audience for the gateway |
| `CHORA_GATEWAY_TENANT_ID` | unset | Process-fallback tenant identity; required |
| `CHORA_GATEWAY_GCID` | unset | Process-fallback GCID; required |
| `CHORA_GATEWAY_INSECURE` | unset | Use plaintext gRPC for a local gateway when set to `1` |
| `PROFILE_CONJURER_SESSION_APP_NAME` | `chora-profile-conjurer` | ADK session `app_name` |
| `PORT` | `8080` | Web server port |
| `CHORA_ENV` | `dev` | Environment label: `dev`, `staging`, or `prod` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | stdout | OTLP/gRPC trace endpoint |
| `CHORA_SERVICE_VERSION` | `dev` | OpenTelemetry service version |

The embedded configuration in `internal/agentconfig/profile_conjurer.yaml` defines the `conjurer` sub-agent as `cheap`, using `longcat-2.5-preview` with `longcat-2.5-preview` as its fallback and prompt version `v1`.

Sessions are stored in memory, so deployments using multiple replicas are not supported by the current implementation. The service does not use NATS.

## Development

The repository is a Go module:

```sh
go mod download
go build ./...
go vet ./...
go test ./...
```

CI also runs `gofmt -l .` and checks that `go mod tidy` produces no changes to `go.mod` or `go.sum`.

Project layout:

| Path | Purpose |
| --- | --- |
| `cmd/profile_conjurer/` | Service entry point and binary tests |
| `internal/agent/` | Prompt composition, session-state mapping, condition extraction, and tests |
| `internal/agentconfig/` | Embedded per-agent model and prompt configuration |
| `internal/agent/testdata/` | Golden prompt test data |
| `.github/workflows/ci.yml` | Formatting, module consistency, vet, and test checks |
| `Dockerfile` | Multi-stage container build for the service |

The prompt composer has golden-file coverage and deterministic unit tests. The test suite does not require a broker, database, model gateway, or network connection.
