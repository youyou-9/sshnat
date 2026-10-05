// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";
import { buildTunnelCliCommand, copyText, formatTunnelRoute } from "./tunnel-command";
import type { Host } from "./api";
import advancedCommands from "./test-fixtures/advanced-ssh-commands.json";

function sshHost(id: string, overrides: Partial<Host> = {}): Host {
  return { id, name: id, user: "root", host: `${id}.example`, port: 22, auth: { method: "password" }, ...overrides };
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
  document.body.innerHTML = "";
});

describe("tunnel route and SSH command formatting", () => {
  for (const fixture of advancedCommands) {
    it(`preserves advanced options in the ${fixture.shell} import fixture for ${fixture.host.host}`, () => {
      expect(buildTunnelCliCommand(fixture.tunnel, fixture.host, { shell: fixture.shell as "posix" | "powershell", useAppDefaults: fixture.useAppDefaults })).toBe(fixture.command);
    });
  }

  it("keeps explicit empty host key policy and the default agent semantically portable", () => {
    const host = sshHost("destination", { hostKeyPolicy: "", auth: { method: "agent", agentSocket: "" } });
    expect(buildTunnelCliCommand({ type: "D", socksPort: 1080 }, host, { shell: "posix" })).toBe(
      "ssh -o StrictHostKeyChecking=accept-new -o IdentityAgent=SSH_AUTH_SOCK -N -D 1080 root@destination.example"
    );
  });

  it("includes app defaults for real UI hosts whose optional fields were omitted by JSON", () => {
    expect(buildTunnelCliCommand({ type: "D", socksPort: 1080 }, sshHost("destination"), { shell: "posix", useAppDefaults: true })).toBe(
      "ssh -o StrictHostKeyChecking=accept-new -o ConnectTimeout=15 -o ServerAliveInterval=15 -N -D 1080 root@destination.example"
    );
  });

  it("keeps the default local listener visible in route labels", () => {
    expect(formatTunnelRoute({ type: "L", localPort: 8080, targetHost: "db.internal", targetPort: 3306 })).toBe(
      "127.0.0.1:8080 → db.internal:3306"
    );
  });

  it("renders IPv6 listeners and targets without ambiguous colons", () => {
    expect(
      formatTunnelRoute({
        type: "L",
        localBindHost: "::1",
        localPort: 8080,
        targetHost: "2001:db8::10",
        targetPort: 443,
      })
    ).toBe("[::1]:8080 → [2001:db8::10]:443");
  });

  it("renders remote Unix socket routes and custom SOCKS bind hosts", () => {
    expect(
      formatTunnelRoute({
        type: "R",
        remoteSocket: "/run/sshnat.sock",
        targetSocket: "/tmp/service.sock",
      })
    ).toBe("/run/sshnat.sock ← /tmp/service.sock");

    expect(formatTunnelRoute({ type: "D", localBindHost: "0.0.0.0", socksPort: 1080 })).toBe(
      "socks5://0.0.0.0:1080"
    );
  });

  it("builds a local socket command with an IPv6 target and non-default SSH options", () => {
    expect(
      buildTunnelCliCommand(
        {
          type: "L",
          localSocket: "/tmp/local.sock",
          targetHost: "2001:db8::10",
          targetPort: 443,
        },
        {
          user: "deploy",
          host: "2001:db8::20",
          port: 2200,
          auth: { method: "key", keyPath: "/home/deploy/my key" },
        },
        { shell: "posix" }
      )
    ).toBe("ssh -i '/home/deploy/my key' -p 2200 -N -L '/tmp/local.sock:[2001:db8::10]:443' deploy@2001:db8::20");
  });

  it("preserves remote sockets and bind addresses in copied commands", () => {
    expect(
      buildTunnelCliCommand(
        {
          type: "R",
          remoteSocket: "/run/remote.sock",
          targetSocket: "/tmp/local.sock",
        },
        { user: "root", host: "server.example", port: 22, auth: { method: "password" } }
      )
    ).toBe("ssh -N -R /run/remote.sock:/tmp/local.sock root@server.example");

    expect(
      buildTunnelCliCommand(
        { type: "D", localBindHost: "0.0.0.0", socksPort: 1080 },
        { user: "root", host: "server.example", port: 22, auth: { method: "password" } }
      )
    ).toBe("ssh -N -D 0.0.0.0:1080 root@server.example");
  });

  it("quotes destination values that contain shell metacharacters", () => {
    expect(
      buildTunnelCliCommand(
        { type: "D", socksPort: 1080 },
        { user: "ops;echo", host: "host.example", port: 22, auth: { method: "password" } }
      )
    ).toBe("ssh -N -D 1080 'ops;echo@host.example'");
  });

  it("expands nested ProxyJump IDs in the same nearest-to-farthest order as the backend", () => {
    const nearest = sshHost("nearest", { host: "2001:db8::1", port: 2200, user: "gateway" });
    const middle = sshHost("middle", { jumpHostIds: [nearest.id] });
    const last = sshHost("last");
    const host = sshHost("destination", { jumpHostIds: [middle.id, last.id] });
    expect(
      buildTunnelCliCommand({ type: "D", socksPort: 1080 }, host, {
        hosts: [host, nearest, middle, last],
        shell: "posix",
      })
    ).toBe("ssh -J 'gateway@[2001:db8::1]:2200,root@middle.example,root@last.example' -N -D 1080 root@destination.example");
  });

  it("does not silently omit missing or cyclic ProxyJump references", () => {
    const host = sshHost("destination", { jumpHostIds: ["missing"] });
    expect(() => buildTunnelCliCommand({ type: "D", socksPort: 1080 }, host, { hosts: [host] })).toThrow(
      "SSH jump host missing not found"
    );
    const jump = sshHost("jump", { jumpHostIds: [host.id] });
    host.jumpHostIds = [jump.id];
    expect(() => buildTunnelCliCommand({ type: "D", socksPort: 1080 }, host, { hosts: [host, jump] })).toThrow(
      "SSH jump chain contains a cycle"
    );
  });

  it("quotes complete forwarding arguments with spaces, quotes and shell metacharacters", () => {
    const tunnel = { type: "R", remoteSocket: "/run/o'brien.sock", targetSocket: "/tmp/service;$x.sock" };
    expect(buildTunnelCliCommand(tunnel, sshHost("destination"), { shell: "posix" })).toBe(
      "ssh -N -R '/run/o'\\''brien.sock:/tmp/service;$x.sock' root@destination.example"
    );
    expect(buildTunnelCliCommand(tunnel, sshHost("destination"), { shell: "powershell" })).toBe(
      "ssh -N -R '/run/o''brien.sock:/tmp/service;$x.sock' root@destination.example"
    );
  });

  it("keeps OpenSSH's own tilde expansion in key paths for both shells", () => {
    const host = sshHost("destination", { auth: { method: "key", keyPath: "~/.ssh/my key" } });
    const tunnel = { type: "D", socksPort: 1080 };
    expect(buildTunnelCliCommand(tunnel, host, { shell: "posix" })).toBe(
      "ssh -i '~/.ssh/my key' -N -D 1080 root@destination.example"
    );
    expect(buildTunnelCliCommand(tunnel, host, { shell: "powershell" })).toBe(
      "ssh -i '~/.ssh/my key' -N -D 1080 root@destination.example"
    );
  });

  it("falls back to focused selection when the clipboard API rejects", async () => {
    const writeText = vi.fn().mockRejectedValue(new Error("denied"));
    vi.stubGlobal("navigator", { clipboard: { writeText } });
    const execCopy = vi.fn().mockReturnValue(true);
    vi.stubGlobal("document", document);
    document.execCommand = execCopy;
    const button = document.createElement("button");
    document.body.appendChild(button);
    button.focus();

    expect(await copyText("ssh -N -D 1080 root@host")).toBe(true);
    expect(writeText).toHaveBeenCalledOnce();
    expect(execCopy).toHaveBeenCalledWith("copy");
    expect(document.querySelector("textarea")).toBeNull();
    expect(document.activeElement).toBe(button);
  });
});
