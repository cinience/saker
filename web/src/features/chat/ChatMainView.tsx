
import { useEffect, useRef, useState } from "react";
import { useAgent, UseAgentUpdate, CopilotChat, useDefaultRenderTool, useConfigureSuggestions } from "@copilotkit/react-core/v2";
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
import { MediaPreview } from "./MessageItem";
import { extractMediaResultFromToolResult } from "./mediaResult";

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
  useConfigureSuggestions({
    suggestions: [
      { title: "生成一张图片", message: "帮我生成一张赛博朋克风格的城市图片" },
      { title: "文字转语音", message: "把下面的文字转成语音：你好世界" },
      { title: "分析视频内容", message: "分析这个视频的内容" },
    ],
  });
  useDefaultRenderTool({
    render: ({ name, status, result }) => {
      const media = extractMediaResultFromToolResult(name, result);
      return (
        <div className="tool-call-progress">
          <div className="tool-call-header">
            <span className="tool-call-icon">{status === "complete" ? "✓" : "⟳"}</span>
            <span className="tool-call-name">{name}</span>
            <span className={`tool-call-status tool-call-status--${status}`}>
              {status === "inProgress" ? "准备中..." : status === "executing" ? "执行中..." : "完成"}
            </span>
          </div>
          {status === "complete" && media ? (
            <MediaPreview type={media.type} url={media.url} />
          ) : status === "complete" && result ? (
            <div className="tool-call-result">{result.length > 200 ? result.slice(0, 200) + "..." : result}</div>
          ) : null}
        </div>
      );
    },
  });
  const { agent } = useAgent({
    updates: [UseAgentUpdate.OnRunStatusChanged, UseAgentUpdate.OnStateChanged, UseAgentUpdate.OnMessagesChanged],
    throttleMs: 100,
  });

  // Inject media artifacts into the last assistant message when a run completes.
  // CopilotKit's Streamdown strips <img> and markdown images, so we inject after render.
  // Strategy: capture artifacts from agent.state DURING the run (state resets after),
  // then inject into DOM when the run finishes. Fallback: parse message content for <img>.
  const pendingArtifactsRef = useRef<Array<{ type: string; url: string; name?: string }>>([]);
  const agentAny = agent as unknown as Record<string, unknown> | undefined;
  const agentState = agentAny?.state as { artifacts?: Array<{ type: string; url: string; name?: string }> } | undefined;
  const agentRunning = agent?.isRunning ?? false;

  useEffect(() => {
    if (!agentAny) return;
    const stateArtifacts = agentState?.artifacts;
    if (agentRunning && stateArtifacts && stateArtifacts.length > 0) {
      pendingArtifactsRef.current = [...stateArtifacts];
      return;
    }
    if (agentRunning) return;

    // Run finished. Resolve artifacts: ref first, then current state, then message parsing.
    let toInject = pendingArtifactsRef.current;
    pendingArtifactsRef.current = [];
    if (toInject.length === 0 && stateArtifacts && stateArtifacts.length > 0) {
      toInject = [...stateArtifacts];
    }
    if (toInject.length === 0) {
      const msgs = (agentAny.messages as Array<{ role: string; content?: string }>) ?? [];
      const lastAssistant = [...msgs].reverse().find(m => m.role === "assistant");
      if (lastAssistant?.content) {
        const imgRe = /<img[^>]+src="([^"]+)"[^>]*>/g;
        let match;
        while ((match = imgRe.exec(lastAssistant.content)) !== null) {
          toInject.push({ type: "image", url: match[1], name: "generated" });
        }
      }
    }
    if (toInject.length === 0) return;

    const timer = setTimeout(() => {
      const container = document.querySelector("[data-testid=\"copilot-message-list\"]");
      if (!container) return;
      const msgEls = container.querySelectorAll("[data-testid=\"copilot-assistant-message\"]");
      const lastMsg = msgEls[msgEls.length - 1];
      if (!lastMsg || lastMsg.querySelector(".saker-injected-media")) return;
      const mediaDiv = document.createElement("div");
      mediaDiv.className = "saker-injected-media";
      mediaDiv.style.cssText = "margin-top:12px";
      for (const a of toInject) {
        if (a.type === "video") {
          const el = document.createElement("video");
          el.src = a.url;
          el.controls = true;
          el.style.cssText = "max-width:100%;border-radius:8px;margin-top:8px";
          mediaDiv.appendChild(el);
        } else if (a.type === "audio") {
          const el = document.createElement("audio");
          el.src = a.url;
          el.controls = true;
          el.style.cssText = "margin-top:8px";
          mediaDiv.appendChild(el);
        } else {
          const el = document.createElement("img");
          el.src = a.url;
          el.alt = a.name || "generated image";
          el.style.cssText = "max-width:100%;border-radius:8px;margin-top:8px;display:block";
          el.loading = "lazy";
          mediaDiv.appendChild(el);
        }
      }
      const prose = lastMsg.querySelector("[class*='prose']");
      if (prose) {
        prose.appendChild(mediaDiv);
      } else {
        lastMsg.insertBefore(mediaDiv, lastMsg.querySelector("[data-testid='copilot-assistant-toolbar']"));
      }
    }, 300);
    return () => clearTimeout(timer);
  }, [agentAny, agentState, agentRunning]);
  const effectiveTurnStatus: TurnStatus = agent?.isRunning ? "running" : turnStatus;

  // Track the threadId to pass to CopilotChat. We defer passing a newly-created
  // thread's ID until after the first run completes to prevent a race: setting
  // threadId while the run is active triggers connectAgent() which fires
  // MESSAGES_SNAPSHOT and wipes the in-flight streaming response.
  const [chatThreadId, setChatThreadId] = useState(activeThreadId);
  const pendingThreadRef = useRef<{ id: string; title: string } | null>(null);
  const prevRunningRef = useRef(false);

  useEffect(() => {
    if (agent?.isRunning && !prevRunningRef.current && !activeThreadId) {
      const agentAny = agent as unknown as Record<string, unknown>;
      const threadId = agentAny.threadId as string | undefined;
      if (threadId) {
        const msgs = agentAny.messages as Array<{ role: string; content: unknown }> | undefined;
        const userMsg = msgs?.find((m) => m.role === "user");
        const raw = userMsg?.content;
        let text = "";
        if (typeof raw === "string") {
          text = raw;
        } else if (Array.isArray(raw)) {
          text = (raw as Array<Record<string, unknown>>)
            .filter((p) => p?.type === "text" && typeof p.text === "string")
            .map((p) => p.text as string)
            .join("\n");
        }
        const title = text ? generateTitle(text) : "New Chat";
        pendingThreadRef.current = { id: threadId, title };
        onThreadStarted(threadId, title);
      }
    }
    if (!agent?.isRunning && prevRunningRef.current && pendingThreadRef.current) {
      setChatThreadId(pendingThreadRef.current.id);
      pendingThreadRef.current = null;
    }
    prevRunningRef.current = agent?.isRunning ?? false;
  }, [agent?.isRunning, activeThreadId, onThreadStarted]);

  useEffect(() => {
    if (!pendingThreadRef.current) {
      setChatThreadId(activeThreadId);
    }
  }, [activeThreadId]);

  const isActive = !!activeThreadId || !!agent?.isRunning;

  return (
    <>
      <div className="chat-main-status">
        <StatusBar connected={wsHealthy} turnStatus={effectiveTurnStatus} />
      </div>
      <CopilotChat
        agentId="default"
        threadId={chatThreadId || undefined}
        className={`saker-copilot-chat${isActive ? " saker-copilot-chat--active" : ""}`}
        labels={{
          chatInputPlaceholder: t("composer.placeholder"),
        }}
        messageView={{ assistantMessage: { onRegenerate: () => {} } }}
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
