import { useEffect, useRef, type MutableRefObject } from "react";
import { toast } from "sonner";
import { RPCClient, resolveWsUrl } from "@/features/rpc/client";
import { setHTTPProjectIdProvider } from "@/features/rpc/httpRpc";
import { useRpcStore } from "@/features/rpc/rpcStore";
import type { ThreadItem, StreamEvent, ApprovalRequest, QuestionRequest } from "@/features/rpc/types";
import { projectIdProvider } from "@/features/project/projectStore";
import { useChatStore } from "./useChatStore";

interface ManuscriptCommand {
  nodeId: string;
  scope?: "selection" | "document" | "entity";
  sourceText?: string;
  selectionStart?: number;
  selectionEnd?: number;
}

export function useRpcSetup(
  manuscriptCommandsRef: MutableRefObject<Map<string, ManuscriptCommand>>
): MutableRefObject<RPCClient | null> {
  const rpcRef = useRef<RPCClient | null>(null);

  const setWsConnected = useChatStore((s) => s.setWsConnected);
  const setWsHasBeenConnected = useChatStore((s) => s.setWsHasBeenConnected);
  const setMessages = useChatStore((s) => s.setMessages);
  const setStreamText = useChatStore((s) => s.setStreamText);
  const setTurnStatus = useChatStore((s) => s.setTurnStatus);
  const setToolEvents = useChatStore((s) => s.setToolEvents);
  const setActiveTurnId = useChatStore((s) => s.setActiveTurnId);
  const setApprovals = useChatStore((s) => s.setApprovals);
  const setQuestions = useChatStore((s) => s.setQuestions);

  useEffect(() => {
    const rpc = new RPCClient(resolveWsUrl());
    rpcRef.current = rpc;
    useRpcStore.getState().setRpc(rpc);
    rpc.setProjectIdProvider(projectIdProvider);
    setHTTPProjectIdProvider(projectIdProvider);

    rpc.on("_connected", () => {
      setWsHasBeenConnected(true);
      setWsConnected(true);
    });

    rpc.on("_disconnected", () => setWsConnected(false));

    rpc.on("thread/item", (params) => {
      const item = params as ThreadItem;
      setMessages((prev) => {
        if (prev.some((m) => m.id === item.id)) return prev;
        return [...prev, item];
      });
      if (item.role === "assistant" && item.turn_id) {
        const command = manuscriptCommandsRef.current.get(item.turn_id);
        if (command) {
          window.dispatchEvent(
            new CustomEvent("manuscript-ai-result", {
              detail: {
                nodeId: command.nodeId,
                turnId: item.turn_id,
                scope: command.scope,
                sourceText: command.sourceText,
                selectionStart: command.selectionStart,
                selectionEnd: command.selectionEnd,
                content: item.content,
              },
            })
          );
          manuscriptCommandsRef.current.delete(item.turn_id);
        }
      }
    });

    rpc.on("thread/item_updated", (params) => {
      const updated = params as ThreadItem;
      setMessages((prev) => prev.map((m) => m.id === updated.id ? updated : m));
    });

    rpc.on("stream/event", (params) => {
      const evt = params as StreamEvent;
      if (evt.delta?.text) {
        setStreamText((prev) => prev + evt.delta!.text!);
      }
      if (
        evt.type === "tool_execution_start" ||
        evt.type === "tool_execution_output" ||
        evt.type === "tool_execution_result"
      ) {
        setToolEvents((prev) => [...prev, evt]);
      }
      if (evt.type === "tool_execution_start") {
        setTurnStatus("running");
      }
    });

    rpc.on("turn/finished", () => {
      setTurnStatus("idle");
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          setStreamText("");
          setToolEvents([]);
        });
      });
      setActiveTurnId("");
    });

    rpc.on("turn/error", (params) => {
      setTurnStatus("error");
      setStreamText("");
      setToolEvents([]);
      const err = params as { turnId: string; error: string };
      toast.error(err.error, { duration: 10000 });
      setTurnStatus("idle");
    });

    rpc.on("approval/request", (params) => {
      const req = params as ApprovalRequest;
      setApprovals((prev) => [...prev, req]);
      setTurnStatus("waiting");
    });

    rpc.on("question/request", (params) => {
      const req = params as QuestionRequest;
      setQuestions((prev) => [...prev, req]);
      setTurnStatus("waiting");
    });

    rpc.on("approval/timeout", (params) => {
      const { approvalId } = params as { approvalId: string };
      setApprovals((prev) => prev.filter((a) => a.id !== approvalId));
      setTurnStatus("running");
    });

    rpc.on("question/timeout", (params) => {
      const { questionId } = params as { questionId: string };
      setQuestions((prev) => prev.filter((q) => q.id !== questionId));
      setTurnStatus("running");
    });

    return () => rpc.disconnect();
  }, []);

  return rpcRef;
}
