export default function LoadingSkeleton({ count = 3 }: { count?: number }) {
  return (
    <div className="space-y-4">
      {Array.from({ length: count }, (_, i) => (
        <div
          key={i}
          className="bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg p-4 animate-pulse"
        >
          <div className="h-4 bg-zinc-200 dark:bg-zinc-800 rounded w-3/4 mb-3" />
          <div className="h-3 bg-zinc-200 dark:bg-zinc-800 rounded w-1/2 mb-2" />
          <div className="flex gap-2">
            <div className="h-5 bg-zinc-200 dark:bg-zinc-800 rounded w-16" />
            <div className="h-5 bg-zinc-200 dark:bg-zinc-800 rounded w-20" />
          </div>
        </div>
      ))}
    </div>
  );
}
