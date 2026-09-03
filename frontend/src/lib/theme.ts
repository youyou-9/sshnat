export type ThemeMode = "system" | "dark" | "light";

export function loadTheme(): ThemeMode {
  if (typeof localStorage === "undefined") return "system";
  const saved = localStorage.getItem("sshnat.theme");
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
  if (typeof localStorage !== "undefined") {
    localStorage.setItem("sshnat.theme", mode);
  }
}
