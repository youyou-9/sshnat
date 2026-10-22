import { describe, it, expect, beforeEach, afterEach, vi } from "vitest";

/**
 * storage.ts 的价值全在"存储坏掉时也不许抛"这条线上：模块顶层的主题/语言/
 * 字体读取一旦抛出，整个入口模块就不求值，用户只会看到一个白窗口。
 */

type StorageLike = {
  getItem: (key: string) => string | null;
  setItem: (key: string, value: string) => void;
  removeItem: (key: string) => void;
  clear: () => void;
};

function installStorage(value: unknown) {
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    get: () => value,
  });
}

function memoryStorage(initial: Record<string, string> = {}): StorageLike & { map: Map<string, string> } {
  const map = new Map(Object.entries(initial));
  return {
    map,
    getItem: (key) => (map.has(key) ? map.get(key)! : null),
    setItem: (key, value) => void map.set(key, value),
    removeItem: (key) => void map.delete(key),
    clear: () => map.clear(),
  };
}

async function loadModule() {
  vi.resetModules();
  return await import("./storage");
}

describe("storage facade", () => {
  beforeEach(() => {
    installStorage(undefined);
  });

  afterEach(() => {
    Reflect.deleteProperty(globalThis, "localStorage");
  });

  it("reads and writes through to localStorage when it works", async () => {
    const storage = memoryStorage();
    installStorage(storage);
    const mod = await loadModule();

    mod.writeSetting("sshnat.theme", "dark");

    expect(storage.map.get("sshnat.theme")).toBe("dark");
    expect(mod.readSetting("sshnat.theme")).toBe("dark");
    expect(mod.storageState()).toBe("ok");
    // 自检用的探针键不能留在用户的存储里
    expect([...storage.map.keys()].some((key) => key.startsWith("__sshnat_probe__"))).toBe(false);
  });

  it("returns null instead of throwing when localStorage is absent", async () => {
    const mod = await loadModule();

    expect(() => mod.writeSetting("sshnat.theme", "dark")).not.toThrow();
    expect(mod.readSetting("sshnat.theme")).toBeNull();
    expect(mod.storageState()).toBe("broken");
  });

  it("survives a localStorage getter that throws (corrupt WebView2 profile)", async () => {
    Object.defineProperty(globalThis, "localStorage", {
      configurable: true,
      get() {
        throw new Error("SecurityError: the local storage database is corrupt");
      },
    });
    const mod = await loadModule();

    expect(mod.readSetting("sshnat.language")).toBeNull();
    expect(() => mod.writeSetting("sshnat.language", "en")).not.toThrow();
    expect(mod.storageState()).toBe("broken");
  });

  it("self-heals by clearing a storage backend that only fails before clear()", async () => {
    const map = new Map<string, string>([["garbage", "x"]]);
    let dirty = true;
    installStorage({
      getItem: (key: string) => {
        if (dirty) throw new Error("corrupt");
        return map.has(key) ? map.get(key)! : null;
      },
      setItem: (key: string, value: string) => {
        if (dirty) throw new Error("corrupt");
        map.set(key, value);
      },
      removeItem: (key: string) => void map.delete(key),
      clear: () => {
        map.clear();
        dirty = false;
      },
    });
    const mod = await loadModule();

    expect(mod.readSetting("sshnat.theme")).toBeNull();
    expect(mod.storageState()).toBe("ok");

    mod.writeSetting("sshnat.theme", "light");
    expect(mod.readSetting("sshnat.theme")).toBe("light");
  });

  it("degrades to memory-less mode when writes always fail", async () => {
    installStorage({
      getItem: () => null,
      setItem: () => {
        throw new Error("QuotaExceededError");
      },
      removeItem: () => {},
      clear: () => {},
    });
    const mod = await loadModule();

    expect(mod.readSetting("sshnat.theme")).toBeNull();
    expect(mod.storageState()).toBe("broken");
    expect(() => mod.writeSetting("sshnat.theme", "dark")).not.toThrow();
  });
});
