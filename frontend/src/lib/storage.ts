/**
 * 安全的本地存储门面。
 *
 * WebView2/Chromium 在配置目录损坏、被其它进程占用、磁盘写满或权限被拒时，
 * 访问 `window.localStorage` 本身就可能抛异常。而主题、语言、终端字体这些
 * 读取都发生在模块顶层（见 theme.ts / state.svelte.ts），一旦抛出就会中断
 * 整个入口模块的求值：用户看到的是一个没有任何提示的白窗口，重启也不会
 * 恢复，因为坏掉的配置仍旧躺在那里。
 *
 * 所以所有读写都必须经过这里：异常一律降级成"没有保存过"，并在首次探测
 * 失败时尝试一次性清空（损坏的库清空后通常自愈），保证界面总能渲染。
 */

type StorageState = "unknown" | "ok" | "broken";

const PROBE_KEY = "__sshnat_probe__";

let state: StorageState = "unknown";

/** 取底层 Storage；访问属性本身就可能抛，必须包起来。 */
function raw(): Storage | undefined {
  try {
    const candidate = (globalThis as { localStorage?: unknown }).localStorage;
    if (candidate && typeof (candidate as Storage).getItem === "function") {
      return candidate as Storage;
    }
  } catch {
    state = "broken";
  }
  return undefined;
}

/** 写入+读回+删除的自检，用来判断存储是否真的可用。 */
function probe(store: Storage): boolean {
  try {
    store.setItem(PROBE_KEY, "1");
    const ok = store.getItem(PROBE_KEY) === "1";
    store.removeItem(PROBE_KEY);
    return ok;
  } catch {
    return false;
  }
}

function ensure(): Storage | undefined {
  if (state === "broken") return undefined;
  const store = raw();
  if (!store) {
    state = "broken";
    return undefined;
  }
  if (state === "ok") return store;
  if (probe(store)) {
    state = "ok";
    return store;
  }
  // 首次探测失败：清一次再试。损坏的库被清空后大多可以恢复，
  // 这样用户不必手动删配置目录，界面也能自己修好。
  try {
    store.clear();
  } catch {
    /* 清不掉就彻底放弃持久化 */
  }
  if (probe(store)) {
    state = "ok";
    return store;
  }
  state = "broken";
  return undefined;
}

/** 读取一个持久化设置；存储不可用时返回 null（等同于"没保存过"）。 */
export function readSetting(key: string): string | null {
  const store = ensure();
  if (!store) return null;
  try {
    return store.getItem(key);
  } catch {
    state = "broken";
    return null;
  }
}

/** 写入一个持久化设置；存储不可用时静默忽略，绝不向上抛。 */
export function writeSetting(key: string, value: string): void {
  const store = ensure();
  if (!store) return;
  try {
    store.setItem(key, value);
  } catch {
    state = "broken";
  }
}

/** 当前存储状态（"unknown" = 还没探测过）。 */
export function storageState(): StorageState {
  return state;
}
