import { StrictMode, lazy, Suspense } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Routes, Route, Navigate } from "react-router-dom";
import { ThemeProvider } from "next-themes";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { ThemeBridge } from "@/theme/theme-bridge";
import "@/fonts.css";
import "@/app/globals.css";

const EditorPage = lazy(() => import("@/app/editor/page"));
const ProjectsPage = lazy(() => import("@/app/projects/page"));

const SAKER_THEMES = [
  "system",
  "light",
  "dark",
  "warm-editorial",
  "cinema-gold",
  "ink-wash",
  "brutalist",
];

function App() {
  return (
    <ThemeProvider
      attribute="data-theme"
      defaultTheme="dark"
      themes={SAKER_THEMES}
      storageKey="saker-theme"
      disableTransitionOnChange={true}
    >
      <ThemeBridge />
      <TooltipProvider>
        <Toaster />
        <Suspense fallback={null}>
          <Routes>
            <Route path="/editor/*" element={<EditorPage />} />
            <Route path="/projects" element={<ProjectsPage />} />
            <Route path="*" element={<RedirectToEditor />} />
          </Routes>
        </Suspense>
      </TooltipProvider>
    </ThemeProvider>
  );
}

function RedirectToEditor() {
  const qs = window.location.search;
  return <Navigate to={qs ? `/editor/${qs}` : "/editor/"} replace />;
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <BrowserRouter basename="/editor">
      <App />
    </BrowserRouter>
  </StrictMode>,
);
