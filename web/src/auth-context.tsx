import {
  createContext,
  type ReactNode,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
} from "react";

import {
  getAuthenticatedSession,
  login as loginRequest,
  logout as logoutRequest,
  updateAccountLocale,
  type Account,
  type AuthenticatedSession,
  type LoginRequest,
} from "@/api/auth";
import { setLocale, type SupportedLocale } from "@/i18n";

type AuthState =
  | { status: "loading" }
  | { status: "anonymous" }
  | { status: "authenticated"; session: AuthenticatedSession };

type AuthContextValue = {
  state: AuthState;
  login: (credentials: LoginRequest) => Promise<void>;
  logout: () => Promise<void>;
  setAccount: (account: Account) => void;
  clearSession: () => void;
  changeLocale: (locale: SupportedLocale) => Promise<void>;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>({ status: "loading" });

  useEffect(() => {
    const controller = new AbortController();
    void getAuthenticatedSession(controller.signal)
      .then(async (session) => {
        if (session === null) {
          setState({ status: "anonymous" });
          return;
        }
        await setLocale(session.account.locale);
        setState({ status: "authenticated", session });
      })
      .catch((error: unknown) => {
        if (error instanceof DOMException && error.name === "AbortError") {
          return;
        }
        setState({ status: "anonymous" });
      });
    return () => controller.abort();
  }, []);

  useEffect(() => {
    const unauthenticated = () => setState({ status: "anonymous" });
    window.addEventListener("ipchronicle:unauthenticated", unauthenticated);
    return () =>
      window.removeEventListener(
        "ipchronicle:unauthenticated",
        unauthenticated,
      );
  }, []);

  const login = useCallback(async (credentials: LoginRequest) => {
    const session = await loginRequest(credentials);
    await setLocale(session.account.locale);
    setState({ status: "authenticated", session });
  }, []);

  const logout = useCallback(async () => {
    await logoutRequest();
    setState({ status: "anonymous" });
  }, []);

  const setAccount = useCallback((account: Account) => {
    setState((current) =>
      current.status === "authenticated"
        ? {
            status: "authenticated",
            session: { ...current.session, account },
          }
        : current,
    );
  }, []);

  const clearSession = useCallback(() => {
    setState({ status: "anonymous" });
  }, []);

  const changeLocale = useCallback(
    async (locale: SupportedLocale) => {
      if (state.status === "authenticated") {
        const account = await updateAccountLocale(locale);
        setAccount(account);
      }
      await setLocale(locale);
    },
    [setAccount, state.status],
  );

  const value = useMemo(
    () => ({
      state,
      login,
      logout,
      setAccount,
      clearSession,
      changeLocale,
    }),
    [changeLocale, clearSession, login, logout, setAccount, state],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (value === null) {
    throw new Error("useAuth must be used inside AuthProvider");
  }
  return value;
}
