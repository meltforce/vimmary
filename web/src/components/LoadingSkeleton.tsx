export default function LoadingSkeleton({ count = 3 }: { count?: number }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
      {Array.from({ length: count }, (_, i) => (
        <div
          key={i}
          className="animate-pulse"
          style={{
            padding: 20,
            background: "var(--vim-surface)",
            border: "1px solid var(--vim-line-soft)",
            borderRadius: 12,
          }}
        >
          <div
            style={{
              height: 14,
              width: "60%",
              background: "var(--vim-surface-2)",
              borderRadius: 4,
              marginBottom: 10,
            }}
          />
          <div
            style={{
              height: 10,
              width: "40%",
              background: "var(--vim-surface-2)",
              borderRadius: 4,
              marginBottom: 12,
            }}
          />
          <div style={{ display: "flex", gap: 8 }}>
            <div
              style={{
                height: 18,
                width: 70,
                background: "var(--vim-surface-2)",
                borderRadius: 999,
              }}
            />
            <div
              style={{
                height: 18,
                width: 90,
                background: "var(--vim-surface-2)",
                borderRadius: 999,
              }}
            />
          </div>
        </div>
      ))}
    </div>
  );
}
