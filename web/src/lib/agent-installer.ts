import type { ReleaseChannel } from "@/api/updates";

const officialInstallerURL =
  "https://raw.githubusercontent.com/ipchronicle/ipchronicle/main/scripts/install-agent.sh";

function shellQuote(value: string) {
  return `'${value.replaceAll("'", `'"'"'`)}'`;
}

export function agentInstallationCommand(
  centerURL: string,
  registrationKey: string,
  channel: ReleaseChannel,
) {
  const channelArgument = channel === "rc" ? " --channel rc" : "";
  return (
    `curl --proto '=https' --tlsv1.2 -fsSL ${shellQuote(officialInstallerURL)} | ` +
    `sh -s -- --center-url ${shellQuote(centerURL)} --registration-key ${shellQuote(registrationKey)}` +
    channelArgument
  );
}

export function agentUninstallCommand(mode: "preserve" | "purge") {
  const purgeArgument = mode === "purge" ? " --purge" : "";
  return (
    `curl --proto '=https' --tlsv1.2 -fsSL ${shellQuote(officialInstallerURL)} | ` +
    `sh -s -- --uninstall${purgeArgument}`
  );
}
