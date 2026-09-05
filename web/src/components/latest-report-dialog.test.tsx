import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { beforeEach, expect, it, vi } from "vitest";
import { getNodeNetwork } from "@/api/network";
import { getProbeSnapshot, type ProbeSnapshot } from "@/api/probes";
import { LatestReportDialog } from "@/components/latest-report-dialog";
import { NavigationContextProvider } from "@/lib/navigation-context";
import i18n from "@/i18n";

vi.mock("@/api/network", () => ({ getNodeNetwork: vi.fn() }));
vi.mock("@/api/probes", () => ({ getProbeSnapshot: vi.fn() }));

const target = {
  nodeId: "node-1",
  nodeName: "Edge 1",
  addressId: "address-1",
  address: "203.0.113.10",
};
const snapshot: ProbeSnapshot = {
  id: "snapshot-1",
  executionId: "execution-1",
  egressId: target.addressId,
  sequence: 1,
  observedAt: "2026-09-05T00:00:00Z",
  rawResult: "e30=",
  starred: false,
  fields: [
    {
      id: "Info.Organization",
      group: "Info",
      path: "Info.Organization",
      expectedTypes: ["string"],
      status: "available",
      value: "Example Network",
    },
  ],
  changes: [],
  formatIssues: [],
};
const network = {
  publicAddresses: [
    {
      id: target.addressId,
      address: target.address,
      family: "ipv4" as const,
      probeEnabled: false,
      available: true,
      pathCount: 1,
      likelyNat: false,
      proxyPath: false,
      firstSeenAt: snapshot.observedAt,
      lastSeenAt: snapshot.observedAt,
      latestSnapshotId: snapshot.id,
    },
  ],
  networkProxies: [],
  addressEvents: [],
  addressGaps: [],
};

beforeEach(async () => {
  vi.resetAllMocks();
  await i18n.changeLanguage("en");
  vi.mocked(getNodeNetwork).mockResolvedValue(network);
  vi.mocked(getProbeSnapshot).mockResolvedValue(snapshot);
});

function openPreview() {
  render(
    <MemoryRouter initialEntries={["/nodes"]}>
      <NavigationContextProvider>
        <LatestReportDialog {...target} />
      </NavigationContextProvider>
    </MemoryRouter>,
  );
  expect(getNodeNetwork).not.toHaveBeenCalled();
  fireEvent.click(
    screen.getByRole("button", { name: `View report for ${target.address}` }),
  );
  return screen.getByRole("dialog");
}

it("loads the canonical latest report on demand and reloads after reopening", async () => {
  const dialog = openPreview();
  expect(dialog).toHaveTextContent("Edge 1");
  expect(
    await within(dialog).findByText("Example Network"),
  ).toBeInTheDocument();
  expect(getProbeSnapshot).toHaveBeenCalledWith(
    snapshot.id,
    expect.any(AbortSignal),
  );
  expect(
    within(dialog).getByRole("link", { name: "Open full report" }),
  ).toHaveAttribute("href", `/probe-snapshots/${snapshot.id}`);
  fireEvent.click(within(dialog).getByRole("button", { name: "Close" }));
  const signal = vi.mocked(getProbeSnapshot).mock.calls[0][1];
  expect(signal?.aborted).toBe(true);
  vi.mocked(getNodeNetwork).mockResolvedValue({
    ...network,
    publicAddresses: [
      { ...network.publicAddresses[0], latestSnapshotId: "snapshot-2" },
    ],
  });
  vi.mocked(getProbeSnapshot).mockResolvedValue({
    ...snapshot,
    id: "snapshot-2",
  });
  fireEvent.click(
    screen.getByRole("button", { name: `View report for ${target.address}` }),
  );
  await waitFor(() =>
    expect(
      screen.getByRole("link", { name: "Open full report" }),
    ).toHaveAttribute("href", "/probe-snapshots/snapshot-2"),
  );
});

it("distinguishes an empty report history from a failed request", async () => {
  vi.mocked(getNodeNetwork).mockResolvedValue({
    ...network,
    publicAddresses: [
      { ...network.publicAddresses[0], latestSnapshotId: undefined },
    ],
  });
  const dialog = openPreview();
  expect(
    await within(dialog).findByText(
      "No successful report is available for this public IP.",
    ),
  ).toBeInTheDocument();
  expect(getProbeSnapshot).not.toHaveBeenCalled();
  expect(within(dialog).queryByRole("link")).not.toBeInTheDocument();
});

it.each(["network", "snapshot"])(
  "shows %s lookup failures and retries without presenting an empty report",
  async (stage) => {
    if (stage === "network")
      vi.mocked(getNodeNetwork).mockRejectedValueOnce(
        new Error("Network unavailable"),
      );
    else vi.mocked(getProbeSnapshot).mockRejectedValueOnce({ status: 404 });
    const dialog = openPreview();
    expect(await within(dialog).findByRole("alert")).toBeInTheDocument();
    expect(
      within(dialog).queryByText(
        "No successful report is available for this public IP.",
      ),
    ).not.toBeInTheDocument();
    fireEvent.click(within(dialog).getByRole("button", { name: "Retry" }));
    expect(
      await within(dialog).findByText("Example Network"),
    ).toBeInTheDocument();
  },
);
