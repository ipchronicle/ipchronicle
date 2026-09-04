import { useTranslation } from "react-i18next";

import { AgentLogViewer } from "@/components/agent-log-viewer";

export function LogsPage() {
  const { t } = useTranslation();
  return (
    <main className="w-full min-w-0 px-4 py-8 sm:px-6 sm:py-10">
      <div className="mb-6 max-w-2xl">
        <p className="text-sm font-medium text-muted-foreground uppercase">
          {t("logs.section")}
        </p>
        <h1 className="mt-2 text-2xl font-semibold sm:text-3xl">
          {t("logs.title")}
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">{t("logs.detail")}</p>
      </div>
      <AgentLogViewer />
    </main>
  );
}
