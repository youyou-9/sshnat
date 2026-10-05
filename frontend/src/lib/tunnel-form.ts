import type { CreateTunnelRequest } from "@/lib/api";

export interface TunnelFormFields {
  name: string;
  hostId: string;
  type: string;
  autoStart: boolean;
  localBindHost: string;
  localPort: string;
  localSocket: string;
  targetHost: string;
  targetPort: string;
  targetSocket: string;
  remoteBindHost: string;
  remotePort: string;
  remoteSocket: string;
  socksPort: string;
}

/** Normalize create/edit payloads and clear inactive endpoint alternatives. */
export function buildTunnelRequest(fields: TunnelFormFields): CreateTunnelRequest {
  const isLocal = fields.type === "L";
  const isRemote = fields.type === "R";
  const isDynamic = fields.type === "D";
  const localSocket = isLocal ? fields.localSocket.trim() : "";
  const remoteSocket = isRemote ? fields.remoteSocket.trim() : "";
  const targetSocket = isLocal || isRemote ? fields.targetSocket.trim() : "";
  return {
    name: fields.name.trim(),
    hostId: fields.hostId,
    type: fields.type,
    autoStart: fields.autoStart,
    localBindHost: (isLocal && !localSocket) || isDynamic ? fields.localBindHost.trim() : "",
    localPort: isLocal && !localSocket ? Number(fields.localPort) || 0 : 0,
    localSocket,
    targetHost: (isLocal || isRemote) && !targetSocket ? fields.targetHost.trim() || "127.0.0.1" : "",
    targetPort: (isLocal || isRemote) && !targetSocket ? Number(fields.targetPort) || 0 : 0,
    targetSocket,
    remoteBindHost: isRemote && !remoteSocket ? fields.remoteBindHost.trim() : "",
    remotePort: isRemote && !remoteSocket ? Number(fields.remotePort) || 0 : 0,
    remoteSocket,
    socksPort: isDynamic ? Number(fields.socksPort) || 0 : 0,
  };
}
