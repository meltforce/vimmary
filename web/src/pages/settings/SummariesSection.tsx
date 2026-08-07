import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  fetchSummaryPrompts,
  setSummaryPrompt,
  fetchProviders,
  fetchModels,
  setModel,
} from "../../api.ts";
import type { ContentSource, ModelInfo, ModelsResponse } from "../../api.ts";
import { usePodcastsEnabled } from "../../features.ts";
import { Section, SectionError, SectionLoading } from "./primitives.tsx";

function ModelSelector() {
  const queryClient = useQueryClient();

  const { data, isLoading } = useQuery<ModelsResponse>({
    queryKey: ["models"],
    queryFn: () => fetchModels(),
  });

  const [selected, setSelected] = useState<string | null>(null);

  const currentKey =
    data?.selected_provider && data?.selected_model
      ? `${data.selected_provider}:${data.selected_model}`
      : "";
  const displaySelected = selected ?? currentKey;

  const save = useMutation({
    mutationFn: (key: string) => {
      if (!key) return setModel("", "");
      const [provider, ...rest] = key.split(":");
      return setModel(provider, rest.join(":"));
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["models"] });
      queryClient.invalidateQueries({ queryKey: ["providers"] });
    },
  });

  const hasChanges = displaySelected !== currentKey;

  if (isLoading)
    return <div style={{ fontSize: 13, color: "var(--vim-ink-3)" }}>Loading models…</div>;
  if (!data?.models?.length)
    return <div style={{ fontSize: 13, color: "var(--vim-ink-3)" }}>No models available</div>;

  const byProvider = new Map<string, ModelInfo[]>();
  const seen = new Set<string>();
  for (const m of data.models as ModelInfo[]) {
    const k = `${m.provider}:${m.id}`;
    if (seen.has(k)) continue;
    seen.add(k);
    const list = byProvider.get(m.provider) || [];
    list.push(m);
    byProvider.set(m.provider, list);
  }

  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
      <div style={{ display: "flex", gap: 8 }}>
        <select
          value={displaySelected}
          onChange={(e) => setSelected(e.target.value)}
          className="vim-input"
          style={{ flex: 1, fontSize: 13 }}
        >
          <option value="">Provider default</option>
          {[...byProvider.entries()].map(([provider, models]) => (
            <optgroup
              key={provider}
              label={provider.charAt(0).toUpperCase() + provider.slice(1)}
            >
              {models.map((m) => (
                <option key={`${provider}:${m.id}`} value={`${provider}:${m.id}`}>
                  {m.display_name || m.id}
                </option>
              ))}
            </optgroup>
          ))}
        </select>
        <button
          onClick={() => save.mutate(displaySelected)}
          disabled={!hasChanges || save.isPending}
          className="vim-btn primary"
          style={{ padding: "8px 14px", fontSize: 12 }}
        >
          {save.isPending ? "Saving…" : "Save"}
        </button>
      </div>
      {save.isError && (
        <p style={{ fontSize: 12, color: "var(--vim-err)", margin: 0 }}>
          {(save.error as Error).message}
        </p>
      )}
    </div>
  );
}

function PromptEditor({
  source,
  level,
  label,
  currentPrompt,
  defaultPrompt,
}: {
  source: ContentSource;
  level: string;
  label: string;
  currentPrompt: string;
  defaultPrompt: string;
}) {
  const queryClient = useQueryClient();
  const [value, setValue] = useState(currentPrompt);
  const [open, setOpen] = useState(false);
  const isCustom = currentPrompt !== defaultPrompt;

  const save = useMutation({
    mutationFn: (prompt: string) => setSummaryPrompt(source, level, prompt),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ["settings", "prompts"] }),
  });

  const reset = useMutation({
    mutationFn: () => setSummaryPrompt(source, level, ""),
    onSuccess: () => {
      setValue(defaultPrompt);
      queryClient.invalidateQueries({ queryKey: ["settings", "prompts"] });
    },
  });

  const hasChanges = value !== currentPrompt;

  return (
    <div style={{ padding: "16px 0", borderBottom: "1px solid var(--vim-line-soft)" }}>
      <div
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 16,
          marginBottom: open ? 12 : 0,
        }}
      >
        <div>
          <div style={{ fontSize: 13, color: "var(--vim-ink-3)", marginBottom: 3 }}>
            {label}
          </div>
          <div style={{ fontSize: 14, color: "var(--vim-ink)" }}>
            {isCustom ? (
              <>
                Custom prompt{" "}
                <span
                  style={{
                    fontFamily: "var(--font-mono)",
                    fontSize: 11,
                    color: "var(--vim-accent-ink)",
                    marginLeft: 4,
                  }}
                >
                  edited
                </span>
              </>
            ) : (
              "Default prompt"
            )}
          </div>
        </div>
        <button
          onClick={() => setOpen(!open)}
          className="vim-btn ghost"
          style={{ padding: "6px 12px", fontSize: 12 }}
        >
          {open ? "Hide ↑" : "Edit ↓"}
        </button>
      </div>
      {open && (
        <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
          <textarea
            value={value}
            onChange={(e) => setValue(e.target.value)}
            rows={12}
            className="vim-input"
            style={{ fontFamily: "var(--font-mono)", fontSize: 12.5, resize: "vertical" }}
          />
          <div style={{ display: "flex", gap: 8, justifyContent: "flex-end" }}>
            {isCustom && (
              <button
                onClick={() => reset.mutate()}
                disabled={reset.isPending}
                className="vim-btn outline danger"
                style={{ padding: "6px 12px", fontSize: 12 }}
              >
                {reset.isPending ? "Resetting…" : "Reset to default"}
              </button>
            )}
            <button
              onClick={() => save.mutate(value)}
              disabled={!hasChanges || save.isPending}
              className="vim-btn primary"
              style={{ padding: "6px 12px", fontSize: 12 }}
            >
              {save.isPending ? "Saving…" : "Save"}
            </button>
          </div>
          {save.isSuccess && (
            <p style={{ fontSize: 12, color: "var(--vim-ok)", margin: 0 }}>Prompt saved.</p>
          )}
          {save.isError && (
            <p style={{ fontSize: 12, color: "var(--vim-err)", margin: 0 }}>
              {(save.error as Error).message}
            </p>
          )}
        </div>
      )}
    </div>
  );
}

