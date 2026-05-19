
import { useState, useRef, useCallback, useMemo, useEffect } from "react";
import { useT } from "@/features/i18n";
import { usePermissions } from "@/features/project/usePermissions";
import { Send, Square, Plus } from "lucide-react";
import type { SkillInfo } from "@/features/rpc/types";
import { useFileUpload, type Attachment } from "./useFileUpload";
import { AttachmentList } from "./AttachmentList";
import { SlashCommandPicker, HELP_SKILL } from "./SlashCommandPicker";

const SLASH_PICKER_MAX = 8;
const RECENT_SKILLS_KEY = "saker.composer.recentSkills";
const RECENT_SKILLS_MAX = 3;

function readRecentSkillNames(): string[] {
  if (typeof window === "undefined") return [];
  try {
    const raw = window.localStorage.getItem(RECENT_SKILLS_KEY);
    if (!raw) return [];
    const arr = JSON.parse(raw);
    return Array.isArray(arr) ? arr.filter((x): x is string => typeof x === "string").slice(0, RECENT_SKILLS_MAX) : [];
  } catch {
    return [];
  }
}

function writeRecentSkillNames(names: string[]) {
  if (typeof window === "undefined") return;
  try {
    window.localStorage.setItem(RECENT_SKILLS_KEY, JSON.stringify(names.slice(0, RECENT_SKILLS_MAX)));
  } catch {
    /* ignore storage errors (private mode, quota, etc.) */
  }
}

interface Props {
  onSend: (text: string, attachments?: Attachment[]) => void;
  onStop?: () => void;
  disabled: boolean;
  running?: boolean;
  skills?: SkillInfo[];
}

/**
 * Detect a slash trigger ending at `caret`.
 * Returns { start, query } when text has "/" at line start or after whitespace,
 * followed by 0+ non-space chars up to caret. Returns null otherwise.
 */
function detectSlashTrigger(value: string, caret: number): { start: number; query: string } | null {
  if (caret <= 0) return null;
  // Walk back from caret: stop at whitespace; if we hit "/", check the char before it.
  let i = caret - 1;
  while (i >= 0) {
    const ch = value[i];
    if (ch === " " || ch === "\n" || ch === "\t") return null;
    if (ch === "/") {
      const prev = i === 0 ? "" : value[i - 1];
      if (i === 0 || prev === " " || prev === "\n" || prev === "\t") {
        return { start: i, query: value.slice(i + 1, caret) };
      }
      return null;
    }
    i--;
  }
  return null;
}

