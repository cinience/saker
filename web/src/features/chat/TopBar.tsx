
import { useEffect, useRef, useState } from "react";
import type { KeyboardEvent as ReactKeyboardEvent, ReactNode } from "react";
import {
  ChevronDown,
  FolderOpen,
  LogOut,
  User as UserIcon,
  X as XIcon,
} from "lucide-react";

import { useT } from "@/features/i18n";
import { useProjectStore } from "@/features/project/projectStore";
import { ProjectSwitcher } from "@/features/project/ProjectSwitcher";
import { CreateProjectDialog } from "@/features/project/CreateProjectDialog";
import { ProjectSettingsPage } from "@/features/project/ProjectSettingsPage";
import { InviteInbox } from "@/features/project/InviteInbox";
import { ThemeSwatchPicker } from "./ThemeSwatchPicker";

interface Props {
  username: string;
  role: string; // global role: admin | user
  onLogout?: () => Promise<void> | void;
  // Optional slot rendered after the brand in topbar-left.
  // ChatApp uses it to host the thread-panel toggle on chat / canvas views.
  leftSlot?: ReactNode;
}

const FOCUSABLE_MENU_SELECTOR =
  'button:not([disabled]), [role="menuitem"]:not([aria-disabled="true"]), [role="option"]:not([aria-disabled="true"])';

/**
 * TopBar is the persistent header. The brand sits on the left and a single
 * user-menu lives on the right; the user dropdown owns project switching
 * (active project label, membership list, "new project", "project
 * settings") so the entire identity surface is colocated.
 */
