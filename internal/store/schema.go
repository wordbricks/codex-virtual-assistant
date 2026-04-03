package store

const schemaSQL = `
PRAGMA journal_mode = WAL;

CREATE TABLE IF NOT EXISTS runs (
	id TEXT PRIMARY KEY,
	chat_id TEXT NOT NULL,
	parent_run_id TEXT,
	status TEXT NOT NULL,
	phase TEXT NOT NULL,
	gate_route TEXT NOT NULL DEFAULT '',
	gate_reason TEXT NOT NULL DEFAULT '',
	gate_decided_at TEXT,
	project_json TEXT NOT NULL DEFAULT '{}',
	user_request_raw TEXT NOT NULL,
	task_spec_json TEXT NOT NULL,
	attempt_count INTEGER NOT NULL DEFAULT 0,
	max_generation_attempts INTEGER NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	completed_at TEXT
);

CREATE TABLE IF NOT EXISTS run_events (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	type TEXT NOT NULL,
	phase TEXT,
	summary TEXT NOT NULL,
	created_at TEXT NOT NULL,
	FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS attempts (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	sequence INTEGER NOT NULL,
	role TEXT NOT NULL,
	input_summary TEXT NOT NULL,
	output_summary TEXT NOT NULL,
	critique TEXT,
	started_at TEXT NOT NULL,
	finished_at TEXT,
	FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS artifacts (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	attempt_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	title TEXT NOT NULL,
	mime_type TEXT NOT NULL,
	path TEXT,
	content TEXT,
	source_url TEXT,
	created_at TEXT NOT NULL,
	FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE,
	FOREIGN KEY (attempt_id) REFERENCES attempts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS evidences (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	attempt_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	summary TEXT NOT NULL,
	detail TEXT,
	created_at TEXT NOT NULL,
	FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE,
	FOREIGN KEY (attempt_id) REFERENCES attempts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS evaluations (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	attempt_id TEXT NOT NULL,
	passed INTEGER NOT NULL,
	score INTEGER NOT NULL,
	summary TEXT NOT NULL,
	missing_requirements_json TEXT NOT NULL,
	incorrect_claims_json TEXT NOT NULL,
	evidence_checked_json TEXT NOT NULL,
	next_action_for_generator TEXT NOT NULL,
	created_at TEXT NOT NULL,
	FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE,
	FOREIGN KEY (attempt_id) REFERENCES attempts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS tool_calls (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	attempt_id TEXT NOT NULL,
	tool_name TEXT NOT NULL,
	input_summary TEXT NOT NULL,
	output_summary TEXT NOT NULL,
	started_at TEXT NOT NULL,
	finished_at TEXT NOT NULL,
	FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE,
	FOREIGN KEY (attempt_id) REFERENCES attempts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS web_steps (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	attempt_id TEXT NOT NULL,
	title TEXT NOT NULL,
	url TEXT,
	summary TEXT NOT NULL,
	occurred_at TEXT NOT NULL,
	FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE,
	FOREIGN KEY (attempt_id) REFERENCES attempts(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS wait_requests (
	id TEXT PRIMARY KEY,
	run_id TEXT NOT NULL,
	kind TEXT NOT NULL,
	title TEXT NOT NULL,
	prompt TEXT NOT NULL,
	risk_summary TEXT,
	created_at TEXT NOT NULL,
	FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS scheduled_runs (
	id TEXT PRIMARY KEY,
	chat_id TEXT NOT NULL,
	parent_run_id TEXT NOT NULL,
	user_request_raw TEXT NOT NULL,
	max_generation_attempts INTEGER NOT NULL,
	scheduled_for TEXT NOT NULL,
	status TEXT NOT NULL,
	run_id TEXT,
	error_message TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	triggered_at TEXT,
	FOREIGN KEY (parent_run_id) REFERENCES runs(id) ON DELETE CASCADE,
	FOREIGN KEY (run_id) REFERENCES runs(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_run_events_run_id_created_at ON run_events(run_id, created_at);
CREATE INDEX IF NOT EXISTS idx_attempts_run_id_sequence ON attempts(run_id, sequence);
CREATE INDEX IF NOT EXISTS idx_artifacts_run_id_created_at ON artifacts(run_id, created_at);
CREATE INDEX IF NOT EXISTS idx_evidences_run_id_created_at ON evidences(run_id, created_at);
CREATE INDEX IF NOT EXISTS idx_evaluations_run_id_created_at ON evaluations(run_id, created_at);
CREATE INDEX IF NOT EXISTS idx_tool_calls_run_id_started_at ON tool_calls(run_id, started_at);
CREATE INDEX IF NOT EXISTS idx_web_steps_run_id_occurred_at ON web_steps(run_id, occurred_at);
CREATE INDEX IF NOT EXISTS idx_wait_requests_run_id_created_at ON wait_requests(run_id, created_at);
CREATE INDEX IF NOT EXISTS idx_scheduled_runs_status_scheduled_for ON scheduled_runs(status, scheduled_for);
CREATE INDEX IF NOT EXISTS idx_scheduled_runs_parent_run_id ON scheduled_runs(parent_run_id, scheduled_for);
CREATE INDEX IF NOT EXISTS idx_scheduled_runs_chat_id_created_at ON scheduled_runs(chat_id, created_at);
`
