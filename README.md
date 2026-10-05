# chora-profile-conjurer

The `profile_conjurer` ADK Go agent crew (Pattern P1 Single: one conjurer
sub-agent). The conjurer reads a learner's free-text bio and completed-course
titles and emits taxonomy-constrained interest tags plus a per-category
proficiency map, as a single JSON object:

```json
{"tags": {"<category>": ["<tag>"]},
 "proficiency": {"per_category": {"<category>": "<level>"}}}
```

The output feeds the C+ profiler page (`profiler_profiles`) and, via the
profile-tag fallback, duel matchmaking. The conjurer's output IS the final
answer — no reflection pair, no sequential pipeline.

Module path: `github.com/apollo-chora/chora-profile-conjurer`.

The crew is cloud-neutral: LLM calls route through `chora-model-gateway`
(gRPC), traces go over standard OTLP, and nothing here requires a cloud
account or managed service.

## Layout

| Path | Purpose |
|---|---|
| `cmd/profile_conjurer/` | Service binary: wires the conjurer, plugins, and the ADK `agentengine` web-mode launcher. |
| `internal/agent/` | The composer (deterministic 6-block CREATE prompt), the session-state → `TaskContext` bridge, and the condition extractor. |
| `internal/agentconfig/` | Embedded per-sub-agent model tier + prompt-version YAML (single source of truth). |

## Operation

The binary serves the ADK launcher's `web` mode over HTTP. Callers create a
session via the `:query` endpoint with `class_method: async_create_session`
and pass `tenant_id`, `user_gcid`, `bio`, and `course_titles_json` in the
session state; the conjurer recomposes its instruction from that state on
every turn.

- **Port** — `8080` by default (ADK launcher `web` mode; override with
  `-port` / `PORT`).
- **Model gateway** — required for LLM calls. `CHORA_GATEWAY_ENDPOINT`
  (default `gateway.chora.site:443`); set `CHORA_GATEWAY_INSECURE=1` for a
  plaintext local gateway. `CHORA_GATEWAY_TENANT_ID` + `CHORA_GATEWAY_GCID`
  are the process-fallback tenant identity (per-request values come from
  session state via the tenant-propagation plugin).
- **Database** — none. Sessions are in-memory (`session.InMemoryService`),
  so the deployment must run with replicas=1.
- **NATS** — not used. This crew does not subscribe to the event-dispatch
  lane; termination events go to the service log via the logging publisher.
- **Traces** — standard OTLP/gRPC via `chora-common/otel`
  (`OTEL_EXPORTER_OTLP_ENDPOINT`); stdout fallback in local dev.

## Configuration

| Variable | Purpose | Local default |
| --- | --- | --- |
| `PROFILE_CONJURER_MODEL` | Override the conjurer primary model | `gemini-3.5-flash` (from `agentconfig`) |
| `CHORA_GATEWAY_ENDPOINT` | Model-gateway gRPC target | `gateway.chora.site:443` |
| `CHORA_GATEWAY_AUDIENCE` | ID-token audience for the gateway | `https://gateway.chora.site` |
| `CHORA_GATEWAY_TENANT_ID` | Process-fallback tenant (required) | unset |
| `CHORA_GATEWAY_GCID` | Process-fallback gcid (required) | unset |
| `CHORA_GATEWAY_INSECURE` | Plaintext gRPC to a local gateway (dev only) | unset |
| `PROFILE_CONJURER_SESSION_APP_NAME` | ADK session `app_name` | `chora-profile-conjurer` |
| `PORT` | Web server port (ADK launcher) | `8080` |
| `CHORA_ENV` | `dev` \| `staging` \| `prod` | `dev` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP/gRPC trace endpoint | stdout |
| `CHORA_SERVICE_VERSION` | OTel `service.version` attribute | `dev` |

Model tiering is agent-driven: the embedded
`internal/agentconfig/profile_conjurer.yaml` declares the conjurer as CHEAP
(`gemini-3.5-flash` → `gemini-2.5-flash` fallback). Mana is a token-budget
quota enforced at the gateway, not a model selector.

## Build and test

```sh
go build ./...
go vet ./...
go test ./...
```

The suite is hermetic — no broker, database, gateway, or network is required.