export function Composer({ onSend, onStop, disabled, running, skills }: Props) {
  const { t } = useT();
  const perms = usePermissions();
  const readOnly = !perms.canEdit;
  const inputDisabled = disabled || readOnly;
  const [text, setText] = useState("");
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const {
    attachments,
    handleFileUpload,
    removeAttachment,
    clearAttachments,
  } = useFileUpload();

  // Slash picker state
  const [slashOpen, setSlashOpen] = useState(false);
  const [slashStart, setSlashStart] = useState(0);
  const [slashQuery, setSlashQuery] = useState("");
  const [slashIndex, setSlashIndex] = useState(0);
  const [recentSkillNames, setRecentSkillNames] = useState<string[]>(() => readRecentSkillNames());

  // Items shown in the slash picker. When the query is empty we surface the
  // /help shortcut and recently-used skills first; once the user types we
  // fall back to plain ranked filtering against the full skill list.
  const filteredSkills = useMemo(() => {
    if (!slashOpen) return [];
    const list = skills ?? [];
    const q = slashQuery.toLowerCase();

    if (!q) {
      const seen = new Set<string>();
      const out: SkillInfo[] = [];
      // /help always pinned at top of empty query
      out.push(HELP_SKILL);
      seen.add(HELP_SKILL.Name);
      for (const name of recentSkillNames) {
        const found = list.find(s => s.Name === name);
        if (found && !seen.has(found.Name)) {
          out.push(found);
          seen.add(found.Name);
        }
      }
      for (const s of list) {
        if (out.length >= SLASH_PICKER_MAX) break;
        if (!seen.has(s.Name)) {
          out.push(s);
          seen.add(s.Name);
        }
      }
      return out.slice(0, SLASH_PICKER_MAX);
    }

    const matches: { skill: SkillInfo; rank: number }[] = [];
    // Make /help findable by typing "/h" or "/help"
    const helpName = HELP_SKILL.Name.toLowerCase();
    if (helpName.startsWith(q) || helpName.includes(q)) {
      matches.push({ skill: HELP_SKILL, rank: helpName.startsWith(q) ? 0 : 1 });
    }
    for (const s of list) {
      const name = (s.Name || "").toLowerCase();
      let rank = -1;
      if (name.startsWith(q)) rank = 0;
      else if (name.includes(q)) rank = 1;
      else if ((s.Keywords || []).some(k => k.toLowerCase().includes(q))) rank = 2;
      else if ((s.Description || "").toLowerCase().includes(q)) rank = 3;
      if (rank >= 0) matches.push({ skill: s, rank });
    }
    matches.sort((a, b) => a.rank - b.rank || a.skill.Name.localeCompare(b.skill.Name));
    return matches.slice(0, SLASH_PICKER_MAX).map(m => m.skill);
  }, [slashOpen, slashQuery, skills, recentSkillNames]);

  // Reset highlighted item when filtered list changes
  useEffect(() => {
    setSlashIndex(0);
  }, [slashQuery, slashOpen]);

  const closeSlash = useCallback(() => {
    setSlashOpen(false);
    setSlashQuery("");
    setSlashIndex(0);
  }, []);

  const insertSlashSelection = useCallback((skill: SkillInfo) => {
    const ta = textareaRef.current;
    if (!ta) return;
    const before = text.slice(0, slashStart);
    const after = text.slice(ta.selectionStart);
    const inserted = `/${skill.Name} `;
    const next = before + inserted + after;
    setText(next);
    closeSlash();
    requestAnimationFrame(() => {
      const t2 = textareaRef.current;
      if (!t2) return;
      const pos = before.length + inserted.length;
      t2.focus();
      t2.setSelectionRange(pos, pos);
      t2.style.height = "auto";
      t2.style.height = Math.min(t2.scrollHeight, 200) + "px";
    });
  }, [text, slashStart, closeSlash]);

  const pushRecentSkill = useCallback((name: string) => {
    setRecentSkillNames(prev => {
      const next = [name, ...prev.filter(n => n !== name)].slice(0, RECENT_SKILLS_MAX);
      writeRecentSkillNames(next);
      return next;
    });
  }, []);

  /**
   * Routes a slash-picker selection. The synthetic /help item is identified by
   * Name comparison so a real "help" skill, if registered, still
   * inserts as text. /help instead navigates to the skills catalog view.
   */
  const handleSelectSkill = useCallback((skill: SkillInfo) => {
    if (skill.Name === HELP_SKILL.Name) {
      closeSlash();
      if (typeof window !== "undefined") {
        window.location.hash = "skills";
      }
      return;
    }
    pushRecentSkill(skill.Name);
    insertSlashSelection(skill);
  }, [closeSlash, pushRecentSkill, insertSlashSelection]);

  const updateSlashFromCaret = useCallback((value: string, caret: number) => {
    if (!skills || skills.length === 0) {
      if (slashOpen) closeSlash();
      return;
    }
    const trig = detectSlashTrigger(value, caret);
    if (trig) {
      setSlashOpen(true);
      setSlashStart(trig.start);
      setSlashQuery(trig.query);
    } else if (slashOpen) {
      closeSlash();
    }
  }, [skills, slashOpen, closeSlash]);

  const handleSend = useCallback(() => {
    const trimmed = text.trim();
    const readyAttachments = attachments.filter(a => a.path && !a.uploading && !a.error);
    if ((!trimmed && readyAttachments.length === 0) || inputDisabled) return;
    onSend(trimmed || " ", readyAttachments.length > 0 ? readyAttachments : undefined);
    setText("");
    clearAttachments();
    closeSlash();
    if (textareaRef.current) {
      textareaRef.current.style.height = "auto";
    }
  }, [text, attachments, inputDisabled, onSend, clearAttachments, closeSlash]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      // Slash picker keyboard handling takes priority
      if (slashOpen && filteredSkills.length > 0) {
        if (e.key === "ArrowDown") {
          e.preventDefault();
          setSlashIndex(i => (i + 1) % filteredSkills.length);
          return;
        }
        if (e.key === "ArrowUp") {
          e.preventDefault();
          setSlashIndex(i => (i - 1 + filteredSkills.length) % filteredSkills.length);
          return;
        }
        if (e.key === "Enter" || e.key === "Tab") {
          e.preventDefault();
          const pick = filteredSkills[slashIndex];
          if (pick) handleSelectSkill(pick);
          return;
        }
        if (e.key === "Escape") {
          e.preventDefault();
          closeSlash();
          return;
        }
      }
      if (e.key === "Enter" && !e.shiftKey) {
        e.preventDefault();
        handleSend();
      }
    },
    [slashOpen, filteredSkills, slashIndex, handleSelectSkill, closeSlash, handleSend]
  );

  const handleInput = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      const value = e.target.value;
      setText(value);
      const ta = e.target;
      ta.style.height = "auto";
      ta.style.height = Math.min(ta.scrollHeight, 200) + "px";
      updateSlashFromCaret(value, ta.selectionStart ?? value.length);
    },
    [updateSlashFromCaret]
  );

  const handleSelect = useCallback(
    (e: React.SyntheticEvent<HTMLTextAreaElement>) => {
      const ta = e.currentTarget;
      updateSlashFromCaret(ta.value, ta.selectionStart ?? ta.value.length);
    },
    [updateSlashFromCaret]
  );

  const handleBlur = useCallback(() => {
    // Delay so click on picker item still registers
    window.setTimeout(() => closeSlash(), 120);
  }, [closeSlash]);

  const handleFileChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    if (e.target.files && e.target.files.length > 0) {
      handleFileUpload(e.target.files);
      e.target.value = "";
    }
  }, [handleFileUpload]);

  const handleDrop = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.dataTransfer.files.length > 0) {
      handleFileUpload(e.dataTransfer.files);
    }
  }, [handleFileUpload]);

  const handleDragOver = useCallback((e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
  }, []);

  // Ctrl+V / Cmd+V paste image from clipboard
  const handlePaste = useCallback((e: React.ClipboardEvent) => {
    const items = e.clipboardData?.items;
    if (!items) return;
    const imageFiles: File[] = [];
    for (const item of Array.from(items)) {
      if (item.type.startsWith("image/")) {
        const file = item.getAsFile();
        if (file) {
          // Give pasted images a descriptive name
          const ext = file.type.split("/")[1] || "png";
          const named = new File([file], `pasted-image-${Date.now()}.${ext}`, { type: file.type });
          imageFiles.push(named);
        }
      }
    }
    if (imageFiles.length > 0) {
      e.preventDefault();
      handleFileUpload(imageFiles);
    }
  }, [handleFileUpload]);

  const anyUploading = attachments.some(a => a.uploading);

  return (
    <div className="gemini-composer-container">
      <div
        className="gemini-composer-pill"
        onDrop={handleDrop}
        onDragOver={handleDragOver}
      >
        <AttachmentList
          attachments={attachments}
          onRemove={removeAttachment}
        />

        <div className="gemini-input-row">
          {/* "+" button */}
          {!readOnly && (
            <button
              className="gemini-attach-btn"
              onClick={() => fileInputRef.current?.click()}
              disabled={inputDisabled}
              aria-label={t("composer.addFiles")}
            >
              <Plus size={20} />
            </button>
          )}
          <input
            ref={fileInputRef}
            type="file"
            multiple
            accept="image/*,video/*,audio/*,.pdf"
            onChange={handleFileChange}
            style={{ display: "none" }}
          />

          <textarea
            ref={textareaRef}
            className="gemini-textarea"
            value={text}
            onChange={handleInput}
            onSelect={handleSelect}
            onKeyDown={handleKeyDown}
            onPaste={handlePaste}
            onBlur={handleBlur}
            placeholder={readOnly ? t("composer.viewerReadOnly") : t("composer.askSaker")}
            aria-label={t("composer.send")}
            disabled={inputDisabled}
            rows={1}
          />

          <div className="gemini-right-actions">
            <div className="send-btn-wrapper">
              {running ? (
                <button className="gemini-stop-btn" onClick={onStop} aria-label={t("composer.stop")}>
                  <Square size={16} fill="currentColor" strokeWidth={0} />
                </button>
              ) : !readOnly ? (
                <button
                  className="gemini-send-btn"
                  onClick={handleSend}
                  disabled={inputDisabled || anyUploading}
                  aria-label={t("composer.send")}
                >
                  <Send size={18} />
                </button>
              ) : null}
            </div>
          </div>
        </div>

        <SlashCommandPicker
                  filteredSkills={filteredSkills}
                  slashIndex={slashIndex}
                  onSelect={insertSlashSelection}
                  onHover={setSlashIndex}
                  recentSkillNames={recentSkillNames}
                  slashQuery={slashQuery}
                />
      </div>
      <div className="gemini-disclaimer">
        {t("composer.disclaimer")}
      </div>
    </div>
  );
}
