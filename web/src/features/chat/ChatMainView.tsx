
import React from "react";
import { useEffect, useRef, useState, useCallback } from "react";
import { useAgent, UseAgentUpdate, CopilotChat, useDefaultRenderTool, useConfigureSuggestions } from "@copilotkit/react-core/v2";
import "@copilotkit/react-core/v2/styles.css";
import { toast } from "sonner";
import { X, RefreshCw, ArrowDown } from "lucide-react";
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
import { httpRequest } from "@/features/rpc/httpRpc";
import { CopilotSlashTextArea } from "./CopilotSlashTextArea";
import { getAgentInternals, getLatestUserText } from "./useAgentInternals";

class ChatErrorBoundary extends React.Component<
  { children: React.ReactNode },
  { hasError: boolean; error: Error | null }
> {
  state = { hasError: false, error: null as Error | null };

  static getDerivedStateFromError(error: Error) {
    return { hasError: true, error };
  }

  render() {
    if (this.state.hasError) {
      return (
        <div className="chat-error-fallback">
          <div className="chat-error-icon">⚠</div>
          <p className="chat-error-text">Chat encountered an error</p>
          <button
            className="chat-error-retry"
            onClick={() => this.setState({ hasError: false, error: null })}
          >
            <RefreshCw size={14} />
            <span>Retry</span>
          </button>
        </div>
      );
    }
    return this.props.children;
  }
}

export interface ChatMainViewProps {
  switchThread: (id: string) => Promise<void>;
  createThread: () => Promise<void>;
  deleteThread: (id: string) => void;
  onThreadStarted: (threadId: string, title: string) => void;
}

function generateTitle(text: string): string {
  const cleaned = text.replace(/\s+/g, " ").trim();
  const firstSentence = cleaned.split(/[。.!?！？\n，,;；]/)[0].trim();
  if (firstSentence.length > 0 && firstSentence.length <= 30) {
    return firstSentence;
  }
  if (cleaned.length <= 30) return cleaned;
  const truncated = cleaned.slice(0, 30);
  const lastBreak = Math.max(
    truncated.lastIndexOf(" "),
    truncated.lastIndexOf("，"),
    truncated.lastIndexOf(","),
    truncated.lastIndexOf("、"),
  );
  return (lastBreak > 10 ? truncated.slice(0, lastBreak) : truncated) + "...";
}

