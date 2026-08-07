-- Service-wide settings, as opposed to the per-user settings that live in
-- columns on `users` and in `user_prompts`. It exists so the LLM API keys and
-- the summary provider can be maintained in the Settings page instead of
-- arriving from setec at startup: resolving them over the network was what left
-- vimmary dead for 6h23min on 2026-08-07 (see INCIDENTS.md).
--
-- Key-value rather than typed columns, because the alternative is a
-- single-row table that needs a migration for every new setting. The keys in
-- use are `mistral_api_key`, `anthropic_api_key` and `summary_provider`;
-- `internal/storage/settings.go` is where they are named.
--
-- No backfill. The keys are entered once by hand after the deploy, which is
-- what allows the setec client to be removed from vimmary entirely rather than
-- kept alive for one release to copy them across.
--
-- Values are stored in plain text, like `users.karakeep_api_key` already is.
-- That is a decision, not an oversight — DECISIONS.md carries what it costs and
-- what would overturn it.
CREATE TABLE app_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
