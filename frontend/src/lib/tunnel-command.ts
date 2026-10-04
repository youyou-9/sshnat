import type { Host, Tunnel } from "@/lib/api";

/** The subset of a tunnel/host used to render a copyable OpenSSH command. */
export type TunnelCommandTunnel = Pick<
  Tunnel,
  | "type"
  | "localBindHost"
  | "localPort"
  | "localSocket"
  | "targetHost"
  | "targetPort"
  | "targetSocket"
  | "remoteBindHost"
  | "remotePort"
  | "remoteSocket"
  | "socksPort"
>;

export type TunnelCommandHost = Pick<Host, "user" | "host" | "port" | "auth"> &
  Partial<Pick<Host, "id" | "jumpHostIds">>;

export type CommandShell = "posix" | "powershell";

export interface TunnelCommandOptions {
  /** Known hosts are needed to resolve saved ProxyJump IDs. */
  hosts?: readonly Host[];
  /** Windows defaults to PowerShell; other platforms default to a POSIX shell. */
  shell?: CommandShell;
}

export function defaultCommandShell(): CommandShell {
  return typeof navigator !== "undefined" && /win/i.test(navigator.platform)
    ? "powershell"
    : "posix";
}

function value(value: string | undefined | null): string {
  return value?.trim() ?? "";
}

/** Format a hostname in a host:port context without making IPv6 ambiguous. */
export function formatHost(host: string | undefined | null): string {
  const normalized = value(host);
  if (!normalized) return "";
  if (normalized.startsWith("[") && normalized.endsWith("]")) return normalized;
  return normalized.includes(":") ? `[${normalized}]` : normalized;
}

export function formatHostPort(host: string | undefined | null, port?: number, fallback = "127.0.0.1"): string {
  const formattedHost = formatHost(value(host) || fallback);
  return `${formattedHost}:${port ?? 0}`;
}

function formatListener(bindHost: string | undefined | null, port?: number, defaultHost = "127.0.0.1"): string {
  const normalizedBindHost = value(bindHost);
  if (!normalizedBindHost) return String(port ?? 0);
  return `${formatHost(normalizedBindHost || defaultHost)}:${port ?? 0}`;
}

function formatTarget(tunnel: TunnelCommandTunnel): string {
  return value(tunnel.targetSocket) || formatHostPort(tunnel.targetHost, tunnel.targetPort);
}

function formatLocalListener(tunnel: TunnelCommandTunnel): string {
  return value(tunnel.localSocket) || formatListener(tunnel.localBindHost, tunnel.localPort);
}

function formatRemoteListener(tunnel: TunnelCommandTunnel): string {
  return value(tunnel.remoteSocket) || formatListener(tunnel.remoteBindHost, tunnel.remotePort, "");
}

/** Return the route shown in tunnel cards and the table. */
export function formatTunnelRoute(tunnel: TunnelCommandTunnel): string {
  switch (tunnel.type) {
    case "L":
      return `${value(tunnel.localSocket) || formatHostPort(tunnel.localBindHost, tunnel.localPort)} → ${formatTarget(tunnel)}`;
    case "R":
      return `${value(tunnel.remoteSocket) || (value(tunnel.remoteBindHost) ? formatListener(tunnel.remoteBindHost, tunnel.remotePort) : `:${tunnel.remotePort ?? 0}`)} ← ${formatTarget(tunnel)}`;
    case "D":
      return `socks5://${value(tunnel.localBindHost) ? formatHost(tunnel.localBindHost) : "127.0.0.1"}:${tunnel.socksPort ?? 0}`;
    default:
      return `${tunnel.type}: ${formatTarget(tunnel)}`;
  }
}

function quoteArg(arg: string, shell: CommandShell): string {
  // Keep ordinary commands readable while protecting values containing shell
  // metacharacters. The safe set covers hostnames, IPv6 brackets, paths and
  // the punctuation accepted by SSH without requiring shell quoting.
  // POSIX [] characters perform pathname expansion, so bracketed IPv6 values
  // are quoted as well. A comma is a PowerShell operator outside argument mode.
  if (/^[A-Za-z0-9_./:@%+\-=]+$/.test(arg)) return arg;
  return shell === "powershell"
    ? `'${arg.replace(/'/g, "''")}'`
    : `'${arg.replace(/'/g, "'\\''")}'`;
}

