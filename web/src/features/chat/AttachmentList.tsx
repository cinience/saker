import { X, FileText, Film, Volume2 } from "lucide-react";
import type { Attachment } from "./useFileUpload";
import { useT } from "@/features/i18n";

interface AttachmentListProps {
  attachments: Attachment[];
  onRemove: (id: string) => void;
}

export function AttachmentList({ attachments, onRemove }: AttachmentListProps) {
  const { t } = useT();

  if (attachments.length === 0) return null;

  const renderAttachmentIcon = (att: Attachment) => {
    if (att.media_type.startsWith("video/")) return <Film size={16} />;
    if (att.media_type.startsWith("audio/")) return <Volume2 size={16} />;
    return <FileText size={16} />;
  };

  return (
    <div className="composer-attachments">
      {attachments.map(att => (
        <div key={att.id} className={`attachment-chip ${att.error ? "attachment-error" : ""} ${att.uploading ? "attachment-uploading" : ""}`}>
          {att.preview ? (
            <img src={att.preview} alt={att.name} className="attachment-thumb" />
          ) : (
            <span className="attachment-icon">{renderAttachmentIcon(att)}</span>
          )}
          <span className="attachment-name">{att.name}</span>
          {att.uploading && (
            <span className="attachment-progress">{att.progress}%</span>
          )}
          <button
            className="attachment-remove"
            onClick={() => onRemove(att.id)}
            aria-label={t("composer.removeFile")}
          >
            <X size={14} />
          </button>
          {/* Upload progress bar */}
          {att.uploading && (
            <div className="attachment-progress-bar">
              <div className="attachment-progress-fill" style={{ width: `${att.progress}%` }} />
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
