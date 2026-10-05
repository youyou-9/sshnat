export type ThemeMode = "system" | "dark" | "light";

function getStorage(): Storage | undefined {
  const candidate = (globalThis as { localStorage?: unknown }).localStorage;
  if (
    candidate &&
    typeof (candidate as Storage).getItem === "function" &&
    typeof (candidate as Storage).setItem === "function"
  ) {
    return candidate as Storage;
  }
  return undefined;
}

export function loadTheme(): ThemeMode {
  const saved = getStorage()?.getItem("sshnat.theme");
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
  getStorage()?.setItem("sshnat.theme", mode);
}
