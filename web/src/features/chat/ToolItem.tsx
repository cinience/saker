
import React from "react";
import { ChevronDown } from "lucide-react";
import { motion } from "framer-motion";
import type { ThreadItem } from "@/features/rpc/types";
import { useT } from "@/features/i18n";
import { MediaPreview } from "./MessageItem";

interface ToolItemCardProps {
  item: ThreadItem;
  onImageClick: (url: string) => void;
}

export const ToolItemCard = React.memo(({ item, onImageClick }: ToolItemCardProps) => {
  const { t } = useT();

  if (!item.content && item.artifacts?.length) {
    return (
      <div className="message tool">
        {item.artifacts.map((a, i) => (
          <MediaPreview key={i} type={a.type} url={a.url} onImageClick={onImageClick} />
        ))}
      </div>
    );
  }

  const toolName = item.tool_name || parseToolContent(item.content).name;
  const output = item.tool_name
    ? item.content
    : parseToolContent(item.content).output;
  const hasArtifacts = item.artifacts && item.artifacts.length > 0;
  const lineCount = output ? output.split("\n").length : 0;
  const inferredMedia = !hasArtifacts ? extractMediaFromText(output) : null;

  return (
    <div className="tool-card">
      <div className="tool-card-header">
        <span className="tool-status-icon status-done" />
        <span className="tool-name">{toolName}</span>
        <span className="tool-status-label">{t("message.done")}</span>
      </div>
      {output && output.trim() && (
        <details className="tool-output-details">
          <summary>{t("message.output")} ({lineCount} {lineCount === 1 ? t("message.line") : t("message.lines")})</summary>
          <pre className="tool-output">
            {output.slice(0, 2000)}
            {output.length > 2000 ? "\n..." : ""}
          </pre>
        </details>
      )}
      {hasArtifacts &&
        item.artifacts!.map((a, i) => (
          <MediaPreview key={i} type={a.type} url={a.url} onImageClick={onImageClick} />
        ))}
      {inferredMedia && (
        <MediaPreview type={inferredMedia.type} url={inferredMedia.url} onImageClick={onImageClick} />
      )}
    </div>
  );
});

ToolItemCard.displayName = "ToolItemCard";

export const ToolGroupCollapse = React.memo(({
  tools,
  onImageClick,
}: {
  tools: ThreadItem[];
  onImageClick: (url: string) => void;
}) => {
  const { t } = useT();
  const seenArtifactTypes = new Set<string>();
  const mediaItems: ThreadItem[] = [];
  const textOnly: ThreadItem[] = [];
  for (const tl of tools) {
    const arts = tl.artifacts?.filter((a) => {
      if (seenArtifactTypes.has(a.type)) return false;
      seenArtifactTypes.add(a.type);
      return true;
    });
    if (arts && arts.length > 0) {
      mediaItems.push({ ...tl, artifacts: arts });
    } else {
      textOnly.push(tl);
    }
  }
  const hasMedia = mediaItems.length > 0;

  return (
    <div className="tool-group">
      {mediaItems.map((item) => (
        <ToolItemCard key={item.id} item={item} onImageClick={onImageClick} />
      ))}
      {textOnly.length > 0 && (
        <details className="tool-group-details">
          <summary className="tool-group-summary">
            <ChevronDown size={12} />
            {textOnly.length} {textOnly.length === 1 ? t("message.toolCall") : t("message.toolCalls")}
            {hasMedia ? "" : ` ${t("message.completed")}`}
          </summary>
          <div className="tool-group-items">
            {textOnly.map((item) => (
              <ToolItemCard key={item.id} item={item} onImageClick={onImageClick} />
            ))}
          </div>
        </details>
      )}
    </div>
  );
});

ToolGroupCollapse.displayName = "ToolGroupCollapse";

export interface ToolCardData {
  name: string;
  toolUseId: string;
  status: "running" | "output" | "done" | "error";
  outputs: string[];
  isError: boolean;
  media?: { type: string; url: string };
}

export const StreamingToolCard = React.memo(({
  card,
  onImageClick
}: {
  card: ToolCardData;
  onImageClick: (url: string) => void;
}) => {
  const { t } = useT();
  const joined = card.outputs.join("\n");
  const lineCount = joined ? joined.split("\n").length : 0;

  return (
    <motion.div
      initial={{ opacity: 0, height: 0 }}
      animate={{ opacity: 1, height: "auto" }}
      className={`tool-card ${card.isError ? "tool-error" : ""}`}
    >
      <div className="tool-card-header">
        <span className={`tool-status-icon status-${card.status}`} />
        <span className="tool-name">{card.name}</span>
        <span className="tool-status-label">
          {card.status === "running"
            ? t("message.running")
            : card.status === "error"
              ? t("message.failed")
              : card.status === "done"
                ? t("message.done")
                : ""}
        </span>
      </div>
      {card.outputs.length > 0 && (
        <details className="tool-output-details">
          <summary>{t("message.output")} ({lineCount} {lineCount === 1 ? t("message.line") : t("message.lines")})</summary>
          <pre className="tool-output">
            {joined.slice(0, 2000)}
            {joined.length > 2000 ? "\n..." : ""}
          </pre>
        </details>
      )}
      {card.media && (
        <MediaPreview type={card.media.type} url={card.media.url} onImageClick={onImageClick} />
      )}
    </motion.div>
  );
});

StreamingToolCard.displayName = "StreamingToolCard";

function parseToolContent(content: string): { name: string; output: string } {
  const match = content.match(/^\[([^\]]+)\]\s*([\s\S]*)$/);
  if (match) {
    return { name: match[1], output: match[2] };
  }
  return { name: "Tool", output: content };
}

function extractMediaFromText(text: string): { type: string; url: string } | null {
  if (!text) return null;
  const urlMatch = text.match(
    /https?:\/\/[^\s"']+\.(png|jpe?g|gif|webp|svg|mp4|webm|mp3|wav|ogg)(\?[^\s"']*)?/i
  );
  if (urlMatch) {
    const ext = urlMatch[1].toLowerCase();
    const t = /^(mp4|webm)$/.test(ext) ? "video" : /^(mp3|wav|ogg)$/.test(ext) ? "audio" : "image";
    return { type: t, url: urlMatch[0] };
  }
  const pathMatch = text.match(
    /\/[\w/._-]+\.(png|jpe?g|gif|webp|mp4|webm|mp3|wav|ogg)\b/i
  );
  if (pathMatch) {
    const ext = pathMatch[1].toLowerCase();
    const t = /^(mp4|webm)$/.test(ext) ? "video" : /^(mp3|wav|ogg)$/.test(ext) ? "audio" : "image";
    return { type: t, url: `/api/files${pathMatch[0]}` };
  }
  const relMatch = text.match(
    /(?:^|[\s"'=])([\w.][\w./_-]*\/[\w./_-]*\.(png|jpe?g|gif|webp|mp4|webm|mp3|wav|ogg))(?:\s|$|["'])/i
  );
  if (relMatch) {
    const ext = relMatch[2].toLowerCase();
    const t = /^(mp4|webm)$/.test(ext) ? "video" : /^(mp3|wav|ogg)$/.test(ext) ? "audio" : "image";
    return { type: t, url: `/api/files/${relMatch[1]}` };
  }
  return null;
}
