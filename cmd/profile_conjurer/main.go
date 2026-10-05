// Command profile_conjurer is the entry point for the profile_conjurer crew
// (Pattern P1 Single: Conjurer) per crew-composition SKILL §1 + §2.
//
// Replaces chora-sharing's clients.StaticTagExtractor (the placeholder wiring
// with the "LLM extractor deferred" comment) and the unused in-process
// clients/tag_extractor.go. The conjurer emits taxonomy-constrained interest
// tags (from a free-text bio) AND infers proficiency (from completed course
// titles). Output feeds the C+ profiler page (profiler_profiles) and, via the
// existing enterQueue profile-tag fallback, duel matchmaking.
//
// Single-agent crew: the conjurer's output IS the final answer surfaced to the
// caller. No reflection pair, no sequential pipeline — one llmagent.New wired
// directly into adkagent.NewSingleLoader.
//
// Per ADR-138 §1 Go-first + ADR-145 polyglot agent-runtime pivot + ADR-146
// Model Broker full retirement + ADR-148 region pivot + ADR-169
// web-mode + ADR-177 full mana umbrella.
//
// Callers MUST create the session via the `:query` endpoint with
// `class_method: async_create_session` and pass:
//
//	state: {
//	  tenant_id:          "<tenant-uuid>",   // required by tenant-propagation
//	  user_gcid:          "<user-gcid>",     // required by tenant-propagation
//	  bio:                "<free-text bio>", // source of interest tags
//	  course_titles_json: "<json array>",   // source of proficiency inference
//	  mana_tier:          "standard",
//	}
//
// Env vars (NEVER inlined per feedback_no_inline_config):
//
//	PROFILE_CONJURER_MODEL          — override conjurer primary (default from agentconfig YAML: gemini-3.5-flash)
//	CHORA_GATEWAY_ENDPOINT        — model-gateway endpoint (default gateway.chora.site:443)
//	CHORA_GATEWAY_TENANT_ID       — process-fallback tenant (required; per-request from session state)
//	CHORA_GATEWAY_GCID            — process-fallback gcid (required; per-request from session state)
//	PROFILE_CONJURER_SESSION_APP_NAME — ADK session app_name (default chora-profile-conjurer)
//	PORT                          — web server port (ADK launcher reads it)
//	CHORA_ENV                     — dev | staging | prod
//
// AGENT-DRIVEN tiering (CR mana-is-quota-not-model-selector 2026-06-01): the
// per-sub-agent model tier + fallback chain is config-declared in the embedded
// agentconfig YAML (single source of truth), mirroring qgen / moderation.
// conjurer = CHEAP (bounded-label extraction). Mana is a token-budget QUOTA
// system (manaplugin gate) — it does NOT select the model; the
// tieredmodelplugin is no longer registered.
package main

import (
	"context"
	"log"
	"log/slog"
	"os"

	adkagent "google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/agentengine"
	"google.golang.org/adk/plugin"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"

	"github.com/apollo-chora/chora-adk-common/modelgatewayclient"
	"github.com/apollo-chora/chora-adk-common/promptstamping"
	"github.com/apollo-chora/chora-adk-common/terminationplugin"
	"github.com/apollo-chora/chora-adk-common/tracing"

	modagent "github.com/apollo-chora/chora-profile-conjurer/internal/agent"
	"github.com/apollo-chora/chora-profile-conjurer/internal/agentconfig"
)

const crewKind = "profile_conjurer"

