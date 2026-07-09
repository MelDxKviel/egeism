import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AppProvider } from "./state";
import App from "./App";
import { ErrorBoundary } from "./error";
import "./theme.css";

const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false, staleTime: 30_000 } },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    {/* Last-resort boundary: even if the provider tree throws, show a card
        instead of a blank page. Per-screen boundaries live inside App. */}
    <ErrorBoundary>
      <QueryClientProvider client={queryClient}>
        <AppProvider>
          <App />
        </AppProvider>
      </QueryClientProvider>
    </ErrorBoundary>
  </StrictMode>
);
