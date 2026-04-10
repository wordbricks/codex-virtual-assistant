package prompting

import (
	"strings"
	"testing"
	"time"

	"github.com/siisee11/CodexVirtualAssistant/internal/assistant"
)

func TestBuildPlannerPromptDeclaresStrictJSONContract(t *testing.T) {
	t.Parallel()

	bundle := BuildPlannerPrompt(PlannerInput{
		UserRequestRaw:        "Research five competitors and build a comparison table.",
		MaxGenerationAttempts: 4,
		Project: assistant.ProjectContext{
			Slug:         "competitor-pricing",
			Name:         "Competitor Pricing",
			Description:  "Track competitor pricing research.",
			WorkspaceDir: "/tmp/projects/competitor-pricing",
			WikiDir:      "/tmp/projects/competitor-pricing/wiki",
		},
		Wiki: assistant.WikiContext{
			Enabled:         true,
			OverviewSummary: "We already track SaaS competitor pricing and past comparisons.",
			IndexSummary:    "topics/pricing.md and reports/run-1.md are relevant.",
		},
	})

	if !strings.Contains(bundle.System, "strict JSON object") {
		t.Fatalf("System prompt = %q, want strict JSON instruction", bundle.System)
	}
	if !strings.Contains(bundle.System, "tools_allowed") || !strings.Contains(bundle.System, "done_definition") {
		t.Fatalf("System prompt = %q, want required planner keys", bundle.System)
	}
	if !strings.Contains(bundle.System, "schedule_plan") {
		t.Fatalf("System prompt = %q, want schedule_plan guidance", bundle.System)
	}
	if !strings.Contains(bundle.User, "Default max generation attempts: 4") {
		t.Fatalf("User prompt = %q, want attempt count", bundle.User)
	}
	if !strings.Contains(bundle.User, "Project wiki context") || !strings.Contains(bundle.User, "Overview: We already track SaaS competitor pricing") {
		t.Fatalf("User prompt = %q, want wiki context summary", bundle.User)
	}
}

