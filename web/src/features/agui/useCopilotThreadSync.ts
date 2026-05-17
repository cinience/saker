"use client";

import { useEffect, useLayoutEffect, useRef } from "react";
import { useAgent, UseAgentUpdate } from "@copilotkit/react-core/v2";

const RUNTIME_URL =
  typeof window !== "undefined"
    ? window.location.port === "10111"
      ? `${window.location.protocol}//${window.location.hostname}:10112/v1/agents/run`
      : "/v1/agents/run"
    : "/v1/agents/run";

interface SnapshotMessage {
  id: string;
  role: "user" | "assistant" | "system" | "tool";
  content: string;
  toolCalls?: Array<{
    id: string;
    type: string;
    function: { name: string; arguments: string };
  }>;
  toolCallId?: string;
}

export function useCopilotThreadSync(threadId: string) {
  const { agent } = useAgent({
    updates: [UseAgentUpdate.OnMessagesChanged, UseAgentUpdate.OnRunStatusChanged],
  });
  const prevThreadIdRef = useRef<string | null>(null);
  const prevAgentRef = useRef<typeof agent | null>(null);
  const loadSeqRef = useRef(0);

  useLayoutEffect(() => {
    if (!agent) return;

    if (threadId) {
      agent.threadId = threadId;
    }

    if (threadId !== prevThreadIdRef.current && prevThreadIdRef.current !== null) {
      agent.setMessages([]);
    }
  }, [threadId, agent]);

  useEffect(() => {
    if (!agent) return;
    if (!threadId) {
      prevThreadIdRef.current = threadId;
      prevAgentRef.current = agent;
      return;
    }

    const sameThread = threadId === prevThreadIdRef.current;
    const sameAgent = agent === prevAgentRef.current;
    if (sameThread && sameAgent) return;

    prevThreadIdRef.current = threadId;
    prevAgentRef.current = agent;

    const loadSeq = ++loadSeqRef.current;

    fetchMessagesViaConnect(threadId)
      .then((msgs) => {
        if (
          loadSeqRef.current === loadSeq &&
          prevThreadIdRef.current === threadId &&
          !agent.isRunning
        ) {
          agent.setMessages(msgs as any);
        }
      })
      .catch((err) => {
        console.warn("copilot thread sync failed:", err);
      });
  }, [threadId, agent]);
}

async function fetchMessagesViaConnect(
  threadId: string,
): Promise<SnapshotMessage[]> {
  const resp = await fetch(RUNTIME_URL, {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
    },
    body: JSON.stringify({
      method: "agent/connect",
      body: { threadId, runId: `connect_${Date.now()}` },
    }),
  });

  if (!resp.ok || !resp.body) return [];

  const reader = resp.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  const messages: SnapshotMessage[] = [];

  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });

    const lines = buffer.split("\n");
    buffer = lines.pop() || "";

    let eventType = "";
    for (const line of lines) {
      if (line.startsWith("event: ")) {
        eventType = line.slice(7).trim();
      } else if (
        line.startsWith("data: ") &&
        eventType === "MESSAGES_SNAPSHOT"
      ) {
        try {
          const payload = JSON.parse(line.slice(6));
          if (Array.isArray(payload.messages)) {
            for (const m of payload.messages) {
              messages.push({
                id: m.id || String(Math.random()),
                role: m.role,
                content: typeof m.content === "string" ? m.content : "",
                ...(m.toolCalls && { toolCalls: m.toolCalls }),
                ...(m.toolCallId && { toolCallId: m.toolCallId }),
              });
            }
          }
        } catch {
          // skip malformed data
        }
        eventType = "";
      } else if (line === "") {
        eventType = "";
      }
    }
  }

  return messages;
}
