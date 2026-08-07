import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import App from "@/App";
import { getSystemStatus } from "@/api/system";
import i18n from "@/i18n";

vi.mock("@/api/system", () => ({
  getSystemStatus: vi.fn(),
}));

const getSystemStatusMock = vi.mocked(getSystemStatus);

describe("system status application", () => {
  beforeEach(async () => {
    window.localStorage.clear();
    document.documentElement.className = "";
    await i18n.changeLanguage("en");
    getSystemStatusMock.mockReset();
  });

  it("renders the real center response", async () => {
    getSystemStatusMock.mockResolvedValue({
      service: "ipchronicle-center",
      status: "ok",
      version: "0.0.0-test",
    });

    render(<App />);

    expect(await screen.findByText("Operational")).toBeInTheDocument();
    expect(screen.getByText("ipchronicle-center")).toBeInTheDocument();
    expect(screen.getByText("0.0.0-test")).toBeInTheDocument();
  });

  it("shows a recoverable API failure", async () => {
    getSystemStatusMock
      .mockRejectedValueOnce(new Error("offline"))
      .mockResolvedValueOnce({
        service: "ipchronicle-center",
        status: "ok",
        version: "dev",
      });

    render(<App />);

    fireEvent.click(await screen.findByRole("button", { name: "Retry" }));
    expect(await screen.findByText("Operational")).toBeInTheDocument();
  });

  it("switches locale and theme without a reload", async () => {
    getSystemStatusMock.mockResolvedValue({
      service: "ipchronicle-center",
      status: "ok",
      version: "dev",
    });

    render(<App />);
    fireEvent.click(
      screen.getByRole("button", { name: "Switch to Simplified Chinese" }),
    );

    expect(
      await screen.findByRole("heading", { name: "系统状态" }),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "使用深色主题" }));
    expect(document.documentElement).toHaveClass("dark");
  });
});
