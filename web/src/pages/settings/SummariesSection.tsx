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
import { Row, Section, SectionError, SectionLoading } from "./primitives.tsx";

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

  if (isLoading) {
    return <span style={{ fontSize: 13, color: "var(--color-neutral-600)" }}>Loading models…</span>;
  }
  if (!data?.models?.length) {
    return <span style={{ fontSize: 13, color: "var(--color-neutral-600)" }}>No models available.</span>;
  }

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
    <>
      <div style={{ display: "flex", gap: 8, alignItems: "center", flexWrap: "wrap" }}>
        <select
          value={displaySelected}
          onChange={(e) => setSelected(e.target.value)}
          className="select"
          style={{ flex: 1, minWidth: 220 }}
          aria-label="Summary model"
        >
          <option value="">Provider default</option>
          {[...byProvider.entries()].map(([provider, models]) => (
            <optgroup key={provider} label={provider.charAt(0).toUpperCase() + provider.slice(1)}>
              {models.map((m) => (
                <option key={`${provider}:${m.id}`} value={`${provider}:${m.id}`}>
                  {m.display_name || m.id}
                </option>
              ))}
            </optgroup>
          ))}
        </select>
        <button
          type="button"
          className="btn btn-primary"
          disabled={!hasChanges || save.isPending}
          onClick={() => save.mutate(displaySelected)}
        >
          {save.isPending ? "Saving…" : "Save"}
        </button>
      </div>
      {save.isError ? <p className="field-error">{(save.error as Error).message}</p> : null}
    </>
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
    <Row label={label}>
      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        <span style={{ fontSize: 14, flex: 1 }}>
          {isCustom ? "Edited" : "Default"}
        </span>
        <button type="button" className="btn btn-secondary" onClick={() => setOpen(!open)}>
          {open ? "Close" : "Edit"}
        </button>
      </div>

      {open ? (
        <div style={{ marginTop: 10 }}>
          <textarea
            className="textarea"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            rows={14}
            style={{ fontFamily: "var(--font-mono)", fontSize: 12.5 }}
          />
          <div style={{ display: "flex", gap: 8, marginTop: 8 }}>
            <button
              type="button"
              className="btn btn-primary"
              disabled={!hasChanges || save.isPending}
              onClick={() => save.mutate(value)}
            >
              {save.isPending ? "Saving…" : "Save"}
            </button>
            {isCustom ? (
              <button
                type="button"
                className="btn btn-danger"
                disabled={reset.isPending}
                onClick={() => reset.mutate()}
              >
                {reset.isPending ? "Resetting…" : "Reset to default"}
              </button>
            ) : null}
          </div>
          {save.isError ? <p className="field-error">{(save.error as Error).message}</p> : null}
        </div>
      ) : null}
    </Row>
  );
}

/**
 * The model choice and the two prompts. The prompts are per content source — a
 * podcast transcript and a talk transcript want different instructions — which
 * is what promptSource selects; the switch only appears when podcasts are
 * configured.
 */
export default function SummariesSection() {
  const [promptSource, setPromptSource] = useState<ContentSource>("youtube");
  const podcastsEnabled = usePodcastsEnabled();

  const { data: prompts, isLoading, error } = useQuery({
    queryKey: ["settings", "prompts", promptSource],
    queryFn: () => fetchSummaryPrompts(promptSource),
  });
  const { data: providers } = useQuery({ queryKey: ["providers"], queryFn: fetchProviders });

  const subtitle =
    "Which model writes the summary, and what it is asked for. A changed prompt applies to the next summary, not to the ones already stored.";

  return (
    <Section title="Summaries" subtitle={subtitle}>
      {providers && providers.providers.length > 0 ? (
        <Row label="Model">
          <ModelSelector />
        </Row>
      ) : null}

      {podcastsEnabled ? (
        <Row label="Prompts for">
          <span className="seg">
            {(["youtube", "podcast"] as ContentSource[]).map((src) => (
              <label key={src} className="seg-opt">
                <input
                  type="radio"
                  name="prompt-source"
                  checked={promptSource === src}
                  onChange={() => setPromptSource(src)}
                />
                {src === "youtube" ? "Videos" : "Podcasts"}
              </label>
            ))}
          </span>
        </Row>
      ) : null}

      {isLoading ? <SectionLoading /> : null}
      {error ? <SectionError error={error as Error} /> : null}

      {prompts ? (
        <>
          <PromptEditor
            key={`${promptSource}-medium`}
            source={promptSource}
            level="medium"
            label="Medium prompt"
            currentPrompt={prompts.medium}
            defaultPrompt={prompts.default_medium}
          />
          <PromptEditor
            key={`${promptSource}-deep`}
            source={promptSource}
            level="deep"
            label="Deep prompt"
            currentPrompt={prompts.deep}
            defaultPrompt={prompts.default_deep}
          />
          <Row label="Placeholders">
            <span className="mono">
              {"{{TITLE}}"} · {"{{LANGUAGE}}"} · {"{{TRANSCRIPT}}"}
            </span>
          </Row>
        </>
      ) : null}
    </Section>
  );
}
