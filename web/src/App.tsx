import { Routes, Route, Navigate } from "react-router-dom";
import { Suspense, lazy } from "react";
import ErrorBoundary from "./components/ErrorBoundary.tsx";
import Layout from "./components/Layout.tsx";
import { Skel } from "./components/LoadingSkeleton.tsx";
import { useFeaturesResolved, usePodcastsEnabled } from "./features.ts";

const VideoListPage = lazy(() => import("./pages/VideoListPage.tsx"));
const VideoDetailPage = lazy(() => import("./pages/VideoDetailPage.tsx"));
const PodcastListPage = lazy(() => import("./pages/PodcastListPage.tsx"));
const PodcastNewPage = lazy(() => import("./pages/PodcastNewPage.tsx"));
const StatsPage = lazy(() => import("./pages/StatsPage.tsx"));
const SettingsPage = lazy(() => import("./pages/SettingsPage.tsx"));

/* The route chunk is loading. Reserve the page header's height with skeleton
   blocks rather than centring a word, so the layout does not jump when the
   real header arrives. */
function Loading() {
  return (
    <div className="page-head">
      <div>
        <Skel w={90} h={10} />
        <div style={{ marginTop: 10 }}>
          <Skel w={220} h={34} />
        </div>
      </div>
    </div>
  );
}

export default function App() {
  const podcasts = usePodcastsEnabled();
  const featuresResolved = useFeaturesResolved();

  return (
    <Layout>
      <ErrorBoundary>
        <Suspense fallback={<Loading />}>
          <Routes>
            <Route path="/" element={<VideoListPage />} />
            <Route path="/video/:id" element={<VideoDetailPage />} />
            {/* Without cast2md the podcast routes are not registered at all, so
                a bookmarked or pasted podcast URL lands on the video list
                rather than on an empty page explaining a missing feature. The
                catch-all below does the redirect. */}
            {podcasts && (
              <>
                <Route path="/podcasts" element={<PodcastListPage />} />
                <Route path="/podcasts/new" element={<PodcastNewPage />} />
                {/* Podcast summaries reuse the detail page; the route differs
                    so back-links and the RSS entry links point at the right
                    list. */}
                <Route path="/podcast/:id" element={<VideoDetailPage />} />
              </>
            )}
            <Route path="/stats" element={<StatsPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            {/* The redirect waits for the feature flags. Until they arrive
                `podcasts` is false, the routes above do not exist, and a direct
                hit on /podcasts or /podcasts/new — the cast2md deep link, a
                bookmark, an RSS entry — would match here and lose its URL to
                `replace` before the answer came back. Once resolved this route
                behaves as before. */}
            <Route
              path="*"
              element={featuresResolved ? <Navigate to="/" replace /> : <Loading />}
            />
          </Routes>
        </Suspense>
      </ErrorBoundary>
    </Layout>
  );
}
