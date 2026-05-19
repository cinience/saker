import { HelpCircle, Clock } from "lucide-react";
import type { SkillInfo } from "@/features/rpc/types";
import { useT } from "@/features/i18n";

export const HELP_SKILL: SkillInfo = {
  Name: "help",
  Description: "Open the skills catalog",
  Scope: "user",
  RelatedSkills: [],
  Keywords: ["help", "skills", "list", "catalog"],
};

interface SlashCommandPickerProps {
  filteredSkills: SkillInfo[];
  slashIndex: number;
  onSelect: (skill: SkillInfo) => void;
  onHover: (index: number) => void;
  recentSkillNames: string[];
  slashQuery: string;
}

export function SlashCommandPicker({
  filteredSkills,
  slashIndex,
  onSelect,
  onHover,
  recentSkillNames,
  slashQuery,
}: SlashCommandPickerProps) {
  const { t } = useT();

  if (filteredSkills.length === 0) return null;

  return (
    <div className="slash-picker" role="listbox" aria-label={t("composer.slashHint")}>
      <div className="slash-picker__header">{t("composer.slashHint")}</div>
      <ul className="slash-picker__list">
        {filteredSkills.map((s, i) => {
          const isHelp = s.Name === HELP_SKILL.Name; // Use Name for comparison if reference might change
          const isRecent = !isHelp && !slashQuery && recentSkillNames.includes(s.Name);
          return (
            <li
              key={s.Name}
              className={`slash-item${i === slashIndex ? " slash-item--active" : ""}`}
              role="option"
              aria-selected={i === slashIndex}
              onMouseDown={(e) => { e.preventDefault(); onSelect(s); }}
              onMouseEnter={() => onHover(i)}
            >
              <span className="slash-item__name">
                {isHelp ? <HelpCircle size={12} aria-hidden="true" /> :
                  isRecent ? <Clock size={12} aria-hidden="true" /> : null}
                /{s.Name}
              </span>
              {s.Description && (
                <span className="slash-item__desc">{s.Description}</span>
              )}
              {isHelp ? (
                <span className="slash-item__scope slash-item__scope--help">help</span>
              ) : s.Scope ? (
                <span className={`slash-item__scope slash-item__scope--${s.Scope}`}>{s.Scope}</span>
              ) : null}
            </li>
          );
        })}
      </ul>
    </div>
  );
}
