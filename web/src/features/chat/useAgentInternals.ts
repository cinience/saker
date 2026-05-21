import { useMemo } from "react";

interface AgentMessage {
  role: string;
  content: unknown;
}

interface AgentInternals {
  threadId: string | undefined;
  messages: AgentMessage[];
}

function extractTextFromContent(content: unknown): string {
  if (typeof content === "string") return content;
  if (Array.isArray(content)) {
    return (content as Array<Record<string, unknown>>)
      .filter((p) => p?.type === "text" && typeof p.text === "string")
      .map((p) => p.text as string)
      .join("\n");
  }
  return "";
}

export function getAgentInternals(agent: unknown): AgentInternals {
  const agentAny = agent as Record<string, unknown> | null | undefined;
  return {
    threadId: (agentAny?.threadId as string) || undefined,
    messages: (agentAny?.messages as AgentMessage[]) || [],
  };
}

export function getLatestUserText(messages: AgentMessage[], fromEnd = false): string {
  const userMsg = fromEnd
    ? [...messages].reverse().find((m) => m.role === "user")
    : messages.find((m) => m.role === "user");
  if (!userMsg) return "";
  return extractTextFromContent(userMsg.content);
}

export function useAgentInternals(agent: unknown): AgentInternals {
  return useMemo(() => getAgentInternals(agent), [agent]);
}
