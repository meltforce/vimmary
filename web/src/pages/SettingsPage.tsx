import { useState } from "react";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  fetchWebhookInfo,
  fetchKarakeepStatus,
  setKarakeepAPIKey,
} from "../api.ts";
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

  const saveKey = useMutation({
    mutationFn: (key: string) => setKarakeepAPIKey(key),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["settings", "karakeep"] });
      setApiKey("");
    },
  });

  const isLoading = webhookLoading || karakeepLoading;
  const error = webhookError || karakeepError;

  if (isLoading) return <LoadingSkeleton count={2} />;
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
