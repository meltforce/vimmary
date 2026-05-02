import { Routes, Route, Navigate } from "react-router-dom";
import { Suspense, lazy } from "react";
import ErrorBoundary from "./components/ErrorBoundary.tsx";
import Layout from "./components/Layout.tsx";

const VideoListPage = lazy(() => import("./pages/VideoListPage.tsx"));
const VideoDetailPage = lazy(() => import("./pages/VideoDetailPage.tsx"));
const StatsPage = lazy(() => import("./pages/StatsPage.tsx"));
const SettingsPage = lazy(() => import("./pages/SettingsPage.tsx"));

function Loading() {
  return (
    <div className="flex items-center justify-center" style={{ padding: "80px 0" }}>
      <div style={{ color: "var(--vim-ink-3)", fontFamily: "var(--font-mono)", fontSize: 12, letterSpacing: "0.04em" }}>
        Loading…
      </div>
    </div>
  );
}

export default function App() {
  return (
    <Layout>
      <ErrorBoundary>
        <Suspense fallback={<Loading />}>
          <Routes>
            <Route path="/" element={<VideoListPage />} />
            <Route path="/video/:id" element={<VideoDetailPage />} />
            <Route path="/stats" element={<StatsPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </Suspense>
      </ErrorBoundary>
    </Layout>
  );
}
