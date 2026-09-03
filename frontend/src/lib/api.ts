// 对 Wails 生成绑定的薄封装，统一类型与事件名。
import * as TunnelService from "@/../bindings/github.com/sshnat/sshnat/app/tunnelservice";
import * as HostService from "@/../bindings/github.com/sshnat/sshnat/app/hostservice";
import * as SettingsService from "@/../bindings/github.com/sshnat/sshnat/app/settingsservice";
import type {
  AppInfo,
  CreateTunnelRequest,
  TunnelView,
} from "@/../bindings/github.com/sshnat/sshnat/app/models";
import type { Host as HostModel } from "@/../bindings/github.com/sshnat/sshnat/core/config/models";

export { TunnelService, HostService, SettingsService };
export type { AppInfo, CreateTunnelRequest, TunnelView };
export type Host = HostModel;
export type Tunnel = TunnelView; // 运行时视图即隧道条目（含状态字段）

// Wails 事件名（与 app/services.go 保持一致）。
export const EV = {
  tunnelStatus: "sshnat:tunnel-status",
  tunnelStats: "sshnat:tunnel-stats",
  log: "sshnat:log",
} as const;

export interface StatusEvent {
  tunnelId: string;
  status: string;
  previous?: string;
  error?: string;
  attempt?: number;
  nextInMs?: number;
}

export interface StatsEvent {
  tunnelId: string;
  tx: number;
  rx: number;
  txTotal: number;
  rxTotal: number;
  conns: number;
  totalConns: number;
}
