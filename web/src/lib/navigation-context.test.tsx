import { fireEvent, render, screen } from "@testing-library/react";
import { Link, MemoryRouter, Route, Routes, useLocation } from "react-router";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  NavigationContextProvider,
  NavigationSourceLink,
  SourceAwareBackLink,
} from "@/lib/navigation-context";

describe("navigation source context", () => {
  beforeEach(() => {
    vi.spyOn(window, "scrollTo").mockImplementation(() => undefined);
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((callback) => {
      callback(0);
      return 0;
    });
  });

  afterEach(() => vi.restoreAllMocks());

  it("returns to the exact marked source URL", () => {
    renderRoutes("/source?tab=reports&page=3");
    fireEvent.click(screen.getByRole("link", { name: "Open detail" }));
    expect(screen.getByText("Detail page")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("link", { name: "Back" }));
    expect(screen.getByText("/source?tab=reports&page=3")).toBeInTheDocument();
  });

  it("uses the semantic parent without a source", () => {
    renderRoutes("/detail");
    const back = screen.getByRole("link", { name: "Back" });
    expect(back).toHaveAttribute("href", "/fallback");
    fireEvent.click(back);
    expect(screen.getByText("Fallback page")).toBeInTheDocument();
  });

  it("retains the original source across nested detail navigation", () => {
    renderRoutes("/source?status=offline");
    fireEvent.click(screen.getByRole("link", { name: "Open detail" }));
    fireEvent.click(screen.getByRole("link", { name: "Open detail tab" }));
    fireEvent.click(screen.getByRole("link", { name: "Back" }));
    expect(screen.getByText("/source?status=offline")).toBeInTheDocument();
  });

  it("captures scroll position when the source link is activated", () => {
    renderRoutes("/source");
    Object.defineProperty(window, "scrollX", { configurable: true, value: 12 });
    Object.defineProperty(window, "scrollY", {
      configurable: true,
      value: 480,
    });
    fireEvent.click(screen.getByRole("link", { name: "Open detail" }));
    fireEvent.click(screen.getByRole("link", { name: "Back" }));
    expect(window.scrollTo).toHaveBeenCalledWith(12, 480);
  });
});

function renderRoutes(initialEntry: string) {
  return render(
    <MemoryRouter initialEntries={[initialEntry]}>
      <NavigationContextProvider>
        <Routes>
          <Route
            path="/source"
            element={
              <>
                <CurrentPath />
                <NavigationSourceLink to="/detail">
                  Open detail
                </NavigationSourceLink>
              </>
            }
          />
          <Route path="/detail" element={<DetailPage />} />
          <Route path="/detail/tab" element={<DetailPage />} />
          <Route path="/fallback" element={<p>Fallback page</p>} />
        </Routes>
      </NavigationContextProvider>
    </MemoryRouter>,
  );
}

function DetailPage() {
  const location = useLocation();
  return (
    <>
      <p>Detail page</p>
      <SourceAwareBackLink fallback="/fallback">Back</SourceAwareBackLink>
      <Link to="/detail/tab" state={location.state}>
        Open detail tab
      </Link>
    </>
  );
}

function CurrentPath() {
  const location = useLocation();
  return <p>{`${location.pathname}${location.search}`}</p>;
}
