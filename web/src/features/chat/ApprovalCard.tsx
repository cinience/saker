import { useState } from "react";
import type { ApprovalRequest } from "@/features/rpc/types";
import DOMPurify from "dompurify";
import { useT } from "@/features/i18n";

interface Props {
  approval: ApprovalRequest;
  onRespond: (id: string, decision: "allow" | "deny") => void | Promise<void>;
}

function highlightJson(json: string): string {
  return json.replace(
    /("(?:\\.|[^"\\])*")\s*:/g,
    '<span class="json-key">$1</span>:'
  ).replace(
    /:\s*("(?:\\.|[^"\\])*")/g,
    ': <span class="json-string">$1</span>'
  ).replace(
    /:\s*(\d+(?:\.\d+)?)/g,
    ': <span class="json-number">$1</span>'
  ).replace(
    /:\s*(true|false)/g,
    ': <span class="json-bool">$1</span>'
  ).replace(
    /:\s*(null)/g,
    ': <span class="json-null">$1</span>'
  );
}

export function ApprovalCard({ approval, onRespond }: Props) {
  const { t } = useT();
  const [submitting, setSubmitting] = useState<"allow" | "deny" | null>(null);
  const [submitted, setSubmitted] = useState<"allow" | "deny" | null>(null);

  const respond = async (decision: "allow" | "deny") => {
    if (submitting || submitted) return;
    setSubmitting(decision);
    try {
      await onRespond(approval.id, decision);
      setSubmitted(decision);
    } catch (e) {
      console.error("approval response error:", e);
      setSubmitting(null);
    }
  };

  if (submitted) {
    return (
      <div className="approval-card approval-card--submitted">
        <div className="approval-result">
          <span className="approval-result-tool">{approval.tool_name}</span>
          <span className={`approval-result-decision approval-result-decision--${submitted}`}>
            {submitted === "allow" ? t("approval.allowed") : t("approval.denied")}
          </span>
        </div>
      </div>
    );
  }

  return (
    <div className="approval-card">
      <div className="tool-info">
        <strong>{approval.tool_name}</strong> {t("approval.requiresApproval")}
      </div>
      {approval.reason && (
        <div className="approval-reason">{approval.reason}</div>
      )}
      {approval.tool_params &&
        Object.keys(approval.tool_params).length > 0 && (
          <div
            className="approval-params"
            dangerouslySetInnerHTML={{
              __html: DOMPurify.sanitize(highlightJson(
                JSON.stringify(approval.tool_params, null, 2)
                  .replace(/&/g, "&amp;")
                  .replace(/</g, "&lt;")
                  .replace(/>/g, "&gt;")
              ), { ALLOWED_TAGS: ["span"] }),
            }}
          />
        )}
      {submitted ? (
        <div className="card-response-status">{t("approval.submitted")}</div>
      ) : (
        <div className="actions">
          <button
            className="btn-allow"
            onClick={() => respond("allow")}
            disabled={!!submitting}
          >
            {submitting === "allow" ? t("common.submitting") : t("approval.allow")}
          </button>
          <button
            className="btn-deny"
            onClick={() => respond("deny")}
            disabled={!!submitting}
          >
            {submitting === "deny" ? t("common.submitting") : t("approval.deny")}
          </button>
        </div>
      )}
    </div>
  );
}
