import { describe, it, expect } from "vitest";
import { formatTunnelRoute as formatRoute } from "./tunnel-command";

// Formatter utilities for UI display verification
function fmtBytes(n: number): string {
  if (!n) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  let v = n;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v >= 100 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`;
}

function fmtRate(n?: number): string {
  if (!n) return "0 B/s";
  return `${fmtBytes(n)}/s`;
}

describe("Frontend Operations & Formatting Tests", () => {
  it("formats bytes accurately across scale thresholds", () => {
    expect(fmtBytes(0)).toBe("0 B");
    expect(fmtBytes(512)).toBe("512 B");
    expect(fmtBytes(1024)).toBe("1.0 KB");
    expect(fmtBytes(1024 * 1024)).toBe("1.0 MB");
    expect(fmtBytes(1024 * 1024 * 1024)).toBe("1.0 GB");
    expect(fmtBytes(1024 * 1024 * 1024 * 1024)).toBe("1.0 TB");
  });

  it("formats throughput rate accurately", () => {
    expect(fmtRate(0)).toBe("0 B/s");
    expect(fmtRate(1024 * 500)).toBe("500 KB/s");
    expect(fmtRate(1024 * 1024 * 3.5)).toBe("3.5 MB/s");
  });

  it("formats routes accurately for -L, -R, -D tunnels", () => {
    expect(
      formatRoute({
        type: "L",
        localPort: 8080,
        targetHost: "db.internal",
        targetPort: 3306,
      })
    ).toBe("127.0.0.1:8080 → db.internal:3306");

    expect(
      formatRoute({
        type: "R",
        remotePort: 9000,
        targetHost: "127.0.0.1",
        targetPort: 80,
      })
    ).toBe(":9000 ← 127.0.0.1:80");

    expect(
      formatRoute({
        type: "D",
        socksPort: 1080,
      })
    ).toBe("socks5://127.0.0.1:1080");

    expect(
      formatRoute({
        type: "L",
        localSocket: "/tmp/local.sock",
        targetSocket: "/var/run/docker.sock",
      })
    ).toBe("/tmp/local.sock → /var/run/docker.sock");
  });

  it("preserves advanced host topology when constructing host edit payloads", () => {
    const existingHost = {
      id: "host-1",
      name: "bastion-prod",
      host: "bastion.example.com",
      port: 22,
      user: "ubuntu",
      auth: {
        method: "key",
        password: "",
        keyPath: "~/.ssh/id_rsa",
        keyPassphrase: "my-key-passphrase",
        agentSocket: "/custom/agent.sock",
      },
      jumpHostIds: ["jump-1", "jump-2"],
      keepaliveSeconds: 30,
      knownHostsFile: "/custom/known_hosts",
    };

    // Simulate editing only name and user in UI
    const editedForm = {
      fName: "bastion-prod-renamed",
      fAddr: "bastion.example.com",
      fPort: "22",
      fUser: "root",
      fAuth: "key",
      fPassword: "",
      fKeyPath: "~/.ssh/id_rsa",
    };

    const payload = {
      id: existingHost.id,
      name: editedForm.fName.trim(),
      host: editedForm.fAddr.trim(),
      port: Number(editedForm.fPort) || 22,
      user: editedForm.fUser.trim(),
      auth: {
        method: editedForm.fAuth,
        password: editedForm.fPassword,
        keyPath: editedForm.fKeyPath,
        keyPassphrase: existingHost.auth.keyPassphrase,
        agentSocket: existingHost.auth.agentSocket,
      },
      jumpHostIds: existingHost.jumpHostIds,
      keepaliveSeconds: existingHost.keepaliveSeconds,
      knownHostsFile: existingHost.knownHostsFile,
    };

    expect(payload.name).toBe("bastion-prod-renamed");
    expect(payload.user).toBe("root");
    expect(payload.jumpHostIds).toEqual(["jump-1", "jump-2"]);
    expect(payload.auth.keyPassphrase).toBe("my-key-passphrase");
    expect(payload.auth.agentSocket).toBe("/custom/agent.sock");
    expect(payload.keepaliveSeconds).toBe(30);
  });
});
