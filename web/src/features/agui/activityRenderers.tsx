
import type { ReactActivityMessageRenderer } from "@copilotkit/react-core/v2";

interface ActivityContent {
  tool?: string;
  status?: string;
  step?: string;
  iteration?: number;
}

const anySchema = {
  safeParse: (value: unknown) => ({ success: true as const, data: value as ActivityContent }),
  "~standard": {
    version: 1 as const,
    vendor: "saker",
    validate: (value: unknown) => ({ value: value as ActivityContent }),
  },
};

function ToolExecutionActivity({ content }: { activityType: string; content: ActivityContent; message: unknown; agent: unknown }) {
  const status = content.status ?? "running";
  const toolName = content.tool ?? "tool";
  return (
    <div className="activity-indicator">
      <span className={`activity-dot activity-dot--${status}`} />
      <span className="activity-tool">{toolName}</span>
      <span className="activity-status">
        {status === "running" ? "执行中..." : status === "error" ? "出错" : "完成"}
      </span>
    </div>
  );
}

function IterationActivity({ content }: { activityType: string; content: ActivityContent; message: unknown; agent: unknown }) {
  const step = content.step ?? `iteration_${content.iteration ?? 1}`;
  return (
    <div className="activity-indicator">
      <span className="activity-dot activity-dot--running" />
      <span className="activity-step">{step}</span>
    </div>
  );
}

export const activityRenderers: ReactActivityMessageRenderer<ActivityContent>[] = [
  {
    activityType: "TOOL_EXECUTION",
    content: anySchema as any,
    render: ToolExecutionActivity as any,
  },
  {
    activityType: "ITERATION",
    content: anySchema as any,
    render: IterationActivity as any,
  },
];
