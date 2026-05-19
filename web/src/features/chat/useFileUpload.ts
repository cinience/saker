import { useState, useCallback, useRef } from "react";
import { toast } from "sonner";
import { useT } from "@/features/i18n";

export const MAX_FILE_SIZE = 50 * 1024 * 1024; // 50 MB

export interface Attachment {
  id: string;
  file: File;
  name: string;
  media_type: string;
  preview?: string;    // object URL for image thumbnails
  uploading: boolean;
  progress: number;    // 0-100
  path?: string;       // server path after upload
  error?: string;
}

export function useFileUpload() {
  const { t } = useT();
  const [attachments, setAttachments] = useState<Attachment[]>([]);
  const nextAttachIdRef = useRef(0);

  const resolveUploadUrl = (): string => {
    if (typeof window === "undefined") return "http://127.0.0.1:10112/api/upload";
    const { protocol, hostname, port } = window.location;
    if (port === "10111") return `${protocol}//${hostname}:10112/api/upload`;
    return `${protocol}//${window.location.host}/api/upload`;
  };

  const uploadFileWithProgress = (
    file: File,
    onProgress: (pct: number) => void,
  ): Promise<Partial<Attachment>> => {
    return new Promise((resolve) => {
      const xhr = new XMLHttpRequest();
      xhr.open("POST", resolveUploadUrl());

      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) {
          onProgress(Math.round((e.loaded / e.total) * 100));
        }
      };

      xhr.onload = () => {
        if (xhr.status >= 200 && xhr.status < 300) {
          try {
            const data = JSON.parse(xhr.responseText);
            resolve({ path: data.path, media_type: data.media_type, uploading: false, progress: 100 });
          } catch {
            resolve({ error: "Invalid server response", uploading: false, progress: 0 });
          }
        } else {
          resolve({ error: xhr.responseText || `Upload failed (${xhr.status})`, uploading: false, progress: 0 });
        }
      };

      xhr.onerror = () => {
        resolve({ error: "Network error", uploading: false, progress: 0 });
      };

      const formData = new FormData();
      formData.append("file", file);
      xhr.send(formData);
    });
  };

  const handleFileUpload = useCallback(async (files: FileList | File[]) => {
    const newAttachments: Attachment[] = [];
    for (const file of Array.from(files)) {
      // Size pre-check
      if (file.size > MAX_FILE_SIZE) {
        toast.error(`${file.name}: ${t("composer.fileTooLarge")}`);
        continue;
      }

      const id = `att-${++nextAttachIdRef.current}`;
      const att: Attachment = {
        id,
        file,
        name: file.name,
        media_type: file.type || "application/octet-stream",
        uploading: true,
        progress: 0,
      };
      // Generate preview for images
      if (file.type.startsWith("image/")) {
        att.preview = URL.createObjectURL(file);
      }
      newAttachments.push(att);
    }
    
    if (newAttachments.length === 0) return;
    setAttachments(prev => [...prev, ...newAttachments]);

    // Upload each file with progress
    for (const att of newAttachments) {
      uploadFileWithProgress(att.file, (pct) => {
        setAttachments(prev =>
          prev.map(a => a.id === att.id ? { ...a, progress: pct } : a)
        );
      }).then(result => {
        if (result.error) {
          toast.error(`${att.name}: ${t("composer.uploadFailed")}`);
        }
        setAttachments(prev =>
          prev.map(a => a.id === att.id ? { ...a, ...result } : a)
        );
      });
    }
  }, [t]);

  const removeAttachment = useCallback((id: string) => {
    setAttachments((prev) => {
      const att = prev.find((p) => p.id === id);
      if (att?.preview) {
        URL.revokeObjectURL(att.preview);
      }
      return prev.filter((p) => p.id !== id);
    });
  }, []);

  const clearAttachments = useCallback(() => {
    setAttachments((prev) => {
      prev.forEach((att) => {
        if (att.preview) {
          URL.revokeObjectURL(att.preview);
        }
      });
      return [];
    });
  }, []);

  return {
    attachments,
    handleFileUpload,
    removeAttachment,
    clearAttachments,
  };
}
