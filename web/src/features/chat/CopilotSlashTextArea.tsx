import React, { useRef, useState, useMemo, useEffect, useCallback } from "react";
import { SlashCommandPicker, HELP_SKILL } from "./SlashCommandPicker";
import { useChatStore } from "./useChatStore";
import type { SkillInfo } from "@/features/rpc/types";

const SLASH_PICKER_MAX = Infinity;
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
  } catch { /* ignore */ }
}

function detectSlashTrigger(value: string, caret: number): { start: number; query: string } | null {
  if (caret <= 0) return null;
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

const nativeTextAreaValueSetter = typeof window !== "undefined"
  ? Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, "value")?.set
  : undefined;

export const CopilotSlashTextArea = React.forwardRef<
  HTMLTextAreaElement,
  React.TextareaHTMLAttributes<HTMLTextAreaElement>
>((props, forwardedRef) => {
  const { onChange, onKeyDown, onSelect, onBlur, value, ...rest } = props;
  const skills = useChatStore((s) => s.skills);
  const innerRef = useRef<HTMLTextAreaElement>(null);

  const mergedRef = useCallback(
    (node: HTMLTextAreaElement | null) => {
      (innerRef as React.MutableRefObject<HTMLTextAreaElement | null>).current = node;
      if (typeof forwardedRef === "function") {
        forwardedRef(node);
      } else if (forwardedRef) {
        (forwardedRef as React.MutableRefObject<HTMLTextAreaElement | null>).current = node;
      }
    },
    [forwardedRef],
  );

  const [slashOpen, setSlashOpen] = useState(false);
  const [slashStart, setSlashStart] = useState(0);
  const [slashQuery, setSlashQuery] = useState("");
  const [slashIndex, setSlashIndex] = useState(0);
  const [recentSkillNames, setRecentSkillNames] = useState<string[]>(() => readRecentSkillNames());

  const filteredSkills = useMemo(() => {
    if (!slashOpen) return [];
    const list = skills ?? [];
    const q = slashQuery.toLowerCase();

    if (!q) {
      const seen = new Set<string>();
      const out: SkillInfo[] = [];
      out.push(HELP_SKILL);
      seen.add(HELP_SKILL.Name);
      for (const name of recentSkillNames) {
        const found = list.find((s) => s.Name === name);
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
    const helpName = HELP_SKILL.Name.toLowerCase();
    if (helpName.startsWith(q) || helpName.includes(q)) {
      matches.push({ skill: HELP_SKILL, rank: helpName.startsWith(q) ? 0 : 1 });
    }
    for (const s of list) {
      const name = (s.Name || "").toLowerCase();
      let rank = -1;
      if (name.startsWith(q)) rank = 0;
      else if (name.includes(q)) rank = 1;
      else if ((s.Keywords || []).some((k) => k.toLowerCase().includes(q))) rank = 2;
      else if ((s.Description || "").toLowerCase().includes(q)) rank = 3;
      if (rank >= 0) matches.push({ skill: s, rank });
    }
    matches.sort((a, b) => a.rank - b.rank || a.skill.Name.localeCompare(b.skill.Name));
    return matches.slice(0, SLASH_PICKER_MAX).map((m) => m.skill);
  }, [slashOpen, slashQuery, skills, recentSkillNames]);

  useEffect(() => {
    setSlashIndex(0);
  }, [slashQuery, slashOpen]);

  const closeSlash = useCallback(() => {
    setSlashOpen(false);
    setSlashQuery("");
    setSlashIndex(0);
  }, []);

  const updateSlashFromCaret = useCallback(
    (val: string, caret: number) => {
      if (!skills || skills.length === 0) {
        if (slashOpen) closeSlash();
        return;
      }
      const trig = detectSlashTrigger(val, caret);
      if (trig) {
        setSlashOpen(true);
        setSlashStart(trig.start);
        setSlashQuery(trig.query);
      } else if (slashOpen) {
        closeSlash();
      }
    },
    [skills, slashOpen, closeSlash],
  );

  const pushRecentSkill = useCallback((name: string) => {
    setRecentSkillNames((prev) => {
      const next = [name, ...prev.filter((n) => n !== name)].slice(0, RECENT_SKILLS_MAX);
      writeRecentSkillNames(next);
      return next;
    });
  }, []);

  const insertSlashSelection = useCallback(
    (skill: SkillInfo) => {
      if (skill.Name === HELP_SKILL.Name) {
        closeSlash();
        if (typeof window !== "undefined") {
          window.location.hash = "skills";
        }
        return;
      }

      pushRecentSkill(skill.Name);

      const ta = innerRef.current;
      if (!ta || !nativeTextAreaValueSetter) return;

      const cur = String(value ?? "");
      const before = cur.slice(0, slashStart);
      const afterPos = ta.selectionStart ?? slashStart + slashQuery.length + 1;
      const after = cur.slice(afterPos);
      const inserted = `/${skill.Name} `;
      const next = before + inserted + after;

      nativeTextAreaValueSetter.call(ta, next);
      ta.dispatchEvent(new Event("input", { bubbles: true }));

      closeSlash();
      requestAnimationFrame(() => {
        const pos = before.length + inserted.length;
        ta.focus();
        ta.setSelectionRange(pos, pos);
      });
    },
    [value, slashStart, slashQuery, closeSlash, pushRecentSkill],
  );

  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLTextAreaElement>) => {
      onChange?.(e);
      updateSlashFromCaret(e.target.value, e.target.selectionStart ?? e.target.value.length);
    },
    [onChange, updateSlashFromCaret],
  );

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (slashOpen && filteredSkills.length > 0) {
        if (e.key === "ArrowDown") {
          e.preventDefault();
          setSlashIndex((i) => (i + 1) % filteredSkills.length);
          return;
        }
        if (e.key === "ArrowUp") {
          e.preventDefault();
          setSlashIndex((i) => (i - 1 + filteredSkills.length) % filteredSkills.length);
          return;
        }
        if (e.key === "Enter" || e.key === "Tab") {
          e.preventDefault();
          e.stopPropagation();
          const pick = filteredSkills[slashIndex];
          if (pick) insertSlashSelection(pick);
          return;
        }
        if (e.key === "Escape") {
          e.preventDefault();
          closeSlash();
          return;
        }
      }
      onKeyDown?.(e);
    },
    [slashOpen, filteredSkills, slashIndex, insertSlashSelection, closeSlash, onKeyDown],
  );

  const handleSelect = useCallback(
    (e: React.SyntheticEvent<HTMLTextAreaElement>) => {
      onSelect?.(e);
      const ta = e.currentTarget;
      updateSlashFromCaret(ta.value, ta.selectionStart ?? ta.value.length);
    },
    [onSelect, updateSlashFromCaret],
  );

  const handleBlur = useCallback(
    (e: React.FocusEvent<HTMLTextAreaElement>) => {
      onBlur?.(e);
      window.setTimeout(() => closeSlash(), 120);
    },
    [onBlur, closeSlash],
  );

  return (
    <div style={{ position: "relative", width: "100%" }}>
      <textarea
        ref={mergedRef}
        {...rest}
        value={value}
        onChange={handleChange}
        onKeyDown={handleKeyDown}
        onSelect={handleSelect}
        onBlur={handleBlur}
      />
      <SlashCommandPicker
        filteredSkills={filteredSkills}
        slashIndex={slashIndex}
        onSelect={insertSlashSelection}
        onHover={setSlashIndex}
        recentSkillNames={recentSkillNames}
        slashQuery={slashQuery}
      />
    </div>
  );
});

CopilotSlashTextArea.displayName = "CopilotSlashTextArea";
