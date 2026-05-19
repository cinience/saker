
import { useEffect, useRef } from "react";
import { useAgent, UseAgentUpdate, CopilotChat, useDefaultRenderTool } from "@copilotkit/react-core/v2";
import "@copilotkit/react-core/v2/styles.css";
import { ThreadPanel } from "./ThreadPanel";
import { EmptyState } from "./EmptyState";
import { useT } from "@/features/i18n";
import "./copilot-theme.css";
import { StatusBar } from "./StatusBar";
import { useChatStore } from "./useChatStore";
import { useIsMobile, type TurnStatus } from "./chatUtils";
import { SakerCopilotProvider } from "@/features/agui/provider";
import { useAguiHitlActions } from "@/features/agui/hitlActions";

export interface ChatMainViewProps {
  switchThread: (id: string) => Promise<void>;
  createThread: () => Promise<void>;
  deleteThread: (id: string) => void;
  onThreadStarted: (threadId: string, title: string) => void;
}

function generateTitle(text: string): string {
  const firstSentence = text.split(/[。.!?！？\n]/)[0].trim();
  if (firstSentence.length > 0 && firstSentence.length <= 40) {
    return firstSentence;
  }
  if (text.length <= 40) return text;
  const truncated = text.slice(0, 40);
  const lastSpace = truncated.lastIndexOf(" ");
  return (lastSpace > 20 ? truncated.slice(0, lastSpace) : truncated) + "...";
}

function CopilotChatArea({
  activeThreadId,
  onThreadStarted,
}: {
  activeThreadId: string;
  onThreadStarted: (threadId: string, title: string) => void;
}) {
  const { t } = useT();
  const wsConnected = useChatStore((s) => s.wsConnected);
  const wsHasBeenConnected = useChatStore((s) => s.wsHasBeenConnected);
  const turnStatus = useChatStore((s) => s.turnStatus);
  const wsHealthy = wsConnected || !wsHasBeenConnected;

  useAguiHitlActions();
  useDefaultRenderTool({
    render: ({ name, status, result }) => (
      <div className="tool-call-progress">
        <div className="tool-call-header">
          <span className="tool-call-icon">{status === "complete" ? "✓" : "⟳"}</span>
          <span className="tool-call-name">{name}</span>
          <span className={`tool-call-status tool-call-status--${status}`}>
            {status === "inProgress" ? "准备中..." : status === "executing" ? "执行中..." : "完成"}
          </span>
        </div>
        {status === "complete" && result && (
          <div className="tool-call-result">{result.length > 200 ? result.slice(0, 200) + "..." : result}</div>
        )}
      </div>
    ),
  });
  const { agent } = useAgent({ updates: [UseAgentUpdate.OnRunStatusChanged] });
  const effectiveTurnStatus: TurnStatus = agent?.isRunning ? "running" : turnStatus;

  // v2 CopilotChat overwrites onSubmitMessage internally, so the Saker
  // auto-create-thread callback never fires.  Detect when the agent starts
  // a run without an active Saker thread and sync it to the store.
  // The Go handler already persists the thread via ensureThread().
  const prevRunningRef = useRef(false);
  useEffect(() => {
    if (agent?.isRunning && !prevRunningRef.current && !activeThreadId) {
      const agentAny = agent as Record<string, unknown>;
      const threadId = agentAny.threadId as string | undefined;
      if (threadId) {
        const msgs = agentAny.messages as Array<{ role: string; content: unknown }> | undefined;
        const userMsg = msgs?.find((m) => m.role === "user");
        const content = userMsg?.content;
        const title = typeof content === "string" && content
          ? generateTitle(content)
          : "New Chat";
        onThreadStarted(threadId, title);
      }
    }
    prevRunningRef.current = agent?.isRunning ?? false;
  }, [agent?.isRunning, activeThreadId, onThreadStarted]);

  const isActive = !!activeThreadId || !!agent?.isRunning;

  return (
    <>
      <div className="chat-main-status">
        <StatusBar connected={wsHealthy} turnStatus={effectiveTurnStatus} />
      </div>
      <CopilotChat
        agentId="default"
        threadId={activeThreadId || undefined}
        className={`saker-copilot-chat${isActive ? " saker-copilot-chat--active" : ""}`}
        labels={{
          title: "",
          placeholder: t("composer.placeholder"),
          initial: "",
        }}
      />
    </>
  );
}

export function ChatMainView({
  switchThread,
  createThread,
  deleteThread,
  onThreadStarted,
}: ChatMainViewProps) {
  const isMobile = useIsMobile();
  const activeThreadId = useChatStore((s) => s.activeThreadId);
  const setMobileDrawerOpen = useChatStore((s) => s.setMobileDrawerOpen);
  const mobileDrawerOpen = useChatStore((s) => s.mobileDrawerOpen);
  const wsConnected = useChatStore((s) => s.wsConnected);
  const wsHasBeenConnected = useChatStore((s) => s.wsHasBeenConnected);
  const skills = useChatStore((s) => s.skills);
  const { t } = useT();

  const wsHealthy = wsConnected || !wsHasBeenConnected;

  return (
    <>
      {isMobile && mobileDrawerOpen && (
        <div
          className="thread-panel-overlay"
          onClick={() => setMobileDrawerOpen(false)}
        />
      )}
      <ThreadPanel
        onSelectThread={(id) => {
          switchThread(id);
          if (isMobile) setMobileDrawerOpen(false);
        }}
        onCreateThread={() => {
          createThread();
          if (isMobile) setMobileDrawerOpen(false);
        }}
        onDeleteThread={deleteThread}
      />
      <div className={`main${!activeThreadId ? " main--empty" : ""}`} id="main-content">
        {!wsHealthy && (
          <div className="connection-status" role="alert">
            {t("chat.disconnected")}
          </div>
        )}

        {!activeThreadId && (
          <div className="messages">
            <EmptyState
              connected={wsHealthy}
              skills={skills}
            />
          </div>
        )}
        <SakerCopilotProvider>
          <CopilotChatArea
            activeThreadId={activeThreadId}
            onThreadStarted={onThreadStarted}
          />
        </SakerCopilotProvider>
      </div>
    </>
  );
}