function homePathArg(path: string, shell: CommandShell): string {
  // IdentityFile supports tilde expansion inside OpenSSH itself. Keeping the
  // tilde token quoted avoids shell-specific $HOME expressions and also lets
  // the SSH command importer retain the portable path stored in config.
  const normalized = path.startsWith("~\\") ? `~/${path.slice(2)}` : path;
  return quoteArg(normalized, shell);
}

function destinationHost(host: string): string {
  const normalized = value(host) || "host";
  return normalized.startsWith("[") && normalized.endsWith("]") ? normalized.slice(1, -1) : normalized;
}

function formatDestination(user: string, host: string, shell: CommandShell): string {
  const destination = `${value(user) || "root"}@${destinationHost(host)}`;
  return quoteArg(destination, shell);
}

function resolveJumpHosts(host: TunnelCommandHost | undefined, hosts: readonly Host[]): Host[] {
  if (!host?.jumpHostIds?.length) return [];
  const known = new Map(hosts.map((entry) => [entry.id, entry]));
  const visiting = new Set(host.id ? [host.id] : []);
  const flattened: Host[] = [];
  const visit = (id: string) => {
    if (!id) return;
    if (visiting.has(id)) throw new Error("SSH jump chain contains a cycle");
    const jump = known.get(id);
    if (!jump) throw new Error(`SSH jump host ${id} not found`);
    visiting.add(id);
    for (const nested of jump.jumpHostIds ?? []) visit(nested);
    visiting.delete(id);
    flattened.push(jump);
  };
  for (const id of host.jumpHostIds) visit(id);
  return flattened;
}

function formatJumpHost(host: Host): string {
  const user = value(host.user) || "root";
  const port = host.port || 22;
  return `${user}@${formatHost(host.host)}${port === 22 ? "" : `:${port}`}`;
}

/** Build an OpenSSH command quoted for the selected terminal shell. */
export function buildTunnelCliCommand(
  tunnel: TunnelCommandTunnel,
  host?: TunnelCommandHost,
  options: TunnelCommandOptions = {},
): string {
  const shell = options.shell ?? defaultCommandShell();
  const parts = ["ssh"];
  const authMethod = host?.auth?.method || "password";
  const keyPath = value(host?.auth?.keyPath);
  if (authMethod === "key" && keyPath) parts.push("-i", homePathArg(keyPath, shell));

  const port = host?.port ?? 22;
  if (port && port !== 22) parts.push("-p", String(port));
  const jumps = resolveJumpHosts(host, options.hosts ?? []);
  if (jumps.length) parts.push("-J", quoteArg(jumps.map(formatJumpHost).join(","), shell));
  parts.push("-N");

  switch (tunnel.type) {
    case "L":
      parts.push("-L", quoteArg(`${formatLocalListener(tunnel)}:${formatTarget(tunnel)}`, shell));
      break;
    case "R":
      parts.push("-R", quoteArg(`${formatRemoteListener(tunnel)}:${formatTarget(tunnel)}`, shell));
      break;
    case "D":
      parts.push("-D", quoteArg(formatListener(tunnel.localBindHost, tunnel.socksPort), shell));
      break;
    default:
      break;
  }

  parts.push(formatDestination(host?.user ?? "root", host?.host ?? "host", shell));
  return parts.join(" ");
}

/** Copy text in both secure browser contexts and the Wails desktop webview. */
export async function copyText(text: string): Promise<boolean> {
  if (typeof navigator !== "undefined" && navigator.clipboard) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      // Permission or secure-context failures can still be copied through
      // the focused selection in a Wails webview or older browser.
    }
  }
  if (typeof document === "undefined") return false;

  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  const previousActiveElement = document.activeElement;
  textarea.focus();
  textarea.select();
  let copied = false;
  try {
    copied = typeof document.execCommand === "function" && document.execCommand("copy");
  } finally {
    textarea.remove();
    if (previousActiveElement instanceof HTMLElement) previousActiveElement.focus();
  }
  return copied;
}
