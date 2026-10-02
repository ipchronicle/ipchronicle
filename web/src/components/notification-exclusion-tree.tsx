import { useMemo, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import type { NotificationProbeField } from "@/api/notifications";
import { Checkbox } from "@/components/ui/checkbox";
import {
  presentProbeField,
  presentProbeFieldGroup,
} from "@/lib/probe-field-label";

type ScopeNode = {
  id: string;
  children: ScopeNode[];
  field?: NotificationProbeField;
};

function isDescendant(scope: string, parent: string) {
  return scope === parent || scope.startsWith(parent + "/");
}

function buildTree(fields: NotificationProbeField[]) {
  const roots: ScopeNode[] = [];
  const nodes = new Map<string, ScopeNode>();
  for (const field of fields) {
    let parent: ScopeNode | undefined;
    for (const scope of field.scope) {
      let node = nodes.get(scope);
      if (!node) {
        node = { id: scope, children: [] };
        nodes.set(scope, node);
        if (parent) parent.children.push(node);
        else roots.push(node);
      }
      if (scope === field.scope[field.scope.length - 1]) node.field = field;
      parent = node;
    }
  }
  return roots;
}

function scopeLabel(
  node: ScopeNode,
  fields: NotificationProbeField[],
  t: ReturnType<typeof useTranslation>["t"],
) {
  const field =
    node.field ?? fields.find((item) => item.scope.includes(node.id));
  if (field?.scope[field.scope.length - 1] === node.id) {
    return presentProbeField(field, t).name;
  }
  if (!node.id.includes("/")) return presentProbeFieldGroup(node.id, t).name;
  const parts = node.id.split("/");
  if (parts[0] === "Type" && parts.length === 2) return parts[1];
  if (parts[0] === "Score" && parts.length === 2) return parts[1];
  if (parts[0] === "Factor" && parts.length === 2) return parts[1];
  if (parts[0] === "Media" && parts.length === 2) return parts[1];
  if (parts[0] === "Mail" && parts.length === 2) return parts[1];
  const category = parts.at(-1) ?? node.id;
  return t(`notifications.rules.scopeCategory.${category}`, {
    defaultValue: category,
  });
}

function collectDescendants(node: ScopeNode): string[] {
  return [node.id, ...node.children.flatMap(collectDescendants)];
}

export function NotificationExclusionTree({
  fields,
  value,
  onChange,
}: {
  fields: NotificationProbeField[];
  value: string[];
  onChange: (value: string[]) => void;
}) {
  const { t } = useTranslation();
  const roots = useMemo(() => buildTree(fields), [fields]);

  function toggle(scope: string, checked: boolean) {
    const next = value.filter((item) => !isDescendant(item, scope));
    if (checked) next.push(scope);
    onChange([...new Set(next)].sort());
  }

  function renderNode(node: ScopeNode, depth: number): ReactNode {
    const descendants = collectDescendants(node);
    const selectedDescendants = descendants.filter((item) =>
      value.some((selected) => isDescendant(item, selected)),
    );
    const checked = selectedDescendants.length === descendants.length;
    const partial = selectedDescendants.length > 0 && !checked;
    return (
      <div key={node.id} className="space-y-1">
        <label className="flex items-center gap-2 rounded-md px-2 py-1.5 text-sm hover:bg-muted/60">
          <Checkbox
            checked={partial ? "indeterminate" : checked}
            onCheckedChange={(next) => toggle(node.id, next === true)}
          />
          <span>{scopeLabel(node, fields, t)}</span>
        </label>
        {node.children.length > 0 ? (
          <div
            className="ml-5 border-l pl-3"
            style={{ marginLeft: `${depth * 0.75 + 1.25}rem` }}
          >
            {node.children.map((child) => renderNode(child, depth + 1))}
          </div>
        ) : null}
      </div>
    );
  }

  return (
    <div className="rounded-lg border bg-muted/20 p-2">
      {roots.length > 0 ? (
        roots.map((root) => renderNode(root, 0))
      ) : (
        <p className="px-2 py-1 text-sm text-muted-foreground">
          {t("notifications.rules.exclusionEmpty")}
        </p>
      )}
    </div>
  );
}
