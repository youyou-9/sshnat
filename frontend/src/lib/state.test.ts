import { describe, it, expect, vi, beforeEach } from "vitest";

// Mock @wailsio/runtime
const listeners: Record<string, Function> = {};
vi.mock("@wailsio/runtime", () => ({
  Events: {
    On: (name: string, cb: Function) => {
      listeners[name] = cb;
      return () => {
        delete listeners[name];
      };
    },
    Emit: vi.fn(),
  },
}));

// Mock API bindings
vi.mock("./api", () => ({
  EV: {
    tunnelStatus: "sshnat:tunnel-status",
    tunnelStats: "sshnat:tunnel-stats",
    log: "sshnat:log",
  },
  TunnelService: {
    List: vi.fn().mockResolvedValue([]),
    Start: vi.fn().mockResolvedValue(undefined),
    Stop: vi.fn().mockResolvedValue(undefined),
    Delete: vi.fn().mockResolvedValue(undefined),
  },
  HostService: {
    List: vi.fn().mockResolvedValue([]),
    Save: vi.fn().mockResolvedValue(undefined),
    Delete: vi.fn().mockResolvedValue(undefined),
  },
}));

import { app, initApp, setLanguage, refreshTunnels, refreshHosts, deleteTunnel } from "./state.svelte";
import { EV, TunnelService } from "./api";

describe("Frontend Global State & Event Handling", () => {
  beforeEach(() => {
    app.tunnels = [];
    app.hosts = [];
    app.logs = [];
    app.txHistory = {};
    app.rxHistory = {};
    app.totalTx = 0;
    app.totalRx = 0;
  });

  it("sets language and updates localStorage", () => {
    setLanguage("en");
    expect(app.language).toBe("en");
    setLanguage("zh");
    expect(app.language).toBe("zh");
  });

  it("handles tunnel status events and resets history on disconnect", async () => {
    const cleanup = await initApp();

    app.tunnels = [
      {
        id: "tnl-1",
        name: "test-tunnel",
        hostId: "host-1",
        type: "L",
        status: "connected",
        running: true,
        error: "",
        stats: { tx: 100, rx: 200, activeConn: 1, totalConn: 1 },
      } as any,
    ];
    app.txHistory["tnl-1"] = [1024, 2048];
    app.rxHistory["tnl-1"] = [512, 1024];

    // Trigger status event to 'stopped'
    listeners[EV.tunnelStatus]({
      data: { tunnelId: "tnl-1", status: "stopped", error: "" },
    });

    const t = app.tunnels.find((x) => x.id === "tnl-1");
    expect(t?.status).toBe("stopped");
    expect(t?.running).toBe(false);
    expect(app.txHistory["tnl-1"]).toEqual([]);
    expect(app.rxHistory["tnl-1"]).toEqual([]);
    expect(app.totalTx).toBe(0);
    expect(app.totalRx).toBe(0);

    cleanup();
  });

  it("handles stats events and recalculates total throughput correctly", async () => {
    const cleanup = await initApp();

    app.tunnels = [
      {
        id: "t1",
        name: "tunnel 1",
        hostId: "h1",
        type: "L",
        status: "connected",
        running: true,
        error: "",
        stats: { tx: 0, rx: 0, activeConn: 0, totalConn: 0 },
      } as any,
      {
        id: "t2",
        name: "tunnel 2",
        hostId: "h2",
        type: "L",
        status: "connected",
        running: true,
        error: "",
        stats: { tx: 0, rx: 0, activeConn: 0, totalConn: 0 },
      } as any,
    ];

    // Stats event for t1
    listeners[EV.tunnelStats]({
      data: {
        tunnelId: "t1",
        tx: 1000,
        rx: 2000,
        txTotal: 1000,
        rxTotal: 2000,
        conns: 1,
        totalConns: 1,
      },
    });

    // Stats event for t2
    listeners[EV.tunnelStats]({
      data: {
        tunnelId: "t2",
        tx: 3000,
        rx: 4000,
        txTotal: 3000,
        rxTotal: 4000,
        conns: 2,
        totalConns: 2,
      },
    });

    expect(app.totalTx).toBe(4000); // 1000 + 3000
    expect(app.totalRx).toBe(6000); // 2000 + 4000

    cleanup();
  });

  it("appends logs with unique IDs and caps at 200 items", async () => {
    const cleanup = await initApp();

    for (let i = 0; i < 210; i++) {
      listeners[EV.log]({
        data: {
          time: new Date().toISOString(),
          message: `Log message ${i}`,
          tunnelId: "t1",
        },
      });
    }

    expect(app.logs.length).toBe(200);
    expect(app.logs[0].id).toBeGreaterThan(app.logs[1].id);
    expect(app.logs[0].message).toBe("Log message 209");

    cleanup();
  });

  it("deletes tunnel and cleans up history records", async () => {
    app.txHistory["del-tnl"] = [100, 200];
    app.rxHistory["del-tnl"] = [300, 400];

    await deleteTunnel("del-tnl");

    expect(TunnelService.Delete).toHaveBeenCalledWith("del-tnl");
    expect(app.txHistory["del-tnl"]).toBeUndefined();
    expect(app.rxHistory["del-tnl"]).toBeUndefined();
  });
});
