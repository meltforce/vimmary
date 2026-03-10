import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  fetchWebhookInfo,
  fetchKarakeepStatus,
  setKarakeepAPIKey,
  fetchSummaryPrompts,
  setSummaryPrompt,
  fetchProviders,
  fetchModels,
  setModel,
} from "../api.ts";
import type { ModelInfo } from "../api.ts";
import LoadingSkeleton from "../components/LoadingSkeleton.tsx";

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  return (
    <button
      onClick={() => {
        navigator.clipboard.writeText(text);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      }}
      className="px-2 py-1 text-xs bg-zinc-100 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400 rounded hover:bg-zinc-200 dark:hover:bg-zinc-700 transition-colors shrink-0"
    >
      {copied ? "Copied" : "Copy"}
    </button>
  );
}

function PromptEditor({
  level,
  label,
  currentPrompt,
  defaultPrompt,
}: {
  level: string;
  label: string;
  currentPrompt: string;
  defaultPrompt: string;
}) {
  const queryClient = useQueryClient();
  const [value, setValue] = useState(currentPrompt);
  const isCustom = currentPrompt !== defaultPrompt;

  const save = useMutation({
    mutationFn: (prompt: string) => setSummaryPrompt(level, prompt),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["settings", "prompts"] });
    },
  });

  const reset = useMutation({
    mutationFn: () => setSummaryPrompt(level, ""),
    onSuccess: () => {
      setValue(defaultPrompt);
      queryClient.invalidateQueries({ queryKey: ["settings", "prompts"] });
    },
  });

  const hasChanges = value !== currentPrompt;

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <h3 className="text-sm font-medium text-zinc-600 dark:text-zinc-400">
          {label}
          {isCustom && (
            <span className="ml-2 text-xs text-cyan-600 dark:text-cyan-400">
              (custom)
            </span>
          )}
        </h3>
        <div className="flex gap-2">
          {isCustom && (
            <button
              onClick={() => reset.mutate()}
              disabled={reset.isPending}
              className="px-3 py-1 text-xs text-zinc-600 dark:text-zinc-400 border border-zinc-300 dark:border-zinc-700 rounded hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors disabled:opacity-50"
            >
              {reset.isPending ? "Resetting..." : "Reset to Default"}
            </button>
          )}
          <button
            onClick={() => save.mutate(value)}
            disabled={!hasChanges || save.isPending}
            className="px-3 py-1 text-xs bg-cyan-600 text-white rounded hover:bg-cyan-700 transition-colors disabled:opacity-50"
          >
            {save.isPending ? "Saving..." : "Save"}
          </button>
        </div>
      </div>
      <textarea
        value={value}
        onChange={(e) => setValue(e.target.value)}
        rows={12}
        className="w-full px-3 py-2 text-sm font-mono rounded-md border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 resize-y"
      />
      {save.isSuccess && (
        <p className="text-xs text-green-600 dark:text-green-400">
          Prompt saved.
        </p>
      )}
      {save.isError && (
        <p className="text-xs text-red-600 dark:text-red-400">
          {(save.error as Error).message}
        </p>
      )}
    </div>
  );
}