function CopilotChatArea({
  activeThreadId,
  onThreadStarted,
}: {
  activeThreadId: string;
  onThreadStarted: (threadId: string, title: string) => void;
}) {
  const { t } = useT();
  const serverReachable = useChatStore((s) => s.serverReachable);
  const turnStatus = useChatStore((s) => s.turnStatus);
  const [lightboxUrl, setLightboxUrl] = useState<string | null>(null);
  const onImageClick = useCallback((url: string) => setLightboxUrl(url), []);
  const lightboxCloseRef = useRef<HTMLButtonElement | null>(null);

  useEffect(() => {
    if (!lightboxUrl) return;
    lightboxCloseRef.current?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setLightboxUrl(null);
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [lightboxUrl]);
  const [showScrollBtn, setShowScrollBtn] = useState(false);
  const scrollContainerRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    const THRESHOLD = 100;
    const onScroll = (e: Event) => {
      const el = e.target as HTMLElement;
      const distanceFromBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
      setShowScrollBtn(distanceFromBottom > THRESHOLD);
    };

    const observer = new MutationObserver(() => {
      const container = document.querySelector<HTMLElement>(
        ".saker-copilot-chat [class*='cpk:overflow-y-auto'], .saker-copilot-chat .copilotKitMessages"
      );
      if (container && container !== scrollContainerRef.current) {
        scrollContainerRef.current?.removeEventListener("scroll", onScroll);
        scrollContainerRef.current = container;
        container.addEventListener("scroll", onScroll, { passive: true });
      }
    });

    observer.observe(document.body, { childList: true, subtree: true });

    return () => {
      observer.disconnect();
      scrollContainerRef.current?.removeEventListener("scroll", onScroll);
    };
  }, []);

  useAguiHitlActions();
  useConfigureSuggestions({
    suggestions: [
      { title: t("suggestion.generateImage"), message: t("suggestion.generateImageMsg") },
      { title: t("suggestion.textToSpeech"), message: t("suggestion.textToSpeechMsg") },
      { title: t("suggestion.analyzeVideo"), message: t("suggestion.analyzeVideoMsg") },
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
              {status === "inProgress" ? t("tool.preparing") : status === "executing" ? t("tool.executing") : t("tool.complete")}
            </span>
          </div>
          {status === "complete" && media ? (
            <MediaPreview type={media.type} url={media.url} onImageClick={onImageClick} />
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

  const effectiveTurnStatus: TurnStatus = agent?.isRunning ? "running" : turnStatus;

  // Track the threadId to pass to CopilotChat. We defer passing a newly-created
  // thread's ID until after the first run completes to prevent a race: setting
  // threadId while the run is active triggers connectAgent() which fires
  // MESSAGES_SNAPSHOT and wipes the in-flight streaming response.
  const [chatThreadId, setChatThreadId] = useState(activeThreadId);
  const pendingThreadRef = useRef<{ id: string; title: string } | null>(null);
  const prevRunningRef = useRef(false);

  const threads = useChatStore((s) => s.threads);
  const setThreads = useChatStore((s) => s.setThreads);
  const titleUpdatedRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    if (agent?.isRunning && !prevRunningRef.current && !activeThreadId) {
      const { threadId, messages } = getAgentInternals(agent);
      if (threadId) {
        const text = getLatestUserText(messages);
        const title = text ? generateTitle(text) : "New Chat";
        pendingThreadRef.current = { id: threadId, title };
        onThreadStarted(threadId, title);
      }
    }

    if (agent?.isRunning && !prevRunningRef.current && activeThreadId) {
      const thread = threads.find((t) => t.id === activeThreadId);
      if (thread && (thread.title === "New Chat" || thread.title === "New Thread") && !titleUpdatedRef.current.has(activeThreadId)) {
        const { messages } = getAgentInternals(agent);
        const text = getLatestUserText(messages, true);
        if (text) {
          const title = generateTitle(text);
          titleUpdatedRef.current.add(activeThreadId);
          setThreads((prev) => prev.map((t) => (t.id === activeThreadId ? { ...t, title } : t)));
          httpRequest("thread/update", { threadId: activeThreadId, title }).catch(() => {});
        }
      }
    }

    if (!agent?.isRunning && prevRunningRef.current && pendingThreadRef.current) {
      setChatThreadId(pendingThreadRef.current.id);
      pendingThreadRef.current = null;
    }
    prevRunningRef.current = agent?.isRunning ?? false;
  }, [agent?.isRunning, activeThreadId, onThreadStarted, threads, setThreads]);

  useEffect(() => {
    if (!pendingThreadRef.current) {
      setChatThreadId(activeThreadId);
    }
  }, [activeThreadId]);

  const isActive = !!activeThreadId || !!agent?.isRunning;

  return (
    <>
      <div className="chat-main-status">
        <StatusBar connected={serverReachable} turnStatus={effectiveTurnStatus} />
      </div>
      <CopilotChat
        agentId="default"
        threadId={chatThreadId || undefined}
        className={`saker-copilot-chat${isActive ? " saker-copilot-chat--active" : ""}`}
        input={{ textArea: CopilotSlashTextArea }}
        labels={{
          chatInputPlaceholder: t("composer.placeholder"),
          chatInputToolbarAddButtonLabel: t("composer.addAttachments"),
          chatDisclaimerText: t("composer.disclaimerCpk"),
        }}
        attachments={{
          enabled: true,
          accept: "image/*,video/*,audio/*,application/pdf",
          maxSize: 50 * 1024 * 1024,
          onUpload: (file: File) => {
            const base = window.location.port === "10111"
              ? `${window.location.protocol}//${window.location.hostname}:17000`
              : "";
            const shortName = file.name.length > 20 ? file.name.slice(0, 18) + "…" : file.name;
            const toastId = toast.loading(`${t("composer.uploading")}: ${shortName}`);

            return new Promise((resolve, reject) => {
              const xhr = new XMLHttpRequest();
              xhr.open("POST", `${base}/api/upload`);
              xhr.withCredentials = true;

              xhr.upload.onprogress = (e) => {
                if (e.lengthComputable) {
                  const pct = Math.round((e.loaded / e.total) * 100);
                  toast.loading(`${t("composer.uploading")}: ${shortName} (${pct}%)`, { id: toastId });
                }
              };

              xhr.onload = () => {
                if (xhr.status >= 200 && xhr.status < 300) {
                  toast.dismiss(toastId);
                  const { path, media_type } = JSON.parse(xhr.responseText);
                  resolve({ type: "url" as const, value: path, mimeType: media_type, metadata: { filename: file.name } });
                } else {
                  toast.error(`${t("composer.uploadFailed")}: ${shortName}`, { id: toastId });
                  reject(new Error(`Upload failed: ${xhr.statusText}`));
                }
              };

              xhr.onerror = () => {
                toast.error(`${t("composer.uploadFailed")}: ${shortName}`, { id: toastId });
                reject(new Error("Network error"));
              };

              const form = new FormData();
              form.append("file", file);
              xhr.send(form);
            });
          },
          onUploadFailed: (info: { reason: string; file: File; message: string }) => {
            const name = info.file.name.length > 20 ? info.file.name.slice(0, 18) + "…" : info.file.name;
            if (info.reason === "invalid-type") {
              toast.error(`${t("composer.fileTypeUnsupported")}: ${name}`);
            } else if (info.reason === "file-too-large") {
              toast.error(`${t("composer.fileTooLarge")}: ${name}`);
            } else {
              toast.error(`${t("composer.uploadFailed")}: ${name}`);
            }
          },
        }}
        messageView={{
          assistantMessage: { onRegenerate: () => {} },
          userMessage: { onEditMessage: () => {} },
        }}
      />
      {showScrollBtn && (
        <button
          className="scroll-to-bottom-btn"
          onClick={() => {
            scrollContainerRef.current?.scrollTo({ top: scrollContainerRef.current.scrollHeight, behavior: "smooth" });
          }}
          aria-label={t("message.scrollToBottom")}
        >
          <ArrowDown size={16} />
        </button>
      )}
      {lightboxUrl && (
        <div className="lightbox-overlay" onClick={() => setLightboxUrl(null)} role="dialog" aria-modal="true" aria-label={t("message.fullSizePreview")}>
          <button
            ref={lightboxCloseRef}
            className="lightbox-close"
            aria-label={t("message.closeLightbox")}
            onClick={(e) => { e.stopPropagation(); setLightboxUrl(null); }}
          >
            <X size={20} />
          </button>
          <img
            src={lightboxUrl}
            alt={t("message.fullSizePreview")}
            className="lightbox-img"
            onClick={(e) => e.stopPropagation()}
          />
        </div>
      )}
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
  const serverReachable = useChatStore((s) => s.serverReachable);
  const skills = useChatStore((s) => s.skills);
  const { t } = useT();

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
        {!serverReachable && (
          <div className="connection-status" role="alert">
            {t("chat.disconnected")}
          </div>
        )}

        {!activeThreadId && (
          <div className="messages">
            <EmptyState
              connected={serverReachable}
              skills={skills}
            />
          </div>
        )}
        <ChatErrorBoundary>
          <SakerCopilotProvider>
            <CopilotChatArea
              activeThreadId={activeThreadId}
              onThreadStarted={onThreadStarted}
            />
          </SakerCopilotProvider>
        </ChatErrorBoundary>
      </div>
    </>
  );
}