export function TopBar({ username, role, onLogout, leftSlot }: Props) {
  const { t } = useT();
  const [menuOpen, setMenuOpen] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);
  const userMenuRef = useRef<HTMLDivElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);

  const projects = useProjectStore((s) => s.projects);
  const currentId = useProjectStore((s) => s.currentProjectId);
  const currentProject = projects.find((p) => p.id === currentId) ?? null;

  useEffect(() => {
    if (!menuOpen) return;
    const onDoc = (e: MouseEvent) => {
      if (!menuRef.current?.contains(e.target as Node)) setMenuOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [menuOpen]);

  // When the menu opens via keyboard (ArrowDown/Enter/Space on trigger),
  // shift focus to the first focusable item so subsequent ArrowUp/Down
  // navigation works without an extra Tab.
  useEffect(() => {
    if (!menuOpen) return;
    const first = userMenuRef.current?.querySelector<HTMLElement>(
      FOCUSABLE_MENU_SELECTOR,
    );
    first?.focus();
  }, [menuOpen]);

  const handleMenuKeyDown = (e: ReactKeyboardEvent<HTMLDivElement>) => {
    if (!userMenuRef.current) return;
    const items = Array.from(
      userMenuRef.current.querySelectorAll<HTMLElement>(FOCUSABLE_MENU_SELECTOR),
    );
    if (items.length === 0) return;
    const idx = items.indexOf(document.activeElement as HTMLElement);
    switch (e.key) {
      case "ArrowDown":
        e.preventDefault();
        items[(idx + 1 + items.length) % items.length]?.focus();
        break;
      case "ArrowUp":
        e.preventDefault();
        items[(idx - 1 + items.length) % items.length]?.focus();
        break;
      case "Home":
        e.preventDefault();
        items[0]?.focus();
        break;
      case "End":
        e.preventDefault();
        items[items.length - 1]?.focus();
        break;
      case "Escape":
        e.preventDefault();
        setMenuOpen(false);
        triggerRef.current?.focus();
        break;
      case "Tab":
        // Closing on Tab matches WAI-ARIA menu guidance and prevents a
        // user from getting trapped inside the popover.
        setMenuOpen(false);
        break;
    }
  };

  const handleTriggerKeyDown = (
    e: ReactKeyboardEvent<HTMLButtonElement>,
  ) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      setMenuOpen(true);
    } else if (e.key === "Escape" && menuOpen) {
      e.preventDefault();
      setMenuOpen(false);
    }
  };

  return (
    <header className="topbar">
      <div className="topbar-left">
        <span className="topbar-brand" title="Saker">
          <svg viewBox="0 0 128 128" width={22} height={22} aria-hidden="true">
            <rect x="30" y="4" width="68" height="28" rx="5" fill="currentColor"/>
            <rect x="18" y="16" width="16" height="16" rx="4" fill="currentColor" opacity="0.5"/>
            <rect x="94" y="16" width="16" height="16" rx="4" fill="currentColor" opacity="0.5"/>
            <rect x="10" y="38" width="12" height="40" rx="3" fill="currentColor" opacity="0.6"/>
            <rect x="106" y="38" width="12" height="40" rx="3" fill="currentColor" opacity="0.6"/>
            <polygon points="38,38 56,56 38,74 20,56" fill="currentColor" opacity="0.85"/>
            <circle cx="38" cy="56" r="6" fill="var(--bg)"/>
            <circle cx="38" cy="56" r="2.5" fill="currentColor"/>
            <polygon points="90,38 108,56 90,74 72,56" fill="currentColor" opacity="0.85"/>
            <circle cx="90" cy="56" r="6" fill="var(--bg)"/>
            <circle cx="90" cy="56" r="2.5" fill="currentColor"/>
            <polygon points="64,42 76,66 52,66" fill="currentColor"/>
            <rect x="42" y="88" width="44" height="22" rx="5" fill="currentColor"/>
            <rect x="30" y="88" width="16" height="14" rx="4" fill="currentColor" opacity="0.6"/>
            <rect x="82" y="88" width="16" height="14" rx="4" fill="currentColor" opacity="0.6"/>
          </svg>
          <span className="topbar-brand-text accent-grad-text">Saker</span>
        </span>
        {leftSlot}
      </div>
      <div className="topbar-right" ref={menuRef}>
        {username && <InviteInbox />}
        {username ? (
          <>
            <button
              ref={triggerRef}
              className="topbar-user-btn"
              onClick={() => setMenuOpen((v) => !v)}
              onKeyDown={handleTriggerKeyDown}
              aria-haspopup="menu"
              aria-expanded={menuOpen}
            >
              <div className="topbar-avatar" aria-hidden="true">
                {username.slice(0, 1).toUpperCase()}
              </div>
              <span className="topbar-username">{username}</span>
              <ChevronDown size={14} strokeWidth={1.75} />
            </button>
            {menuOpen && (
              <div
                ref={userMenuRef}
                className="topbar-user-menu"
                role="menu"
                onKeyDown={handleMenuKeyDown}
              >
                <div className="topbar-user-menu-header">
                  <UserIcon size={14} strokeWidth={1.75} />
                  <div className="topbar-user-menu-header-text">
                    <div className="topbar-user-menu-header-row">
                      <span className="topbar-user-menu-header-name">{username}</span>
                      {role && (
                        <span className={`role-badge role-${role}`}>{role}</span>
                      )}
                    </div>
                    {currentProject && (
                      <span
                        className="topbar-current-project"
                        aria-label={t("project.title")}
                      >
                        <FolderOpen size={12} strokeWidth={1.75} />
                        <span>{currentProject.name}</span>
                      </span>
                    )}
                  </div>
                </div>
                <div className="topbar-user-menu-section-label">
                  {t("settings.theme")}
                </div>
                <div className="topbar-user-menu-theme-row">
                  <ThemeSwatchPicker />
                </div>
                <div className="project-switcher-sep" />
                <div className="topbar-user-menu-section-label">
                  {t("project.title")}
                </div>
                <ProjectSwitcher
                  onAction={() => setMenuOpen(false)}
                  onCreate={() => setCreateOpen(true)}
                  onOpenSettings={() => setSettingsOpen(true)}
                />
                {onLogout && (
                  <>
                    <div className="project-switcher-sep" />
                    <button
                      role="menuitem"
                      className="topbar-user-menu-item"
                      onClick={() => {
                        setMenuOpen(false);
                        onLogout();
                      }}
                    >
                      <LogOut size={14} strokeWidth={1.75} />
                      <span>{t("auth.logout")}</span>
                    </button>
                  </>
                )}
              </div>
            )}
          </>
        ) : (
          <span className="topbar-username topbar-username-anon">
            {t("auth.notSignedIn")}
          </span>
        )}
      </div>
      <CreateProjectDialog
        open={createOpen}
        onClose={() => setCreateOpen(false)}
      />
      {settingsOpen && (
        <div
          className="modal-backdrop"
          onClick={(e) => {
            if (e.target === e.currentTarget) setSettingsOpen(false);
          }}
        >
          <div
            className="modal-card project-settings-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="project-settings-modal-title"
          >
            <button
              className="modal-close"
              onClick={() => setSettingsOpen(false)}
              aria-label={t("project.cancel")}
            >
              <XIcon size={16} strokeWidth={1.75} />
            </button>
            <h2 id="project-settings-modal-title" className="sr-only">
              {t("project.settings")}
            </h2>
            <ProjectSettingsPage />
          </div>
        </div>
      )}
    </header>
  );
}