function ModelSelector({ provider }: { provider: string }) {
  const queryClient = useQueryClient();

  const { data, isLoading } = useQuery({
    queryKey: ["models", provider],
    queryFn: () => fetchModels(provider),
  });

  const [selected, setSelected] = useState<string | null>(null);

  // Sync selected state when data loads
  const currentSelected = data?.selected ?? "";
  const displaySelected = selected ?? currentSelected;

  const save = useMutation({
    mutationFn: (model: string) => setModel(provider, model),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["models", provider] });
      queryClient.invalidateQueries({ queryKey: ["providers"] });
    },
  });

  const hasChanges = displaySelected !== currentSelected;

  if (isLoading) return <div className="text-sm text-zinc-500">Loading models...</div>;
  if (!data?.models?.length) return <div className="text-sm text-zinc-500">No models available</div>;

  return (
    <div className="space-y-2">
      <label className="text-sm font-medium text-zinc-600 dark:text-zinc-400 capitalize">
        {provider}
      </label>
      <div className="flex gap-2">
        <select
          value={displaySelected}
          onChange={(e) => setSelected(e.target.value)}
          className="flex-1 px-3 py-2 text-sm rounded-md border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100"
        >
          <option value="">Provider default</option>
          {(data.models as ModelInfo[]).map((m) => (
            <option key={m.id} value={m.id}>
              {m.display_name || m.id}
            </option>
          ))}
        </select>
        <button
          onClick={() => save.mutate(displaySelected)}
          disabled={!hasChanges || save.isPending}
          className="px-4 py-2 text-sm bg-cyan-600 text-white rounded-md hover:bg-cyan-700 transition-colors disabled:opacity-50"
        >
          {save.isPending ? "Saving..." : "Save"}
        </button>
      </div>
      {save.isSuccess && (
        <p className="text-xs text-green-600 dark:text-green-400">Model saved.</p>
      )}
      {save.isError && (
        <p className="text-xs text-red-600 dark:text-red-400">
          {(save.error as Error).message}
        </p>
      )}
    </div>
  );
}

