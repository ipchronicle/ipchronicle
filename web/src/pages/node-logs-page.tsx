import { useParams } from "react-router";

import { AgentLogViewer } from "@/components/agent-log-viewer";

export function NodeLogsPage() {
  const { nodeId = "" } = useParams();
  return <AgentLogViewer nodeId={nodeId} />;
}
