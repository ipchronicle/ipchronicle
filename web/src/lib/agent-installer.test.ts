import { describe, expect, it } from "vitest";

import { agentInstallationCommand } from "@/lib/agent-installer";

describe("Agent installation command", () => {
  it("uses the fixed official installer without pinning a stable Agent version", () => {
    const command = agentInstallationCommand(
      "https://center.example",
      "registration-secret",
      "stable",
    );

    expect(command).toContain(
      "https://raw.githubusercontent.com/ipchronicle/ipchronicle/main/scripts/install-agent.sh",
    );
    expect(command).toContain("--center-url 'https://center.example'");
    expect(command).toContain("--registration-key 'registration-secret'");
    expect(command).not.toContain("--version");
    expect(command).not.toContain("--channel");
  });

  it("passes the selected RC channel to the installer and quotes credentials", () => {
    const command = agentInstallationCommand(
      "https://center.example",
      "key'with-quote",
      "rc",
    );

    expect(command).toContain("--registration-key 'key'\"'\"'with-quote'");
    expect(command).toMatch(/ --channel rc$/);
    expect(command).not.toContain("--version");
  });
});
