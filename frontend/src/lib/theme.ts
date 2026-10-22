import { readSetting, writeSetting } from "./storage";

export type ThemeMode = "system" | "dark" | "light";

export function loadTheme(): ThemeMode {
  const saved = readSetting("sshnat.theme");
  if (saved === "dark" || saved === "light" || saved === "system") return saved;
  return "system";
}

export function applyTheme(mode: ThemeMode) {
  if (typeof document === "undefined") return;
  const root = document.documentElement;
  if (mode === "system") {
    root.removeAttribute("data-theme");
  } else {
    root.setAttribute("data-theme", mode);
  }
  writeSetting("sshnat.theme", mode);
}
