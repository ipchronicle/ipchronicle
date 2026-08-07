import { useState } from "react";

export type Theme = "dark" | "light";

const themeStorageKey = "ipchronicle.theme";

function isTheme(value: string | null): value is Theme {
  return value === "dark" || value === "light";
}

export function resolveInitialTheme(): Theme {
  const stored = window.localStorage.getItem(themeStorageKey);
  if (isTheme(stored)) {
    return stored;
  }

  return window.matchMedia("(prefers-color-scheme: dark)").matches
    ? "dark"
    : "light";
}

export function applyTheme(theme: Theme) {
  document.documentElement.classList.toggle("dark", theme === "dark");
  document.documentElement.style.colorScheme = theme;
}

export function initializeTheme() {
  applyTheme(resolveInitialTheme());
}

export function useTheme() {
  const [theme, setThemeState] = useState<Theme>(resolveInitialTheme);

  const setTheme = (nextTheme: Theme) => {
    window.localStorage.setItem(themeStorageKey, nextTheme);
    applyTheme(nextTheme);
    setThemeState(nextTheme);
  };

  return { theme, setTheme };
}