func TestBuildProjectSelectorPromptRequiresProjectInspection(t *testing.T) {
	t.Parallel()

	bundle := BuildProjectSelectorPrompt(ProjectSelectorInput{
		UserRequestRaw: "Estimate our company's infrastructure costs next quarter.",
	})

	if !strings.Contains(bundle.System, "projects/*/PROJECT.md") {
		t.Fatalf("System prompt = %q, want PROJECT.md inspection guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "projects/*/wiki/overview.md") || !strings.Contains(bundle.System, "projects/*/wiki/index.md") {
		t.Fatalf("System prompt = %q, want wiki inspection guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "\"no_project\"") {
		t.Fatalf("System prompt = %q, want no_project guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "project_slug") {
		t.Fatalf("System prompt = %q, want selector schema keys", bundle.System)
	}
	if !strings.Contains(bundle.System, "enduring domain") || !strings.Contains(bundle.System, "final purpose") {
		t.Fatalf("System prompt = %q, want domain-purpose-first routing guidance", bundle.System)
	}
}

func TestBuildGatePromptDeclaresRouteContract(t *testing.T) {
	t.Parallel()

	bundle := BuildGatePrompt(GateInput{
		Run: assistant.Run{
			UserRequestRaw: "Can you just summarize what we already found in the previous run?",
		},
		ParentContext: &ParentRunContext{
			RunID:          "run_parent_123",
			UserRequestRaw: "Research five competitor pricing pages and summarize findings.",
			Summary:        "Collected pricing pages and created a draft comparison table.",
		},
	})

	if !strings.Contains(bundle.System, "\"answer\"") || !strings.Contains(bundle.System, "\"workflow\"") {
		t.Fatalf("System prompt = %q, want route enum guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "route") || !strings.Contains(bundle.System, "reason") || !strings.Contains(bundle.System, "summary") {
		t.Fatalf("System prompt = %q, want gate schema keys", bundle.System)
	}
	if !strings.Contains(bundle.User, "Parent run id: run_parent_123") {
		t.Fatalf("User prompt = %q, want parent run context", bundle.User)
	}
}

func TestBuildAnswerPromptDeclaresReadOrientedContract(t *testing.T) {
	t.Parallel()

	bundle := BuildAnswerPrompt(AnswerInput{
		Run: assistant.Run{
			UserRequestRaw: "What were the top 3 cheapest competitors from last run?",
		},
		ParentContext: &ParentRunContext{
			RunID:   "run_parent_321",
			Project: assistant.ProjectContext{Slug: "competitor-pricing"},
			Wiki: assistant.WikiContext{
				Enabled:         true,
				OverviewSummary: "The wiki already tracks pricing tiers and comparisons.",
			},
			Summary: "Saved pricing table and evidence from five competitor pages.",
			Artifacts: []assistant.Artifact{
				{ID: "artifact_1", Kind: assistant.ArtifactKindTable, Title: "Pricing table", MIMEType: "text/markdown"},
			},
			Evidence: []assistant.Evidence{
				{ID: "evidence_1", Kind: assistant.EvidenceKindObservation, Summary: "Vendor A: $49/mo starter"},
			},
		},
	})

	if !strings.Contains(strings.ToLower(bundle.System), "read-oriented") {
		t.Fatalf("System prompt = %q, want read-oriented guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "needs_user_input") || !strings.Contains(bundle.System, "wait_kind") {
		t.Fatalf("System prompt = %q, want answer wait schema keys", bundle.System)
	}
	if !strings.Contains(bundle.User, "Parent artifacts") || !strings.Contains(bundle.User, "Parent evidence highlights") {
		t.Fatalf("User prompt = %q, want parent context details", bundle.User)
	}
	if !strings.Contains(bundle.User, "Parent wiki context follows") || !strings.Contains(bundle.User, "The wiki already tracks pricing tiers") {
		t.Fatalf("User prompt = %q, want wiki context details", bundle.User)
	}
}

func TestBuildGeneratorPromptPrefersExplicitStatePersistence(t *testing.T) {
	t.Parallel()

	bundle := BuildGeneratorPrompt(GeneratorInput{
		Run: assistant.Run{
			UserRequestRaw: "Use https://example.com/source and save results to the target list.",
		},
	})

	if !strings.Contains(bundle.System, "project-specific browser profile and CDP port are available") || !strings.Contains(bundle.System, "--session-name") {
		t.Fatalf("System prompt = %q, want project-profile-first guidance and session-name warning", bundle.System)
	}
	if !strings.Contains(bundle.System, "agent-browser --cdp <port> open about:blank") {
		t.Fatalf("System prompt = %q, want explicit project CDP command guidance", bundle.System)
	}
	if strings.Contains(bundle.System, "agent-browser connect http://localhost:<port>") {
		t.Fatalf("System prompt = %q, should not encourage agent-browser connect for project CDP reuse", bundle.System)
	}
	if !strings.Contains(bundle.System, "Reuse the same project profile across runs so login state persists") {
		t.Fatalf("System prompt = %q, want profile persistence guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "agent-browser open <url> --headed") {
		t.Fatalf("System prompt = %q, want current agent-browser open guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "agent-browser snapshot -i --json") {
		t.Fatalf("System prompt = %q, want current agent-browser snapshot guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "Persist auth with explicit state files instead.") {
		t.Fatalf("System prompt = %q, want explicit state persistence guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "--auto-connect") {
		t.Fatalf("System prompt = %q, want auto-connect fallback guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "immediately save a fresh auth state to a project-local path") {
		t.Fatalf("System prompt = %q, want auto-connect save guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "do not keep relying on --auto-connect in the same task") {
		t.Fatalf("System prompt = %q, want auto-connect handoff guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "agent-browser state load <path>") {
		t.Fatalf("System prompt = %q, want explicit state load guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "When using --auto-connect") || !strings.Contains(bundle.System, "return a wait_request for approval") {
		t.Fatalf("System prompt = %q, want Chrome remote debugging approval guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "agent-browser") {
		t.Fatalf("System prompt = %q, want agent-browser guidance", bundle.System)
	}
	if !strings.Contains(bundle.System, "--headed") {
		t.Fatalf("System prompt = %q, want headed browser guidance", bundle.System)
	}
	if !strings.Contains(strings.ToLower(bundle.System), "webm") {
		t.Fatalf("System prompt = %q, want WebM recording guidance", bundle.System)
	}
	if !strings.Contains(bundle.User, "Original user request: Use https://example.com/source and save results to the target list.") {
		t.Fatalf("User prompt = %q, want original user request context", bundle.User)
	}
}

func TestBuildContractPromptDeclaresStrictJSONContract(t *testing.T) {
	t.Parallel()

	bundle := BuildContractPrompt(ContractInput{
		Run: assistant.Run{
			UserRequestRaw: "Take the cafes from https://www.diningcode.com/list.dc?query=foo and save them into Naver Map.",
			TaskSpec: assistant.TaskSpec{
				Goal:             "Compare competitor pricing",
				Deliverables:     []string{"Pricing table"},
				DoneDefinition:   []string{"Produce the pricing table"},
				EvidenceRequired: []string{"Source URLs"},
			},
		},
	})

	if !strings.Contains(bundle.System, "decision") || !strings.Contains(bundle.System, "acceptance_criteria") {
		t.Fatalf("System prompt = %q, want contract schema keys", bundle.System)
	}
	if !strings.Contains(bundle.System, "strict JSON object") {
		t.Fatalf("System prompt = %q, want strict JSON instruction", bundle.System)
	}
	if !strings.Contains(bundle.User, "Original user request: Take the cafes from https://www.diningcode.com/list.dc?query=foo and save them into Naver Map.") {
		t.Fatalf("User prompt = %q, want original user request context", bundle.User)
	}
}

func TestDecodeGateOutputBuildsRouteDecision(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"route": "answer",
		"reason": "The request can be answered from existing run evidence.",
		"summary": "Route to answer for a read-only follow-up."
	}`)

	route, reason, summary, err := DecodeGateOutput(raw)
	if err != nil {
		t.Fatalf("DecodeGateOutput() error = %v", err)
	}
	if route != assistant.RunRouteAnswer {
		t.Fatalf("route = %q, want %q", route, assistant.RunRouteAnswer)
	}
	if reason == "" || summary == "" {
		t.Fatalf("reason/summary = %q / %q, want non-empty values", reason, summary)
	}
}

func TestDecodeAnswerOutputBuildsAnswerResult(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"summary": "Prepared a direct answer from parent artifacts.",
		"output": "The cheapest three were Vendor A, Vendor C, and Vendor E.",
		"needs_user_input": false,
		"wait_kind": "",
		"wait_title": "",
		"wait_prompt": "",
		"wait_risk_summary": ""
	}`)

	output, err := DecodeAnswerOutput(raw)
	if err != nil {
		t.Fatalf("DecodeAnswerOutput() error = %v", err)
	}
	if output.Output == "" || output.NeedsUserInput {
		t.Fatalf("output = %#v, want non-empty output and no wait", output)
	}
}

func TestDecodePlannerOutputNormalizesTaskSpec(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"goal":"Compare competitor pricing",
		"deliverables":["Pricing table","Summary memo"],
		"constraints":["Use public sources only"],
		"tools_allowed":[],
		"tools_required":["agent-browser"],
		"done_definition":[],
		"evidence_required":[],
		"risk_flags":["public-web-research"],
		"max_generation_attempts":0
	}`)

	spec, err := DecodePlannerOutput(raw, PlannerInput{
		UserRequestRaw:        "Compare competitor pricing and summarize it.",
		MaxGenerationAttempts: 3,
	})
	if err != nil {
		t.Fatalf("DecodePlannerOutput() error = %v", err)
	}

	if spec.MaxGenerationAttempts != 3 {
		t.Fatalf("MaxGenerationAttempts = %d, want 3", spec.MaxGenerationAttempts)
	}
	if len(spec.ToolsAllowed) == 0 {
		t.Fatal("ToolsAllowed is empty, want normalized defaults")
	}
	if len(spec.DoneDefinition) == 0 || len(spec.EvidenceRequired) == 0 {
		t.Fatal("expected normalized done definition and evidence requirements")
	}
}

func TestBuildAndDecodeSchedulerPrompt(t *testing.T) {
	t.Parallel()

	bundle := BuildSchedulerPrompt(SchedulerInput{
		Run: assistant.Run{
			UserRequestRaw: "Research hospitals and call them later.",
			TaskSpec: assistant.TaskSpec{
				Goal: "Research hospitals",
				SchedulePlan: &assistant.SchedulePlan{
					Entries: []assistant.ScheduleEntry{
						{ScheduledFor: "13:00", Prompt: "Call the first hospital."},
					},
				},
			},
		},
		Artifacts: []assistant.Artifact{{Title: "Hospital shortlist", Kind: assistant.ArtifactKindReport, MIMEType: "text/markdown"}},
		Evidence:  []assistant.Evidence{{Summary: "Saint Mary Hospital listed oncology intake at +1-555-0100."}},
	})

	if !strings.Contains(bundle.System, "Finalize the deferred execution prompts") {
		t.Fatalf("System prompt = %q, want scheduler guidance", bundle.System)
	}
	if !strings.Contains(bundle.User, "Planned schedule entries") {
		t.Fatalf("User prompt = %q, want schedule entry context", bundle.User)
	}

	entries, err := DecodeSchedulerOutput([]byte(`{"entries":[{"scheduled_for":"2026-04-03T13:00:00Z","prompt":"Call Saint Mary Hospital at +1-555-0100."}]}`))
	if err != nil {
		t.Fatalf("DecodeSchedulerOutput() error = %v", err)
	}
	if len(entries) != 1 || !strings.Contains(entries[0].Prompt, "Saint Mary Hospital") {
		t.Fatalf("entries = %#v, want finalized scheduled entry", entries)
	}
}

func TestDecodeEvaluatorOutputBuildsEvaluation(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"passed": false,
		"score": 68,
		"summary": "The output is missing direct source links.",
		"missing_requirements": ["Direct source URLs for each competitor"],
		"incorrect_claims": [],
		"evidence_checked": ["Draft table"],
		"next_action_for_generator": "Collect direct source URLs and update the table."
	}`)

	evaluation, err := DecodeEvaluatorOutput(raw, "run_123", "attempt_456", time.Date(2026, time.March, 27, 10, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("DecodeEvaluatorOutput() error = %v", err)
	}

	if evaluation.RunID != "run_123" || evaluation.AttemptID != "attempt_456" {
		t.Fatalf("evaluation = %#v, want run and attempt ids", evaluation)
	}
	if evaluation.Score != 68 || evaluation.Passed {
		t.Fatalf("evaluation = %#v, want failed score 68", evaluation)
	}
}

func TestDecodeContractOutputBuildsAcceptanceContract(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"decision": "agreed",
		"summary": "The contract is concrete enough to start generation.",
		"deliverables": ["Pricing table", "Summary memo"],
		"acceptance_criteria": ["Each competitor has a source URL", "The memo summarizes major price differences"],
		"evidence_required": ["Direct pricing page URLs", "Stored artifact with final table"],
		"constraints": ["Use public sources only"],
		"out_of_scope": ["Private pricing data"],
		"revision_notes": ""
	}`)

	contract, decision, err := DecodeContractOutput(raw, assistant.TaskSpec{
		Goal:             "Compare competitor pricing",
		Deliverables:     []string{"Pricing table", "Summary memo"},
		DoneDefinition:   []string{"Produce deliverables"},
		EvidenceRequired: []string{"Source URLs"},
	})
	if err != nil {
		t.Fatalf("DecodeContractOutput() error = %v", err)
	}
	if decision != ContractDecisionAgreed {
		t.Fatalf("decision = %q, want %q", decision, ContractDecisionAgreed)
	}
	if contract.Status != assistant.ContractStatusAgreed {
		t.Fatalf("contract.Status = %q, want %q", contract.Status, assistant.ContractStatusAgreed)
	}
	if len(contract.AcceptanceCriteria) != 2 {
		t.Fatalf("contract = %#v, want populated acceptance criteria", contract)
	}
}

func TestDecodeProjectSelectorOutputBuildsProjectContext(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"project_slug": "infra-cost-estimation",
		"project_name": "Infrastructure Cost Estimation",
		"project_description": "Estimate and track the company's infrastructure spending.",
		"summary": "Selected the existing infrastructure cost estimation project."
	}`)

	project, summary, err := DecodeProjectSelectorOutput(raw)
	if err != nil {
		t.Fatalf("DecodeProjectSelectorOutput() error = %v", err)
	}
	if project.Slug != "infra-cost-estimation" || project.Name == "" || project.Description == "" {
		t.Fatalf("project = %#v, want populated project context", project)
	}
	if summary == "" {
		t.Fatal("summary is empty")
	}
}

func TestBuildEvaluatorPromptIncludesOriginalUserRequest(t *testing.T) {
	t.Parallel()

	bundle := BuildEvaluatorPrompt(EvaluatorInput{
		Run: assistant.Run{
			UserRequestRaw: "Verify the saved list against https://example.com/source.",
			TaskSpec: assistant.TaskSpec{
				Goal:             "Verify the saved list",
				DoneDefinition:   []string{"Compare the saved list to the source"},
				EvidenceRequired: []string{"Source URL", "Saved-list screenshot"},
			},
		},
	})

	if !strings.Contains(bundle.User, "Original user request: Verify the saved list against https://example.com/source.") {
		t.Fatalf("User prompt = %q, want original user request context", bundle.User)
	}
}

func TestBuildReportPromptIncludesAgentMessageContext(t *testing.T) {
	t.Parallel()

	bundle := BuildReportPrompt(ReportInput{
		Run: assistant.Run{
			ChatID:         "chat_abc123",
			UserRequestRaw: "Summarize the saved competitor pricing report.",
			TaskSpec: assistant.TaskSpec{
				Goal:             "Summarize competitor pricing",
				DoneDefinition:   []string{"Deliver the summary"},
				EvidenceRequired: []string{"Saved report"},
			},
		},
		ChatAccountUsername: "cva-chat_abc123",
		MasterUsername:      "supervisor",
	})

	if !strings.Contains(bundle.System, "agent-message catalog prompt") {
		t.Fatalf("System prompt = %q, want catalog prompt instruction", bundle.System)
	}
	if !strings.Contains(bundle.System, "report_payload") {
		t.Fatalf("System prompt = %q, want report payload schema", bundle.System)
	}
	if !strings.Contains(bundle.User, "Chat account username: cva-chat_abc123") {
		t.Fatalf("User prompt = %q, want chat account context", bundle.User)
	}
}

func TestDecodeReportOutputBuildsDeliveryResult(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"summary": "Delivered the final report.",
		"delivery_status": "sent",
		"message_preview": "Final pricing comparison delivered to the user.",
		"report_payload": "{\"root\":\"screen\",\"elements\":{\"screen\":{\"type\":\"Text\",\"props\":{\"text\":\"Done\"},\"children\":[]}}}",
		"needs_user_input": false,
		"wait_kind": "",
		"wait_title": "",
		"wait_prompt": "",
		"wait_risk_summary": ""
	}`)

	output, err := DecodeReportOutput(raw)
	if err != nil {
		t.Fatalf("DecodeReportOutput() error = %v", err)
	}
	if output.DeliveryStatus != "sent" || output.ReportPayload == "" {
		t.Fatalf("output = %#v, want sent payload", output)
	}
}
