
import { useEffect, useRef, useMemo, useCallback, useState } from "react";
import { motion, AnimatePresence } from "framer-motion";
import { X, ArrowDown } from "lucide-react";
import type { ThreadItem, StreamEvent } from "@/features/rpc/types";
import { extractMedia } from "@/features/media/extractMedia";
import { useT } from "@/features/i18n";
import { MessageItem, SakerAvatar } from "./MessageItem";
import { ToolGroupCollapse, StreamingToolCard, type ToolCardData } from "./ToolItem";

/** Scroll to bottom with RAF throttling to avoid layout thrashing during streaming. */
function useThrottledScrollToBottom(
  bottomRef: React.RefObject<HTMLDivElement | null>,
  deps: unknown[]
) {
  const rafRef = useRef<number>(0);
  const isNearBottomRef = useRef(true);

  useEffect(() => {
    const container = bottomRef.current?.parentElement;
    if (container) {
      const threshold = 100;
      isNearBottomRef.current =
        container.scrollHeight - container.scrollTop - container.clientHeight < threshold;
    }

    if (!isNearBottomRef.current) return;

    if (rafRef.current) cancelAnimationFrame(rafRef.current);
    rafRef.current = requestAnimationFrame(() => {
      bottomRef.current?.scrollIntoView({ behavior: "smooth" });
      rafRef.current = 0;
    });

    return () => {
      if (rafRef.current) cancelAnimationFrame(rafRef.current);
    };
  }, deps); // eslint-disable-line react-hooks/exhaustive-deps
}

interface Props {
  messages: ThreadItem[];
  streamText: string;
  streaming: boolean;
  toolEvents: StreamEvent[];
  /** Currently highlighted turn ID from canvas node click. */
  highlightedTurnId?: string | null;
}

interface TurnGroup {
  turnId: string;
  user?: ThreadItem;
  items: ThreadItem[];
}

