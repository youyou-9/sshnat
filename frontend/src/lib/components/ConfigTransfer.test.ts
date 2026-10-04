// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { compile } from "svelte/compiler";
import source from "./ConfigTransfer.svelte?raw";

// Load the DOM entry points explicitly; Vitest otherwise selects SSR exports.
// Internal Svelte helpers have no declarations, so keep their surface opaque.
const runtimeModule = "svelte/internal/client";
const clientModule = "../../../node_modules/svelte/src/index-client.js";
const runtime: Record<string, unknown> = await import(runtimeModule);
const { mount, unmount, flushSync } = await import(clientModule) as Pick<typeof import("svelte"), "mount" | "unmount" | "flushSync">;

// Compile the real component for the DOM. Vite's normal SSR test transform
// would use Svelte's server runtime; small UI shells keep these tests focused
// on migration actions rather than Bits UI's popup implementation.
function domComponent(sourceText: string, dependencies: Record<string, unknown> = {}) {
  const compiled = compile(sourceText, { generate: "client", dev: false }).js.code;
  const componentName = compiled.match(/export default function ([\w$]+)/)?.[1];
  if (!componentName) throw new Error("Compiled component has no default function");
  const body = compiled
    .replace(/^import\s+['"][^'"]+['"];?\s*$/gm, "")
    .replace(/^import\s+(.+?)\s+from\s+(['"])(.+?)\2;?\s*$/gm, (_match, binding: string, _quote, module: string) => {
      if (binding.startsWith("* as ")) return `const ${binding.slice(5)} = modules[${JSON.stringify(module)}];`;
      if (binding.startsWith("{")) return `const ${binding} = modules[${JSON.stringify(module)}];`;
      return `const ${binding} = modules[${JSON.stringify(module)}];`;
    })
    .replace("export default function", "function");
  // Event delegation is registered after the component declaration.
  return new Function("modules", `${body}\nreturn ${componentName};`)({ "svelte/internal/client": runtime, ...dependencies });
}

const Button = domComponent(`<script>let { children, ...rest } = $props();</script><button {...rest}>{@render children?.()}</button>`);
const Label = domComponent(`<script>let { children, ...rest } = $props();</script><label {...rest}>{@render children?.()}</label>`);
const Select = domComponent(`<script>let {value=$bindable(''),options=[],disabled=false,id}= $props();</script><select {id} {disabled} bind:value>{#each options as option}<option value={option.value}>{option.label}</option>{/each}</select>`);
const Switch = domComponent(`<script>let {checked=false,disabled=false,id,onCheckedChange}=$props();</script><button {id} {disabled} role="switch" aria-checked={checked} onclick={()=>onCheckedChange?.(!checked)}></button>`);
const Icon = domComponent(`<svg aria-hidden="true"></svg>`);

const SettingsService = { Import: vi.fn(), Export: vi.fn(), ExportFile: vi.fn() };
const refreshHosts = vi.fn();
const refreshTunnels = vi.fn();
const app = { language: "en", txHistory: {}, rxHistory: {} };
const copyText = vi.fn();
const ConfigTransfer = domComponent(source, {
  "@lucide/svelte": { Copy: Icon, Download: Icon, FileUp: Icon },
  "@/lib/api": { SettingsService },
  "@/lib/state.svelte": { app, refreshHosts, refreshTunnels },
  "@/lib/tunnel-command": { copyText },
  "@/lib/components/ui/Button.svelte": Button,
  "@/lib/components/ui/Label.svelte": Label,
  "@/lib/components/ui/Select.svelte": Select,
  "@/lib/components/ui/Switch.svelte": Switch,
});

let component: ReturnType<typeof mount>;
const button = (text: string) => [...document.querySelectorAll<HTMLButtonElement>("button")].find((item) => item.textContent === text)!;
const settle = async () => { await Promise.resolve(); await Promise.resolve(); await Promise.resolve(); flushSync(); };
const paste = (text: string) => {
  const textarea = document.querySelector<HTMLTextAreaElement>("#import-json")!;
  textarea.value = text;
  textarea.dispatchEvent(new Event("input", { bubbles: true }));
  flushSync();
};

beforeEach(async () => {
  vi.resetAllMocks();
  refreshHosts.mockResolvedValue(undefined);
  refreshTunnels.mockResolvedValue(undefined);
  SettingsService.Import.mockResolvedValue({ hostsAdded: 1, tunnelsAdded: 1 });
  component = mount(ConfigTransfer, { target: document.body });
  await settle();
});
afterEach(async () => { await unmount(component); document.body.innerHTML = ""; });

describe("configuration transfer user actions", () => {
  it("clears committed input when a subsequent refresh fails, preventing accidental repeat imports", async () => {
    refreshTunnels.mockRejectedValueOnce(new Error("offline"));
    paste('{"version":1,"hosts":[],"tunnels":[]}');
    button("Import Configuration").click();
    await settle();
    expect(SettingsService.Import).toHaveBeenCalledTimes(1);
    expect(document.querySelector<HTMLTextAreaElement>("#import-json")!.value).toBe("");
    expect(button("Import Configuration").disabled).toBe(true);
    expect(document.querySelector('[role="alert"]')!.textContent).toContain("Configuration saved");
    expect(document.querySelector('[role="status"]')!.textContent).toContain("Imported");
  });

  it("keeps input for retry when the import itself fails and displays native export failures", async () => {
    SettingsService.Import.mockRejectedValueOnce(new Error("invalid config"));
    paste("invalid");
    button("Import Configuration").click();
    await settle();
    expect(document.querySelector<HTMLTextAreaElement>("#import-json")!.value).toBe("invalid");
    expect(document.querySelector('[role="alert"]')!.textContent).toBe("invalid config");
    SettingsService.ExportFile.mockRejectedValueOnce(new Error("write denied"));
    button("Save Configuration File").click();
    await settle();
    expect(SettingsService.ExportFile).toHaveBeenCalledWith(false);
    expect(document.querySelector('[role="alert"]')!.textContent).toBe("write denied");
    expect(button("Save Configuration File").disabled).toBe(false);
  });

  it("blocks importing stale textarea content while a newly selected file is being read", async () => {
    paste("old document");
    const importButton = button("Import Configuration");
    const textarea = document.querySelector<HTMLTextAreaElement>("#import-json")!;
    let finishRead!: (text: string) => void;
    const file = { size: 100, text: vi.fn(() => new Promise<string>((resolve) => { finishRead = resolve; })) };
    const input = document.querySelector<HTMLInputElement>("#import-file")!;
    Object.defineProperty(input, "files", { configurable: true, value: [file] });
    input.dispatchEvent(new Event("change", { bubbles: true }));
    await settle();
    expect(file.text, document.querySelector('[role="alert"]')?.textContent ?? "No visible file-read error").toHaveBeenCalledOnce();
    expect(importButton.disabled).toBe(true);
    expect(textarea.disabled).toBe(true);
    expect(textarea.value).toBe("");
    importButton.click();
    expect(SettingsService.Import).not.toHaveBeenCalled();
    finishRead("new document");
    await settle();
    expect(importButton.disabled).toBe(false);
    expect(textarea.disabled).toBe(false);
    expect(textarea.value).toBe("new document");
  });

  it("clears stale input and restores controls when reading a file fails", async () => {
    paste("old document");
    const file = { size: 100, text: vi.fn().mockRejectedValue(new Error("file unavailable")) };
    const input = document.querySelector<HTMLInputElement>("#import-file")!;
    Object.defineProperty(input, "files", { configurable: true, value: [file] });
    input.dispatchEvent(new Event("change", { bubbles: true }));
    await settle();
    expect(document.querySelector('[role="alert"]')!.textContent).toBe("file unavailable");
    expect(document.querySelector<HTMLTextAreaElement>("#import-json")!.value).toBe("");
    expect(button("Import Configuration").disabled).toBe(true);
    expect(input.disabled).toBe(false);
    expect(button("Save Configuration File").disabled).toBe(false);
  });

  it("rejects oversized files without reading them or retaining a previous document", async () => {
    paste("old document");
    const file = { size: 10 * 1024 * 1024 + 1, text: vi.fn() };
    const input = document.querySelector<HTMLInputElement>("#import-file")!;
    Object.defineProperty(input, "files", { configurable: true, value: [file] });
    input.dispatchEvent(new Event("change", { bubbles: true }));
    await settle();
    expect(file.text).not.toHaveBeenCalled();
    expect(document.querySelector('[role="alert"]')!.textContent).toContain("10 MiB");
    expect(document.querySelector<HTMLTextAreaElement>("#import-json")!.value).toBe("");
    expect(button("Import Configuration").disabled).toBe(true);
    expect(input.disabled).toBe(false);
  });
});
