"use client";

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

/** Mini pixel-art Saker face avatar */
export function SakerAvatar({ size = 20 }: { size?: number }) {
  return (
    <svg viewBox="0 0 128 128" width={size} height={size}>
      {/* Hair */}
      <rect x="20" y="8" width="16" height="16" rx="3" fill="currentColor" opacity="0.4"/>
      <rect x="36" y="4" width="16" height="22" rx="3" fill="currentColor" opacity="0.7"/>
      <rect x="56" y="0" width="16" height="26" rx="3" fill="currentColor"/>
      <rect x="76" y="4" width="16" height="22" rx="3" fill="currentColor" opacity="0.7"/>
      <rect x="92" y="8" width="16" height="16" rx="3" fill="currentColor" opacity="0.4"/>
      {/* Left eye */}
      <rect x="10" y="38" width="24" height="24" rx="5" fill="currentColor" opacity="0.8"/>
      <rect x="16" y="44" width="12" height="12" rx="12" fill="var(--bg, #1a1a1e)"/>
      <circle cx="22" cy="50" r="3.5" fill="currentColor"/>
      {/* Right eye */}
      <rect x="94" y="38" width="24" height="24" rx="5" fill="currentColor" opacity="0.8"/>
      <rect x="100" y="44" width="12" height="12" rx="12" fill="var(--bg, #1a1a1e)"/>
      <circle cx="106" cy="50" r="3.5" fill="currentColor"/>
      {/* Mouth */}
      <rect x="34" y="84" width="54" height="10" rx="3" fill="currentColor" opacity="0.8"/>
    </svg>
  );
}