// envOr returns os.Getenv(name) if non-empty, else fallback.
// Lesson learned from QGen Iter 2: adkgo plumbs only a handful of env
// vars at CreateReasoningEngine; Chora-specific vars are PATCHed in
// AFTER the initial start. Bootstrap must survive that window.
func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	ctx := context.Background()

	// Wire the OTel TracerProvider + W3C TraceContext propagator BEFORE any
	// agent / runner / plugin construction so every span is exported over
	// standard OTLP and continues an inbound traceparent. ADK Go does NOT
	// auto-wire this; per the trace wave 2026-05-29 every ADK agent calls
	// tracing.Init so it is traceable at per-agent level (service.name =
	// the registry name). chora-adk-common/tracing delegates to
	// chora-common/otel (OTLP/gRPC to OTEL_EXPORTER_OTLP_ENDPOINT, stdout
	// fallback in local dev) — no cloud backend is involved.
	traceShutdown, err := tracing.Init(ctx, "profile_conjurer")
	if err != nil {
		log.Fatalf("tracing.Init: %v", err)
	}
	defer func() {
		if err := traceShutdown(context.Background()); err != nil {
			slog.Error("trace shutdown error", "err", err)
		}
	}()

	// Session app_name for the ADK web-mode session service. The agentengine
	// launcher uses its constructor arg verbatim as the session AppName — in
	// the legacy managed agent runtime that arg was the reasoning-engine
	// resource, but post decommission (ADR-169, web-mode) there is no engine
	// ID, so an EMPTY arg makes session.InMemoryService().Create reject EVERY
	// session with "app_name and user_id are required, got app_name: \"\"". We
	// therefore pass a stable, non-empty app_name DECOUPLED from any runtime
	// resource id. Mirrors the proven chora-familiar / chora-moderation fix.
	sessionAppName := envOr("PROFILE_CONJURER_SESSION_APP_NAME", "chora-profile-conjurer")

	// AGENT-DRIVEN tiering (CR mana-is-quota-not-model-selector 2026-06-01):
	// per-sub-agent model + fallback chain are config-declared in the embedded
	// agentconfig YAML (single source of truth), mirroring qgen / moderation.
	// conjurer = CHEAP (gemini-3.5-flash → gemini-2.5-flash). An individual
	// primary may be overridden via env for quick ops experiments; fallback +
	// tier stay config-declared. This crew routes through chora-model-gateway
	// (ADR-177 full mana umbrella), so the model is selected fully agent-side
	// here and every LLM turn flows through the one chokepoint (central
	// Model Armor, per-tenant budget, token-usage ledger).
	profileConjurerCfg, err := agentconfig.ProfileConjurer()
	if err != nil {
		log.Fatalf("profile_conjurer: load agent config: %v", err)
	}
	conjurerCfg, err := profileConjurerCfg.Sub("conjurer")
	if err != nil {
		log.Fatalf("profile_conjurer: %v", err)
	}
	conjurerModel := envOr("PROFILE_CONJURER_MODEL", conjurerCfg.PrimaryModel)

	choraEnv := envOr("CHORA_ENV", "dev")

	slog.Info("profile_conjurer boot",
		"session_app_name", sessionAppName,
		"conjurer_model", conjurerModel,
		"conjurer_tier", conjurerCfg.Tier,
		"conjurer_fallback", conjurerCfg.FallbackModels,
		"chora_env", choraEnv,
	)

	// Per-sub-agent model — route through chora-model-gateway (ADR-177 full
	// mana umbrella). Replaces a direct Gemini call so the LLM turn
	// flows through the one chokepoint (central Model Armor, per-tenant
	// budget, token-usage ledger). Model selection stays AGENT-DRIVEN
	// (conjurer=CHEAP) via the agentconfig primary + forwarded fallback chain.
	// Per-request tenant/gcid come from session state via the propagation
	// plugin; env values are the process fallback.
	//
	// METERING (ADR-177 §FU-3): a profile_conjurer request is ONE billable unit. The
	// conjurer carries the action_code "profile_conjurer" so the gateway
	// debits exactly once per request. profile_conjurer is a learner-facing
	// extraction call — its mana price is registered in chora_identity's
	// mana_action_pricing (Step 7 of the plan).
	gatewayEndpoint := envOr("CHORA_GATEWAY_ENDPOINT", "gateway.chora.site:443")
	// D6 step 1: the ID-token audience is read HERE and defaulted explicitly.
	// modelgatewayclient still defaults it internally in TWO places
	// (client.go:181-182 and image.go:110-111); passing it makes the value
	// stateable and is what lets step 4 remove those defaults safely.
	gatewayAudience := envOr("CHORA_GATEWAY_AUDIENCE", "https://gateway.chora.site")
	gatewayTenantID := os.Getenv("CHORA_GATEWAY_TENANT_ID")
	gatewayGCID := os.Getenv("CHORA_GATEWAY_GCID")
	if gatewayTenantID == "" || gatewayGCID == "" {
		log.Fatalf("profile_conjurer: CHORA_GATEWAY_TENANT_ID + CHORA_GATEWAY_GCID required " +
			"(process fallback; per-request values come from session state — no silent " +
			"mis-attribution per ADR-169 + feedback_no_stubs_real_wiring)")
	}
	conjurerGemini, err := modelgatewayclient.New(ctx, conjurerGatewayConfig(conjurerCfg, conjurerModel, crewKind, gatewayEndpoint, gatewayGCID, gatewayTenantID, gatewayAudience))
	if err != nil {
		log.Fatalf("profile_conjurer: modelgatewayclient.New(conjurer, %s): %v", conjurerModel, err)
	}

	// Per-request tenant propagation (ADR-169) — stamp tenant_id/user_gcid from
	// session state onto every gateway Invoke. No ActionCodeResolver: the
	// action code is static per crew (set above).
	tenantPropP, err := modelgatewayclient.NewTenantPropagationPlugin(crewKind)
	if err != nil {
		log.Fatalf("profile_conjurer: modelgatewayclient.NewTenantPropagationPlugin: %v", err)
	}

	// Per-turn instruction composition (ADR-169 web-mode migration). The bio +
	// course titles + identity are unknown at boot, so the conjurer recomposes
	// its instruction from the session state the chora-sharing caller populates
	// at async_create_session (bio / course_titles_json / tenant_id /
	// user_gcid). InstructionProvider takes precedence over the static
	// Instruction field.
	conjurer, err := llmagent.New(llmagent.Config{
		Name:        "conjurer",
		Model:       conjurerGemini,
		Description: "Conjures a learner's taxonomy-constrained interest tags (from bio) + proficiency (from completed course titles). P1 Single crew; CHEAP tier bounded-label extraction.",
		// ADR-197 M-A: stamp prompt_version + content_hash + {role} on the span.
		InstructionProvider: promptstamping.WithStamping(
			conjurerCfg.PromptVersion,
			modagent.ProfilerConditions(modagent.RoleConjurer),
			modagent.NewInstructionProvider(modagent.RoleConjurer),
		),
	})
	if err != nil {
		log.Fatalf("llmagent.New(conjurer): %v", err)
	}

	// P1 Single — the conjurer is the whole crew. Wire it directly as the
	// single-loader agent (NO sequentialagent, NO loopagent). The conjurer's
	// output is the final answer surfaced to the caller.
	loader := adkagent.NewSingleLoader(conjurer)

	// NOTE: the tieredmodelplugin (ADR-149 mana × growth LLM matrix) is
	// DELIBERATELY NOT registered here (CR mana-is-quota-not-model-selector
	// 2026-06-01, user directive — mirrors qgen / moderation). Mana is a
	// token-budget QUOTA system (manaplugin gate, above) — it must NOT dictate
	// which LLM model is used. Model selection is AGENT-DRIVEN via the
	// per-sub-agent agentconfig YAML (conjurer=cheap gemini-3.5-flash), the
	// sub-agent getting its own gemini.NewModel at boot. See
	// feedback_mana_is_quota_not_model_selector + the CR tracker.

	// MaxIterations = 3 — P1 Single crew: one extraction turn plus a small
	// retry budget for malformed-JSON self-correction (per the Familiar/QGen
	// scaling pattern; the conjurer's output contract is a strict JSON shape).
	terminationP, err := terminationplugin.New(terminationplugin.Config{
		Publisher:     &terminationplugin.LoggingPublisher{},
		AgentID:       "profile_conjurer",
		Runtime:       "AGENT_EXECUTION_RUNTIME_ADK_GO",
		CrewKind:      crewKind,
		CrewPattern:   "P1_SINGLE",
		MaxIterations: 3,
	})
	if err != nil {
		log.Fatalf("terminationplugin.New: %v", err)
	}

	// SessionService — in-memory (ADR-169 / managed-agent-runtime → web-mode
	// migration 2026-06-01). The managed agent runtime is DECOMMISSIONED;
	// profile_conjurer runs as a plain Kubernetes Service (the agentengine
	// launcher's `web` mode serves HTTP). An in-memory session store removes
	// the dead runtime session dependency. In-memory sessions require
	// replicas=1.
	sessionService := session.InMemoryService()

	config := &launcher.Config{
		SessionService: sessionService,
		AgentLoader:    loader,
		PluginConfig: runner.PluginConfig{
			Plugins: []*plugin.Plugin{tenantPropP, terminationP},
		},
	}

	// NewLauncher's arg becomes the ADK session AppName (NOT a managed-runtime
	// RPC target in web mode — every method handler is constructed with this
	// value, so create/stream/delete agree on the session namespace). Pass the
	// decoupled sessionAppName so every session.Create + StreamQuery carries a
	// non-empty app_name (see above). Mirrors chora-familiar / chora-moderation.
	l := agentengine.NewLauncher(sessionAppName)
	if err := l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}

// crewSurface is the ADR-254 D7 surface value of this crew.
const crewSurface = "profile_conjurer"

// conjurerGatewayConfig is the gateway client identity of the profile_conjurer call. Surface is
// the crew id (ADR-254 D7): the D7 gateway refuses an unstamped Invoke
// (FAILED_PRECONDITION surface_unstamped), so it is set here, once, and
// asserted by a test; everything else is what the call always sent.
func conjurerGatewayConfig(conjurerCfg agentconfig.SubAgentConfig, conjurerModel string, crewKind string, gatewayEndpoint string, gatewayGCID string, gatewayTenantID string, gatewayAudience string) modelgatewayclient.Config {
	return modelgatewayclient.Config{
		Endpoint:         gatewayEndpoint,
		LogicalModelID:   conjurerModel,
		FallbackModelIDs: conjurerCfg.FallbackModels,
		AgentID:          "profile_conjurer",
		CrewKind:         crewKind,
		TenantID:         gatewayTenantID,
		GCID:             gatewayGCID,
		Audience:         gatewayAudience,
		ActionCode:       "profile_conjurer",
		Insecure:         os.Getenv("CHORA_GATEWAY_INSECURE") == "1",
		Surface:          crewSurface,
	}
}
