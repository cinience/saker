import { StrictMode, lazy, Suspense } from "react";
import { createRoot } from "react-dom/client";
import { Toaster } from "sonner";
import { ErrorBoundary } from "@/components/ErrorBoundary";
import "@/fonts.css";
import "@/app/globals.css";

const Home = lazy(() => import("@/app/page"));
const ShareApp = lazy(() =>
  import("@/app/share/[...slug]/ShareClient").then((m) => ({
    default: m.ShareApp,
  })),
);

function Loading() {
  return (
    <div className="auth-loading">
      <div className="auth-loading__spinner" />
    </div>
  );
}

function App() {
  const path = window.location.pathname;
  const isShare = path.startsWith("/share");

  return (
    <ErrorBoundary>
      <Suspense fallback={<Loading />}>
        {isShare ? <ShareApp /> : <Home />}
      </Suspense>
      <Toaster position="bottom-right" richColors />
    </ErrorBoundary>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
