import { lazy, Suspense } from "react";
import { ThemeProvider } from "@/features/chat/ThemeProvider";
import { I18nProvider, useT } from "@/features/i18n";
import { useAuth } from "@/features/auth/useAuth";
import { useEditorBridge } from "@/features/editor-bridge";

const ChatApp = lazy(() =>
  import("@/features/chat/ChatApp").then((m) => ({ default: m.ChatApp })),
);

function AppContent() {
  const auth = useAuth();
  const { t } = useT();
  useEditorBridge({ importedLabel: (t("canvas.editor.imported" as any) as string) || "Imported from editor" });

  if (auth.loading) {
    return <div className="auth-loading"><div className="auth-loading__spinner" /></div>;
  }

  return (
    <Suspense fallback={<div className="auth-loading"><div className="auth-loading__spinner" /></div>}>
      <ChatApp
        authRequired={auth.required}
        authenticated={auth.authenticated}
        onLogin={auth.login}
        onLogout={auth.logout}
        authProviders={auth.providers}
        onOidcLogin={auth.oidcLogin}
      />
    </Suspense>
  );
}

export default function Home() {
  return (
    <ThemeProvider>
      <I18nProvider>
        <AppContent />
      </I18nProvider>
    </ThemeProvider>
  );
}
