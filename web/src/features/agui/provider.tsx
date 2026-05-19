
import { useMemo, type ReactNode } from "react";
import { CopilotKit } from "@copilotkit/react-core/v2";
import { MediaPreview } from "@/features/chat/MessageItem";
import type { ThreadItemArtifact } from "@/features/rpc/types";

function resolveRuntimeUrl(): string {
  if (typeof window === "undefined") return "/v1/agents/run";
  const loc = window.location;
  if (loc.port === "10111") {
    return `${loc.protocol}//${loc.hostname}:17000/v1/agents/run`;
  }
  return "/v1/agents/run";
}

export interface SakerCopilotProviderProps {
  children: ReactNode;
}

const validArtifactTypes = new Set(["image", "video", "audio"]);

function isArtifact(value: unknown): value is ThreadItemArtifact {
  if (!value || typeof value !== "object") return false;
  const artifact = value as Record<string, unknown>;
  return (
    typeof artifact.type === "string" &&
    validArtifactTypes.has(artifact.type) &&
    typeof artifact.url === "string" &&
    artifact.url.length > 0
  );
}

function SakerArtifactMessage({
  stateSnapshot,
}: {
  position: "before" | "after";
  messageIndexInRun: number;
  numberOfMessagesInRun: number;
  stateSnapshot: unknown;
}) {

  const artifacts = (stateSnapshot as { artifacts?: unknown[] } | undefined)
    ?.artifacts
    ?.filter(isArtifact);
  if (!artifacts?.length) return null;

  return (
    <div className="saker-artifact-message">
      {artifacts.map((artifact, index) => (
        <MediaPreview
          key={`${artifact.url}-${index}`}
          type={artifact.type}
          url={artifact.url}
        />
      ))}
    </div>
  );
}

export function SakerCopilotProvider({ children }: SakerCopilotProviderProps) {
  const runtimeUrl = useMemo(() => resolveRuntimeUrl(), []);
  const renderCustomMessages = useMemo(
    () => [{ render: SakerArtifactMessage }],
    [],
  );

  return (
    <CopilotKit
      runtimeUrl={runtimeUrl}
      credentials="include"
      showDevConsole={true}
      useSingleEndpoint={true}
      renderCustomMessages={renderCustomMessages}
    >
      {children}
    </CopilotKit>
  );
}