export default function SettingsPage() {
  const queryClient = useQueryClient();
  const [apiKey, setApiKey] = useState("");

  const {
    data: webhook,
    isLoading: webhookLoading,
    error: webhookError,
  } = useQuery({
    queryKey: ["settings", "webhook"],
    queryFn: fetchWebhookInfo,
  });

  const {
    data: karakeepStatus,
    isLoading: karakeepLoading,
    error: karakeepError,
  } = useQuery({
    queryKey: ["settings", "karakeep"],
    queryFn: fetchKarakeepStatus,
  });

  const {
    data: prompts,
    isLoading: promptsLoading,
    error: promptsError,
  } = useQuery({
    queryKey: ["settings", "prompts"],
    queryFn: fetchSummaryPrompts,
  });

  const { data: providers } = useQuery({
    queryKey: ["providers"],
    queryFn: fetchProviders,
  });

  const saveKey = useMutation({
    mutationFn: (key: string) => setKarakeepAPIKey(key),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["settings", "karakeep"] });
      setApiKey("");
    },
  });

  const isLoading = webhookLoading || karakeepLoading || promptsLoading;
  const error = webhookError || karakeepError || promptsError;

  if (isLoading) return <LoadingSkeleton count={3} />;
  if (error) {
    return (
      <div className="text-red-600 dark:text-red-400 text-sm bg-red-50 dark:bg-red-950/30 border border-red-200 dark:border-red-900/50 rounded-lg p-3">
        {(error as Error).message}
      </div>
    );
  }

  const webhookURL = `${window.location.origin}/webhook/karakeep`;

  return (
    <div className="space-y-8">
      {/* Karakeep API Key */}
      <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg p-6 space-y-4">
        <h2 className="text-zinc-700 dark:text-zinc-300 font-medium">
          Karakeep API Key
        </h2>
        <p className="text-zinc-500 text-sm">
          Your Karakeep API key is used to write summaries back to your
          bookmarks. Get it from Karakeep Settings &rarr; API Keys.
        </p>
        <div className="flex items-center gap-2">
          <span className="text-sm text-zinc-500">Status:</span>
          {karakeepStatus?.configured ? (
            <span className="text-sm text-green-600 dark:text-green-400">
              Configured
            </span>
          ) : (
            <span className="text-sm text-amber-600 dark:text-amber-400">
              Not set
            </span>
          )}
        </div>
        <div className="flex gap-2">
          <input
            type="password"
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            placeholder={
              karakeepStatus?.configured
                ? "Enter new key to replace"
                : "Paste your Karakeep API key"
            }
            className="flex-1 px-3 py-2 text-sm rounded-md border border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 placeholder-zinc-400"
          />
          <button
            onClick={() => saveKey.mutate(apiKey)}
            disabled={!apiKey || saveKey.isPending}
            className="px-4 py-2 text-sm bg-cyan-600 text-white rounded-md hover:bg-cyan-700 transition-colors disabled:opacity-50"
          >
            {saveKey.isPending ? "Saving..." : "Save"}
          </button>
        </div>
        {saveKey.isSuccess && (
          <p className="text-sm text-green-600 dark:text-green-400">
            API key saved.
          </p>
        )}
        {saveKey.isError && (
          <p className="text-sm text-red-600 dark:text-red-400">
            {(saveKey.error as Error).message}
          </p>
        )}
      </div>

      {/* Model Selection */}
      {providers && providers.providers.length > 0 && (
        <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg p-6 space-y-4">
          <div>
            <h2 className="text-zinc-700 dark:text-zinc-300 font-medium">
              Summary Models
            </h2>
            <p className="text-zinc-500 text-sm mt-1">
              Choose which model each provider uses for generating summaries.
              Leave empty for the provider's default model.
            </p>
          </div>
          {providers.providers.map((p) => (
            <ModelSelector key={p} provider={p} />
          ))}
        </div>
      )}

      {/* Summary Prompts */}
      <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg p-6 space-y-5">
        <div>
          <h2 className="text-zinc-700 dark:text-zinc-300 font-medium">
            Summary Prompts
          </h2>
          <p className="text-zinc-500 text-sm mt-1">
            Customize the prompts used when generating summaries. Available
            placeholders:{" "}
            <code className="text-xs bg-zinc-100 dark:bg-zinc-800 px-1 py-0.5 rounded">
              {"{{TITLE}}"}
            </code>
            ,{" "}
            <code className="text-xs bg-zinc-100 dark:bg-zinc-800 px-1 py-0.5 rounded">
              {"{{LANGUAGE}}"}
            </code>
            ,{" "}
            <code className="text-xs bg-zinc-100 dark:bg-zinc-800 px-1 py-0.5 rounded">
              {"{{TRANSCRIPT}}"}
            </code>
          </p>
        </div>
        {prompts && (
          <>
            <PromptEditor
              level="medium"
              label="Medium Summary"
              currentPrompt={prompts.medium}
              defaultPrompt={prompts.default_medium}
            />
            <PromptEditor
              level="deep"
              label="Deep Summary"
              currentPrompt={prompts.deep}
              defaultPrompt={prompts.default_deep}
            />
          </>
        )}
      </div>

      {/* Webhook Configuration */}
      <div className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg p-6 space-y-4">
        <h2 className="text-zinc-700 dark:text-zinc-300 font-medium">
          Karakeep Webhook
        </h2>
        <p className="text-zinc-500 text-sm">
          Add this webhook in Karakeep Settings &rarr; Webhooks &rarr; Create.
          Set the event to <code className="text-xs bg-zinc-100 dark:bg-zinc-800 px-1 py-0.5 rounded">created</code>.
        </p>

        <div className="space-y-3">
          <div>
            <label className="text-xs text-zinc-500 block mb-1">
              Webhook URL
            </label>
            <div className="flex items-center gap-2">
              <code className="flex-1 text-sm bg-zinc-50 dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 rounded px-3 py-2 text-zinc-700 dark:text-zinc-300 break-all">
                {webhookURL}
              </code>
              <CopyButton text={webhookURL} />
            </div>
          </div>

          <div>
            <label className="text-xs text-zinc-500 block mb-1">
              Bearer Token
            </label>
            <div className="flex items-center gap-2">
              <code className="flex-1 text-sm bg-zinc-50 dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 rounded px-3 py-2 text-zinc-700 dark:text-zinc-300 break-all font-mono">
                {webhook?.token}
              </code>
              <CopyButton text={webhook?.token ?? ""} />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