/**
 * SummariesSection holds the model choice and the two prompts. The prompts are
 * per content source — a podcast transcript and a talk transcript want different
 * instructions — which is what promptSource selects; the switch only appears
 * when podcasts are configured.
 */
export default function SummariesSection() {
  const [promptSource, setPromptSource] = useState<ContentSource>("youtube");
  const podcastsEnabled = usePodcastsEnabled();

  const { data: prompts, isLoading, error } = useQuery({
    queryKey: ["settings", "prompts", promptSource],
    queryFn: () => fetchSummaryPrompts(promptSource),
  });
  const { data: providers } = useQuery({
    queryKey: ["providers"],
    queryFn: fetchProviders,
  });

  if (isLoading)
    return (
      <Section title="Summaries" subtitle="Model and prompt configuration.">
        <SectionLoading what="prompts" />
      </Section>
    );
  if (error)
    return (
      <Section title="Summaries" subtitle="Model and prompt configuration.">
        <SectionError error={error as Error} />
      </Section>
    );

  return (
  <Section title="Summaries" subtitle="Model and prompt configuration.">
    {providers && providers.providers.length > 0 && (
      <div style={{ padding: "16px 0", borderBottom: "1px solid var(--vim-line-soft)" }}>
        <div style={{ fontSize: 13, color: "var(--vim-ink-3)", marginBottom: 8 }}>
          Model
        </div>
        <ModelSelector />
      </div>
    )}
    {podcastsEnabled && (
      <div style={{ padding: "16px 0", borderBottom: "1px solid var(--vim-line-soft)" }}>
        <div style={{ fontSize: 13, color: "var(--vim-ink-3)", marginBottom: 8 }}>
          Prompts for
        </div>
        <div style={{ display: "flex", gap: 6 }}>
          {(["youtube", "podcast"] as ContentSource[]).map((src) => (
            <button
              key={src}
              onClick={() => setPromptSource(src)}
              className={promptSource === src ? "vim-btn primary" : "vim-btn ghost"}
              style={{ padding: "6px 14px", fontSize: 12 }}
            >
              {src === "youtube" ? "Videos" : "Podcasts"}
            </button>
          ))}
        </div>
      </div>
    )}
    {prompts && (
      <>
        <PromptEditor
          key={`${promptSource}-medium`}
          source={promptSource}
          level="medium"
          label="Medium summary prompt"
          currentPrompt={prompts.medium}
          defaultPrompt={prompts.default_medium}
        />
        <PromptEditor
          key={`${promptSource}-deep`}
          source={promptSource}
          level="deep"
          label="Deep summary prompt"
          currentPrompt={prompts.deep}
          defaultPrompt={prompts.default_deep}
        />
        <div style={{ padding: "12px 0 16px", fontSize: 11.5, color: "var(--vim-ink-4)" }}>
          Placeholders:{" "}
          <code
            style={{
              fontFamily: "var(--font-mono)",
              background: "var(--vim-surface-2)",
              padding: "1px 5px",
              borderRadius: 3,
            }}
          >
            {"{{TITLE}}"}
          </code>
          ,{" "}
          <code
            style={{
              fontFamily: "var(--font-mono)",
              background: "var(--vim-surface-2)",
              padding: "1px 5px",
              borderRadius: 3,
            }}
          >
            {"{{LANGUAGE}}"}
          </code>
          ,{" "}
          <code
            style={{
              fontFamily: "var(--font-mono)",
              background: "var(--vim-surface-2)",
              padding: "1px 5px",
              borderRadius: 3,
            }}
          >
            {"{{TRANSCRIPT}}"}
          </code>
        </div>
      </>
    )}
  </Section>
  );
}
