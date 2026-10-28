import { describe, it, expect, beforeEach, vi } from "vitest";

/**
 * 回归：后端推送的隧道状态必须真的落到界面上。
 *
 * Wails 绑定返回的是生成的模型类实例（TunnelView/Host），Svelte 5 的 `$state`
 * 不会为类实例建代理，于是"就地把 tunnel.status 改掉"只改数据、不触发重渲染，
 * 用户看到的就是"已经连上却一直显示连接中…"。这里锁住两条保证：
 * 1) 入库时隧道/主机被拍平成普通对象（后续任何就地修改都能被追踪）；
 * 2) 状态与统计事件整体替换条目，不依赖代理语义。
 */

const { listeners, FakeTunnelView, FakeHost } = vi.hoisted(() => {
  const listeners: Record<string, (ev: unknown) => void> = {};

  // 模拟 Wails 生成绑定：返回类实例而不是普通对象。
  class FakeTunnelView {
    id = "tnl-1";
    name = "api";
    hostId = "host-1";
    type = "L";
    localPort = 8080;
    targetHost = "127.0.0.1";
    targetPort = 80;
    status = "stopped";
    running = false;
    error = "";
    stats = { tx: 0, rx: 0, activeConn: 0, totalConn: 0 };
  }

  class FakeHost {
    id = "host-1";
    name = "orc";
    host = "example.com";
    port = 22;
    user = "root";
  }

  return { listeners, FakeTunnelView, FakeHost };
});

vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, cb: (ev: unknown) => void) => {
      listeners[name] = cb;
      return () => {
        delete listeners[name];
      };
    },
    Emit: vi.fn(),
  },
}));

vi.mock("./api", () => ({
  EV: {
    tunnelStatus: "sshnat:tunnel-status",
    tunnelStats: "sshnat:tunnel-stats",
    log: "sshnat:log",
  },
  TunnelService: {
    List: vi.fn().mockResolvedValue([new FakeTunnelView()]),
    Start: vi.fn().mockResolvedValue(undefined),
    Stop: vi.fn().mockResolvedValue(undefined),
    Delete: vi.fn().mockResolvedValue(undefined),
  },
  HostService: {
    List: vi.fn().mockResolvedValue([new FakeHost()]),
    Save: vi.fn().mockResolvedValue(undefined),
    Delete: vi.fn().mockResolvedValue(undefined),
  },
  SettingsService: {
    GetLanguage: vi.fn().mockResolvedValue(""),
    SetLanguage: vi.fn().mockResolvedValue(undefined),
  },
}));

import { app, initApp, refreshTunnels } from "./state.svelte";
import { TunnelService } from "./api";

function emit(name: string, data: unknown) {
  const listener = listeners[name];
  expect(listener, `no listener registered for ${name}`).toBeTypeOf("function");
  listener({ name, data });
}

describe("tunnel status propagation", () => {
  beforeEach(async () => {
    vi.clearAllMocks();
    (TunnelService.List as unknown as ReturnType<typeof vi.fn>).mockResolvedValue([new FakeTunnelView()]);
    app.tunnels = [];
    app.hosts = [];
    app.txHistory = {};
    app.rxHistory = {};
    await refreshTunnels();
  });

  it("flattens binding class instances into reactive plain objects", async () => {
    await initApp().then((cleanup) => cleanup());

    expect(app.tunnels).toHaveLength(1);
    expect(Object.getPrototypeOf(app.tunnels[0])).toBe(Object.prototype);
    expect(app.tunnels[0].status).toBe("stopped");
    expect(Object.getPrototypeOf(app.hosts[0])).toBe(Object.prototype);
  });

  it("applies a connected status event to the stored tunnel", async () => {
    const cleanup = await initApp();

    emit("sshnat:tunnel-status", {
      tunnelId: "tnl-1",
      status: "connected",
      previous: "starting",
      error: "",
    });

    expect(app.tunnels[0].status).toBe("connected");
    expect(app.tunnels[0].running).toBe(true);
    cleanup();
  });

  it("replaces the tunnel entry so subscribers re-render", async () => {
    const cleanup = await initApp();
    const before = app.tunnels[1] ?? app.tunnels[0];

    emit("sshnat:tunnel-status", { tunnelId: "tnl-1", status: "connected" });

    // 整体替换（新对象身份），而不是就地修改
    expect(app.tunnels[0]).not.toBe(before);
    cleanup();
  });

  it("applies stats so connection counters and totals show up", async () => {
    const cleanup = await initApp();

    emit("sshnat:tunnel-status", { tunnelId: "tnl-1", status: "connected" });
    emit("sshnat:tunnel-stats", {
      tunnelId: "tnl-1",
      tx: 1024,
      rx: 2048,
      txTotal: 4096,
      rxTotal: 8192,
      conns: 3,
      totalConns: 9,
    });

    expect(app.tunnels[0].stats).toEqual({ tx: 4096, rx: 8192, activeConn: 3, totalConn: 9 });
    expect(app.totalTx).toBe(1024);
    expect(app.totalRx).toBe(2048);
    cleanup();
  });

  it("marks the tunnel stopped again on a stopped event", async () => {
    const cleanup = await initApp();

    emit("sshnat:tunnel-status", { tunnelId: "tnl-1", status: "connected" });
    emit("sshnat:tunnel-status", { tunnelId: "tnl-1", status: "stopped" });

    expect(app.tunnels[0].status).toBe("stopped");
    expect(app.tunnels[0].running).toBe(false);
    cleanup();
  });
});
