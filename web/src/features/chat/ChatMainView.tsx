"use client";

import { useCallback } from "react";
import { useAgent, UseAgentUpdate } from "@copilotkit/react-core/v2";
import { ThreadPanel } from "./ThreadPanel";
import { EmptyState } from "./EmptyState";
import { useT } from "@/features/i18n";
import { CopilotChat } from "@copilotkit/react-ui";
import "@copilotkit/react-ui/styles.css";
import "./copilot-theme.css";
import { StatusBar } from "./StatusBar";
import { useChatStore } from "./useChatStore";
import { useIsMobile, type TurnStatus } from "./chatUtils";

export interface ChatMainViewProps {
  switchThread: (id: string) => Promise<void>;
  createThread: () => Promise<void>;
  deleteThread: (id: string) => void;
  onAutoCreateThread: (title: string) => Promise<void>;
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

export function ChatMainView({
  switchThread,
  createThread,
  deleteThread,
  onAutoCreateThread,
}: ChatMainViewProps) {
  const { t } = useT();
  const isMobile = useIsMobile();
  const activeThreadId = useChatStore((s) => s.activeThreadId);
  const setMobileDrawerOpen = useChatStore((s) => s.setMobileDrawerOpen);
  const mobileDrawerOpen = useChatStore((s) => s.mobileDrawerOpen);
  const panelCollapsed = useChatStore((s) => s.panelCollapsed);
  const wsConnected = useChatStore((s) => s.wsConnected);
  const wsHasBeenConnected = useChatStore((s) => s.wsHasBeenConnected);
  const turnStatus = useChatStore((s) => s.turnStatus);
  const skills = useChatStore((s) => s.skills);

  const wsHealthy = wsConnected || !wsHasBeenConnected;

  const { agent } = useAgent({ updates: [UseAgentUpdate.OnRunStatusChanged] });
  const effectiveTurnStatus: TurnStatus = agent?.isRunning ? "running" : turnStatus;

  const handleSubmitMessage = useCallback(
    async (text: string) => {
      if (!activeThreadId) {
        await onAutoCreateThread(generateTitle(text));
      }
    },
    [activeThreadId, onAutoCreateThread],
  );

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
        <div className="chat-main-status">
          <StatusBar connected={wsHealthy} turnStatus={effectiveTurnStatus} />
        </div>
        <CopilotChat
          className={`saker-copilot-chat${activeThreadId ? " saker-copilot-chat--active" : ""}`}
          onSubmitMessage={handleSubmitMessage}
          labels={{
            title: "",
            placeholder: t("composer.placeholder"),
            initial: "",
          }}
          icons={{
            sendIcon: undefined,
            activityIcon: undefined,
          }}
        />
      </div>
    </>
  );
}
