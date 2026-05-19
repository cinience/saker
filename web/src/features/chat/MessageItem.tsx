
import React, { useMemo, useEffect, useRef, useState, useCallback } from "react";
import { User, Copy, Brain } from "lucide-react";
import DOMPurify from "dompurify";
import type { ThreadItem } from "@/features/rpc/types";
import { renderMarkdown } from "./markdown";
import { useT } from "@/features/i18n";

interface MessageItemProps {
  item: ThreadItem;
  onImageClick: (url: string) => void;
}

export const MessageItem = React.memo(({ item, onImageClick }: MessageItemProps) => {
  const { t } = useT();
  const contentRef = useRef<HTMLDivElement>(null);
  const [collapsed, setCollapsed] = useState(false);
  const [isLong, setIsLong] = useState(false);

  // Handle Reasoning/Thinking Block (Custom detection for Saker)
  const isReasoning = item.role === "assistant" && item.content.includes("<thought>");
  
  const displayContent = useMemo(() => {
    if (!isReasoning) return item.content;
    return item.content.replace(/<thought>([\s\S]*?)<\/thought>/g, "");
  }, [item.content, isReasoning]);

  const thoughtText = useMemo(() => {
    if (!isReasoning) return null;
    return item.content.match(/<thought>([\s\S]*?)<\/thought>/)?.[1];
  }, [item.content, isReasoning]);

  const html = useMemo(() => {
    const rendered = renderMarkdown(displayContent);
    // Use the same sanitization as in markdown.ts but maybe slightly more relaxed if needed.
    // Actually renderMarkdown already sanitizes, so we only need to re-sanitize if we want to be STRICKER.
    // The previous code was being too strict and stripping the code block wrappers.
    return rendered;
  }, [displayContent]);

  useEffect(() => {
    if (item.role === "assistant" && contentRef.current) {
      setIsLong(contentRef.current.scrollHeight > 600);
    }
  }, [html, item.role]);

  const copyMessage = useCallback(() => {
    navigator.clipboard.writeText(item.content).then(() => {
      // visual feedback could be added here if there was a toast system available in context
    });
  }, [item.content]);

  return (
    <div className={`message ${item.role}`}>
      <div className="message-role">
        {item.role === "user" ? (
          <User size={16} />
        ) : (
          <div className="assistant-avatar">
            <SakerAvatar size={20} />
          </div>
        )}
      </div>

      <div className="message-body">
        {thoughtText && (
          <details className="thought-block-details">
            <summary className="thought-block-summary">
              <Brain size={14} style={{ color: 'var(--accent)' }} />
              <span>{t("message.thoughtProcess")}</span>
            </summary>
            <div className="thought-block-content">
              {thoughtText}
            </div>
          </details>
        )}

        <div
          ref={contentRef}
          className={`message-content${isLong && collapsed ? " content-collapsed" : ""}`}
          dangerouslySetInnerHTML={{ __html: html }}
        />
        
        {isLong && (
          <button
            className="content-toggle-btn"
            onClick={() => setCollapsed(!collapsed)}
          >
            {collapsed ? t("message.showMore") : t("message.showLess")}
          </button>
        )}
        
        {item.role === "assistant" && (
          <div className="message-actions">
            <button className="icon-btn-sm" onClick={copyMessage} title={t("message.copyMessage")}>
              <Copy size={14} />
            </button>
          </div>
        )}

        {item.artifacts?.map((a, i) => (
          <MediaPreview key={i} type={a.type} url={a.url} onImageClick={onImageClick} />
        ))}
      </div>
    </div>
  );
});

MessageItem.displayName = "MessageItem";

export const MediaPreview = React.memo(({
  type,
  url,
  onImageClick,
}: {
  type: string;
  url: string;
  onImageClick?: (url: string) => void;
}) => {
  const { t } = useT();
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState(false);

  // Timeout fallback: if image hasn't loaded or errored within 8s, treat as error
  useEffect(() => {
    if (type !== "image" || loaded || error) return;
    const timer = setTimeout(() => {
      setError(true);
    }, 8000);
    return () => clearTimeout(timer);
  }, [type, loaded, error]);

  if (type === "image") {
    return (
      <div className="tool-media">
        {!loaded && !error && <div className="media-skeleton" />}
        {error ? (
          <div className="media-error">{t("message.imageFailedToLoad")}</div>
        ) : (
          <img
            src={url}
            alt={t("message.generatedImage")}
            onLoad={() => setLoaded(true)}
            onError={() => setError(true)}
            onClick={() => onImageClick?.(url)}
            style={{
              display: loaded ? "block" : "none",
              cursor: onImageClick ? "zoom-in" : undefined,
            }}
          />
        )}
      </div>
    );
  }
  if (type === "video") {
    return (
      <div className="tool-media">
        <video src={url} controls preload="metadata" />
      </div>
    );
  }
  if (type === "audio") {
    return (
      <div className="tool-media">
        <audio src={url} controls preload="metadata" />
      </div>
    );
  }
  return null;
});

MediaPreview.displayName = "MediaPreview";

/** Mini saker falcon (猎隼) emblem — matches TUI mascot */
export function SakerAvatar({ size = 20 }: { size?: number }) {
  return (
    <svg viewBox="0 0 128 128" width={size} height={size}>
      {/* Crown (▗▄███▄▖) */}
      <rect x="30" y="4" width="68" height="28" rx="5" fill="currentColor"/>
      <rect x="18" y="16" width="16" height="16" rx="4" fill="currentColor" opacity="0.5"/>
      <rect x="94" y="16" width="16" height="16" rx="4" fill="currentColor" opacity="0.5"/>
      {/* Side bars (▐ ▌) */}
      <rect x="10" y="38" width="12" height="40" rx="3" fill="currentColor" opacity="0.6"/>
      <rect x="106" y="38" width="12" height="40" rx="3" fill="currentColor" opacity="0.6"/>
      {/* Left eye (◈) */}
      <polygon points="38,38 56,56 38,74 20,56" fill="currentColor" opacity="0.85"/>
      <circle cx="38" cy="56" r="6" fill="var(--bg, #0f0f12)"/>
      <circle cx="38" cy="56" r="2.5" fill="currentColor"/>
      {/* Right eye (◈) */}
      <polygon points="90,38 108,56 90,74 72,56" fill="currentColor" opacity="0.85"/>
      <circle cx="90" cy="56" r="6" fill="var(--bg, #0f0f12)"/>
      <circle cx="90" cy="56" r="2.5" fill="currentColor"/>
      {/* Beak (▼) */}
      <polygon points="64,42 76,66 52,66" fill="currentColor"/>
      {/* Jaw (▀▄█▄▀) */}
      <rect x="42" y="88" width="44" height="22" rx="5" fill="currentColor"/>
      <rect x="30" y="88" width="16" height="14" rx="4" fill="currentColor" opacity="0.6"/>
      <rect x="82" y="88" width="16" height="14" rx="4" fill="currentColor" opacity="0.6"/>
    </svg>
  );
}
