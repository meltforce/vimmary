import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Route, Routes } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import App from "./App.tsx";
import LoginPage from "./pages/LoginPage.tsx";
import { ThemeProvider } from "./theme.tsx";
import "./index.css";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 60_000,
      refetchOnWindowFocus: false,
    },
  },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <ThemeProvider>
      <BrowserRouter>
        <QueryClientProvider client={queryClient}>
          {/* The login page stands outside App, whose shell fetches data that
              answers 401 without a session. */}
          <Routes>
            <Route path="/login" element={<LoginPage />} />
            <Route path="*" element={<App />} />
          </Routes>
        </QueryClientProvider>
      </BrowserRouter>
    </ThemeProvider>
  </StrictMode>
);
