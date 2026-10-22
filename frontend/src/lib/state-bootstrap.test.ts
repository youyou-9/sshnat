import { describe, it, expect, afterEach, vi } from "vitest";

/**
 * 入口模块必须能在"本地存储已损坏"或"确实存过字号"的情况下完成求值。
 * 之前把终端字号/字体常量声明在 `app` 之后，一旦用户真的存过字号，
 * 模块顶层读取常量就会 TDZ 抛错，Svelte 挂不上 —— 用户看到的白窗口，
 * 且重启也修不好。这个测试锁住"带持久化值也能正常初始化"。
 */

vi.mock("@wailsio/runtime", () => ({
  Events: { On: () => () => {}, Emit: vi.fn() },
}));

vi.mock("./api", () => ({
  EV: { tunnelStatus: "sshnat:tunnel-status", tunnelStats: "sshnat:tunnel-stats", log: "sshnat:log" },
  TunnelService: {},
  HostService: {},
  SettingsService: {},
}));

function installStorage(entries: Record<string, string>) {
  const map = new Map(Object.entries(entries));
  const storage = {
    getItem: (key: string) => (map.has(key) ? map.get(key)! : null),
    setItem: (key: string, value: string) => void map.set(key, value),
    removeItem: (key: string) => void map.delete(key),
    clear: () => map.clear(),
  };
  Object.defineProperty(globalThis, "localStorage", { configurable: true, get: () => storage });
  return map;
}

async function bootState() {
  vi.resetModules();
  return await import("./state.svelte");
}

describe("state module bootstrap", () => {
  afterEach(() => {
    Reflect.deleteProperty(globalThis, "localStorage");
  });

  it("boots with a persisted terminal font and size", async () => {
    installStorage({ "sshnat.termFont": "Consolas", "sshnat.termFontSize": "15" });

    const mod = await bootState();

    expect(mod.app.terminalFontFamily).toBe("Consolas");
    expect(mod.app.terminalFontSize).toBe(15);
  });

  it("boots with defaults when nothing is persisted", async () => {
    installStorage({});

    const mod = await bootState();

    expect(mod.app.terminalFontFamily).toBe("JetBrains Mono");
    expect(mod.app.terminalFontSize).toBe(13);
    expect(mod.app.language).toBe("zh");
  });

  it("boots when local storage throws on access", async () => {
    Object.defineProperty(globalThis, "localStorage", {
      configurable: true,
      get() {
        throw new Error("SecurityError: local storage is corrupt");
      },
    });

    const mod = await bootState();

    expect(mod.app.terminalFontSize).toBe(13);
    expect(mod.app.theme).toBe("system");
  });

  it("ignores an out-of-range persisted font size", async () => {
    installStorage({ "sshnat.termFontSize": "999", "sshnat.termFont": "Unifont" });

    const mod = await bootState();

    expect(mod.app.terminalFontSize).toBe(13);
    expect(mod.app.terminalFontFamily).toBe("JetBrains Mono");
  });
});
