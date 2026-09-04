import {
  createContext,
  forwardRef,
  useContext,
  useMemo,
  useState,
  type MouseEvent,
  type ReactNode,
} from "react";
import {
  Link,
  useLocation,
  useNavigate,
  type LinkProps,
  type To,
} from "react-router";

const sourceStateKey = "ipchronicleSource";
let sessionSequence = 0;

type NavigationSource = {
  sessionId: string;
  key: string;
  pathname: string;
  search: string;
  hash: string;
  historyIndex?: number;
  scrollX: number;
  scrollY: number;
};

type NavigationState = {
  [sourceStateKey]: NavigationSource;
};

const NavigationSessionContext = createContext<string | undefined>(undefined);

export function NavigationContextProvider({
  children,
}: {
  children: ReactNode;
}) {
  const [sessionId] = useState(createSessionId);
  return (
    <NavigationSessionContext value={sessionId}>
      {children}
    </NavigationSessionContext>
  );
}

export const NavigationSourceLink = forwardRef<
  HTMLAnchorElement,
  Omit<LinkProps, "state">
>(function NavigationSourceLink({ onClick, ...props }, ref) {
  const state = useNavigationSourceState();
  return (
    <Link
      {...props}
      ref={ref}
      state={state}
      onClick={(event) => {
        onClick?.(event);
        if (!event.defaultPrevented) captureNavigationSourceState(state);
      }}
    />
  );
});

export const SourceAwareBackLink = forwardRef<
  HTMLAnchorElement,
  Omit<LinkProps, "to"> & { fallback: To }
>(function SourceAwareBackLink({ fallback, onClick, ...props }, ref) {
  const location = useLocation();
  const navigate = useNavigate();
  const sessionId = useContext(NavigationSessionContext);
  const source = validSource(location.state, sessionId);
  const target = source ? sourcePath(source) : fallback;

  function navigateBack(event: MouseEvent<HTMLAnchorElement>) {
    onClick?.(event);
    if (event.defaultPrevented || !source) return;
    event.preventDefault();

    const currentIndex = browserHistoryIndex();
    if (
      source.historyIndex !== undefined &&
      currentIndex !== undefined &&
      source.historyIndex < currentIndex
    ) {
      void navigate(source.historyIndex - currentIndex);
      return;
    }

    void navigate(target, { replace: true });
    window.requestAnimationFrame(() =>
      window.scrollTo(source.scrollX, source.scrollY),
    );
  }

  return <Link {...props} ref={ref} to={target} onClick={navigateBack} />;
});

export function useNavigationSourceState(): NavigationState {
  const location = useLocation();
  const sessionId = useRequiredNavigationSession();
  return useMemo(
    () => ({
      [sourceStateKey]: {
        sessionId,
        key: location.key,
        pathname: location.pathname,
        search: location.search,
        hash: location.hash,
        historyIndex: browserHistoryIndex(),
        scrollX: window.scrollX,
        scrollY: window.scrollY,
      },
    }),
    [
      location.hash,
      location.key,
      location.pathname,
      location.search,
      sessionId,
    ],
  );
}

export function captureNavigationSourceState(state: NavigationState) {
  state[sourceStateKey] = {
    ...state[sourceStateKey],
    scrollX: window.scrollX,
    scrollY: window.scrollY,
  };
  return state;
}

function useRequiredNavigationSession() {
  const sessionId = useContext(NavigationSessionContext);
  if (!sessionId) {
    throw new Error("NavigationSourceLink requires NavigationContextProvider");
  }
  return sessionId;
}

function validSource(value: unknown, sessionId?: string) {
  if (!sessionId || !value || typeof value !== "object") return undefined;
  const source = (value as Partial<NavigationState>)[sourceStateKey];
  if (
    !source ||
    source.sessionId !== sessionId ||
    typeof source.key !== "string" ||
    !validInternalPath(source.pathname, source.search, source.hash) ||
    !Number.isFinite(source.scrollX) ||
    !Number.isFinite(source.scrollY)
  ) {
    return undefined;
  }
  return source;
}

function validInternalPath(pathname: unknown, search: unknown, hash: unknown) {
  return (
    typeof pathname === "string" &&
    pathname.startsWith("/") &&
    !pathname.startsWith("//") &&
    !pathname.includes("\0") &&
    typeof search === "string" &&
    (search === "" || search.startsWith("?")) &&
    !search.includes("\0") &&
    typeof hash === "string" &&
    (hash === "" || hash.startsWith("#")) &&
    !hash.includes("\0")
  );
}

function sourcePath(source: NavigationSource) {
  return `${source.pathname}${source.search}${source.hash}`;
}

function browserHistoryIndex() {
  const value = window.history.state?.idx;
  return typeof value === "number" && Number.isInteger(value)
    ? value
    : undefined;
}

function createSessionId() {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  sessionSequence += 1;
  return `navigation-${sessionSequence}`;
}
