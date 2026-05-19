
import { useMemo } from "react";
import { X, MessageCircle } from "lucide-react";
import { CanvasView } from "@/features/canvas/CanvasView";
import { ThreadPanel } from "./ThreadPanel";
import { StatusBar } from "./StatusBar";
import { Composer } from "./Composer";
import { type Attachment } from "./useFileUpload";
import { ChatStream } from "./ChatStream";
import { StarterState } from "./StarterState";
import { useT } from "@/features/i18n";
import { useChatStore } from "./useChatStore";
import { useCanvasStore } from "@/features/canvas/store";

export interface CanvasLayoutProps {
  switchThread: (id: string) => Promise<void>;
  createThread: () => Promise<void>;
  deleteThread: (id: string) => void;
  onApproval: (id: string, decision: "allow" | "deny") => void;
  onQuestionRespond: (id: string, answers: Record<string, string>) => void;
  sendMessage: (text: string, attachments?: Attachment[]) => void;
  sendWithAutoCreate: (text: string, attachments?: Attachment[]) => void;
  cancelTurn: () => void;
}

export function CanvasLayout({
  switchThread,
  createThread,
  deleteThread,
  onApproval,
  onQuestionRespond,
  sendMessage,
  sendWithAutoCreate,
  cancelTurn,
}: CanvasLayoutProps) {
  const { t } = useT();

  const threads = useChatStore((s) => s.threads);
  const activeThreadId = useChatStore((s) => s.activeThreadId);
  const panelCollapsed = useChatStore((s) => s.panelCollapsed);
  const wsConnected = useChatStore((s) => s.wsConnected);
  const wsHasBeenConnected = useChatStore((s) => s.wsHasBeenConnected);
  const canvasChatOpen = useChatStore((s) => s.canvasChatOpen);
  const setCanvasChatOpen = useChatStore((s) => s.setCanvasChatOpen);
  const messages = useChatStore((s) => s.messages);
  const streamText = useChatStore((s) => s.streamText);
  const turnStatus = useChatStore((s) => s.turnStatus);
  const toolEvents = useChatStore((s) => s.toolEvents);
  const approvals = useChatStore((s) => s.approvals);
  const questions = useChatStore((s) => s.questions);
  const skills = useChatStore((s) => s.skills);

  const wsHealthy = wsConnected || !wsHasBeenConnected;

  const canvasNodes = useCanvasStore((s) => s.nodes);
  const canvasHasNodes = canvasNodes.length > 0;
  const highlightedTurnId = useCanvasStore((s) => s.highlightedTurnId);

  const activeThread = useMemo(
    () => threads.find((t) => t.id === activeThreadId),
    [threads, activeThreadId]
  );

  return (
    <>
      <ThreadPanel
        onSelectThread={switchThread}
        onCreateThread={createThread}
        onDeleteThread={deleteThread}
      />
      <div className={`canvas-layout${panelCollapsed ? "" : " panel-expanded"}`}>
        {/* Canvas area — shrinks when drawer opens */}
        <div className="canvas-area">
          <CanvasView />
          {!canvasChatOpen && !canvasHasNodes && (
            <div className="composer-area floating-composer">
              <StatusBar connected={wsHealthy} turnStatus={turnStatus} />
              <Composer
                onSend={activeThreadId ? sendMessage : sendWithAutoCreate}
                onStop={cancelTurn}
                disabled={!wsHealthy || turnStatus === "running"}
                running={turnStatus === "running"}
                skills={skills}
              />
            </div>
          )}
          {/* Floating chat ball */}
          {!canvasChatOpen && (
            <button
              className="canvas-chat-fab"
              onClick={() => { setCanvasChatOpen(true); requestAnimationFrame(() => window.dispatchEvent(new Event("resize"))); }}
              aria-label={t("chat.openChat")}
            >
              <MessageCircle size={22} strokeWidth={1.75} />
            </button>
          )}
        </div>

        {/* Right-side chat drawer — same level as canvas */}
        {canvasChatOpen && (
          <div className="canvas-chat-drawer">
            <div className="canvas-chat-drawer-header">
              <h4 className="canvas-chat-drawer-title">
                {activeThread?.title || t("nav.chats")}
              </h4>
              <button
                className="canvas-chat-drawer-close"
                onClick={() => { setCanvasChatOpen(false); requestAnimationFrame(() => window.dispatchEvent(new Event("resize"))); }}
                aria-label={t("chat.closeChat")}
              >
                <X size={18} strokeWidth={2} />
              </button>
            </div>
            <div className="canvas-chat-drawer-messages">
              {!activeThreadId ? (
                <div className="canvas-chat-drawer-empty">
                  <p>{t("chat.selectOrCreate")}</p>
                </div>
              ) : messages.length === 0 && !streamText && turnStatus === "idle" ? (
                <StarterState onSend={sendMessage} />
              ) : (
                <ChatStream
                  messages={messages}
                  streamText={streamText}
                  streaming={turnStatus === "running"}
                  toolEvents={toolEvents}
                  highlightedTurnId={highlightedTurnId}
                  approvals={approvals}
                  questions={questions}
                  onApproval={onApproval}
                  onQuestionRespond={onQuestionRespond}
                />
              )}
            </div>

            <div className="composer-area floating-composer">
              <Composer
                onSend={activeThreadId ? sendMessage : sendWithAutoCreate}
                onStop={cancelTurn}
                disabled={!wsHealthy || turnStatus === "running"}
                running={turnStatus === "running"}
                skills={skills}
              />
            </div>
          </div>
        )}
      </div>
    </>
  );
}
