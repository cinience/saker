
import type { ReactNode } from "react";
import { useAguiHitlActions } from "./hitlActions";

export function CopilotBridge({ children }: { children: ReactNode }) {
  useAguiHitlActions();
  return <>{children}</>;
}
