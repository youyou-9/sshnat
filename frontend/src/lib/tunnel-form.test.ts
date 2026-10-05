import { describe, expect, it } from "vitest";
import { buildTunnelRequest, type TunnelFormFields } from "./tunnel-form";

const fields: TunnelFormFields = {
  name: " Database ", hostId: "ssh-host", type: "L", autoStart: true,
  localBindHost: " ::1 ", localPort: "8080", localSocket: "",
  targetHost: " db.internal ", targetPort: "3306", targetSocket: "",
  remoteBindHost: " 0.0.0.0 ", remotePort: "9090", remoteSocket: "",
  socksPort: "1080",
};

describe("tunnel form submission payload", () => {
  it("preserves local and SOCKS bind addresses when creating or editing TCP listeners", () => {
    expect(buildTunnelRequest(fields)).toMatchObject({
      name: "Database", hostId: "ssh-host", autoStart: true,
      localBindHost: "::1", localPort: 8080, targetHost: "db.internal", targetPort: 3306,
      remoteBindHost: "", remotePort: 0, remoteSocket: "", socksPort: 0,
    });
    expect(buildTunnelRequest({ ...fields, type: "D" })).toMatchObject({
      localBindHost: "::1", localPort: 0, localSocket: "",
      targetHost: "", targetPort: 0, targetSocket: "",
      remoteBindHost: "", remotePort: 0, remoteSocket: "", socksPort: 1080,
    });
  });

  it("submits remote socket endpoints without conflicting ports or stale local data", () => {
    expect(buildTunnelRequest({
      ...fields, type: "R", localSocket: "/tmp/stale.sock",
      remoteSocket: " /run/remote.sock ", targetSocket: " /tmp/service.sock ",
    })).toEqual({
      name: "Database", hostId: "ssh-host", type: "R", autoStart: true,
      localBindHost: "", localPort: 0, localSocket: "",
      targetHost: "", targetPort: 0, targetSocket: "/tmp/service.sock",
      remoteBindHost: "", remotePort: 0, remoteSocket: "/run/remote.sock", socksPort: 0,
    });
  });

  it("allows a local TCP listener to target a Unix socket without discarding its bind host", () => {
    expect(buildTunnelRequest({ ...fields, targetSocket: " /run/db.sock " })).toMatchObject({
      localBindHost: "::1", localPort: 8080, localSocket: "",
      targetHost: "", targetPort: 0, targetSocket: "/run/db.sock",
    });
  });
});