export function MessageStream({
  messages,
  streamText,
  streaming,
  toolEvents,
  highlightedTurnId,
}: Props) {
  const { t } = useT();
  const bottomRef = useRef<HTMLDivElement>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const [lightboxUrl, setLightboxUrl] = useState<string | null>(null);
  const [showScrollBottom, setShowScrollBottom] = useState(false);

  useThrottledScrollToBottom(bottomRef, [messages, streamText, toolEvents]);

  // Handle scroll events to show/hide "Scroll to bottom" button
  useEffect(() => {
    const container = containerRef.current?.parentElement;
    if (!container) return;

    const handleScroll = () => {
      const isAtBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 200;
      setShowScrollBottom(!isAtBottom);
    };

    container.addEventListener("scroll", handleScroll);
    return () => container.removeEventListener("scroll", handleScroll);
  }, []);

  const scrollToBottom = useCallback(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, []);

  const handleClick = useCallback((e: React.MouseEvent) => {
    const target = e.target as HTMLElement;

    // Handle image clicks in message content — open lightbox
    if (target.tagName === "IMG" && target.closest(".message-content")) {
      const src = (target as HTMLImageElement).src;
      if (src) {
        e.preventDefault();
        setLightboxUrl(src);
        return;
      }
    }

    if (!target.classList.contains("copy-btn")) return;
    const wrapper = target.closest(".code-block-wrapper");
    if (!wrapper) return;
    const code = wrapper.querySelector("code");
    if (!code) return;
    navigator.clipboard.writeText(code.textContent || "").then(() => {
      const originalText = target.textContent;
      target.textContent = t("message.copied");
      setTimeout(() => {
        target.textContent = originalText || t("message.copy");
      }, 2000);
    });
  }, [t]);

  useEffect(() => {
    if (!lightboxUrl) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setLightboxUrl(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [lightboxUrl]);

  const turnGroups = useMemo(() => {
    const turns = new Map<string, TurnGroup>();
    const standalone: ThreadItem[] = [];
    const turnOrder: string[] = [];

    for (const m of messages) {
      if (!m.turn_id) {
        standalone.push(m);
        continue;
      }
      if (!turns.has(m.turn_id)) {
        turns.set(m.turn_id, { turnId: m.turn_id, items: [] });
        turnOrder.push(m.turn_id);
      }
      const group = turns.get(m.turn_id)!;
      if (m.role === "user") group.user = m;
      else group.items.push(m);
    }
    return { standalone, turns, turnOrder };
  }, [messages]);

  const toolCards = useMemo(() => {
    const cards: ToolCardData[] = [];
    for (const evt of toolEvents) {
      const name = evt.name || "tool";
      const tid = evt.tool_use_id || "";

      if (evt.type === "tool_execution_start") {
        cards.push({
          name,
          toolUseId: tid,
          status: "running",
          outputs: [],
          isError: false,
        });
      } else if (evt.type === "tool_execution_output") {
        const card = cards.find((c) => c.toolUseId === tid) || cards[cards.length - 1];
        if (card) {
          const text =
            typeof evt.output === "string"
              ? evt.output
              : JSON.stringify(evt.output);
          card.outputs.push(text);
          card.status = "output";
          if (evt.is_error) card.isError = true;
        }
      } else if (evt.type === "tool_execution_result") {
        const card = cards.find((c) => c.toolUseId === tid) || cards[cards.length - 1];
        if (card) {
          card.status = evt.is_error ? "error" : "done";
          if (evt.is_error) card.isError = true;
          if (!evt.is_error) {
            const media = extractMedia(evt);
            if (media) card.media = media;
          }
        }
      }
    }
    return cards;
  }, [toolEvents]);

  const onImageClick = useCallback((url: string) => setLightboxUrl(url), []);

  return (
    <div className="messages-inner" ref={containerRef} onClick={handleClick}>
      {turnGroups.standalone.map((m) => (
        <MessageItem key={m.id} item={m} onImageClick={onImageClick} />
      ))}

      {turnGroups.turnOrder.map((turnId) => {
        const group = turnGroups.turns.get(turnId)!;
        const runs: { role: "assistant" | "tool"; items: ThreadItem[] }[] = [];
        for (const item of group.items) {
          const role = item.role === "tool" ? "tool" : "assistant";
          const last = runs[runs.length - 1];
          if (last && last.role === role) {
            last.items.push(item);
          } else {
            runs.push({ role, items: [item] });
          }
        }
        const isHighlighted = highlightedTurnId === turnId;
        return (
          <div
            key={turnId}
            className={`turn-group${isHighlighted ? " turn-highlighted" : ""}`}
            data-turn-id={turnId}
          >
            {group.user && (
              <MessageItem item={group.user} onImageClick={onImageClick} />
            )}
            {runs.map((run, i) =>
              run.role === "assistant" ? (
                run.items.map((m) => (
                  <MessageItem key={m.id} item={m} onImageClick={onImageClick} />
                ))
              ) : (
                <ToolGroupCollapse key={`tools-${i}`} tools={run.items} onImageClick={onImageClick} />
              )
            )}
          </div>
        );
      })}

      {toolCards.map((card, i) => (
        <StreamingToolCard key={`${card.toolUseId}-${i}`} card={card} onImageClick={onImageClick} />
      ))}

      {streaming && streamText && (
        <motion.div
          initial={{ opacity: 0, y: 10 }}
          animate={{ opacity: 1, y: 0 }}
          className="message assistant"
        >
          <div className="message-role">
            <div className="assistant-avatar">
              <SakerAvatar size={18} />
            </div>
          </div>
          <div className="message-content">
            <pre className="stream-pre">{streamText}<span className="streaming-cursor" /></pre>
          </div>
        </motion.div>
      )}

      <div ref={bottomRef} />

      <AnimatePresence>
        {showScrollBottom && (
          <motion.button
            initial={{ opacity: 0, scale: 0.8, y: 20 }}
            animate={{ opacity: 1, scale: 1, y: 0 }}
            exit={{ opacity: 0, scale: 0.8, y: 20 }}
            className="scroll-bottom-btn"
            onClick={scrollToBottom}
            aria-label={t("message.scrollToBottom")}
          >
            <ArrowDown size={18} />
          </motion.button>
        )}
      </AnimatePresence>

      {lightboxUrl && (
        <div className="lightbox-overlay" onClick={() => setLightboxUrl(null)}>
          <button
            className="lightbox-close"
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
    </div>
  );
}
