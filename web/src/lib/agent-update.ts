import { gt, major, valid } from "semver";

import type { Node } from "@/api/nodes";
import type { AgentUpdateState, AgentUpdateTask } from "@/api/updates";

export function nodeHasAvailableUpdate(node: Node, updates?: AgentUpdateState) {
  const targetVersion = valid(updates?.availableRelease?.version ?? "");
  const currentVersion = valid(node.agentVersion);
  return (
    node.capabilities.includes("agent-update-v1") &&
    targetVersion !== null &&
    currentVersion !== null &&
    major(targetVersion) === major(currentVersion) &&
    gt(targetVersion, currentVersion)
  );
}

export function canRequestAgentUpdate(
  node: Node,
  task: AgentUpdateTask | undefined,
  updates?: AgentUpdateState,
) {
  return (
    nodeHasAvailableUpdate(node, updates) &&
    node.enabled &&
    node.status === "online" &&
    node.deletionStatus === undefined &&
    (task === undefined || isTerminalUpdateTask(task.status))
  );
}

export function isTerminalUpdateTask(status: AgentUpdateTask["status"]) {
  return ["succeeded", "failed", "rolled-back", "rejected", "expired"].includes(
    status,
  );
}
