<script lang="ts">
  import { Terminal, KeyRound, Server, ChevronDown, ChevronUp, Copy, Check } from "@lucide/svelte";
  import { untrack } from "svelte";
  import Dialog from "@/lib/components/ui/Dialog.svelte";
  import Button from "@/lib/components/ui/Button.svelte";
  import Input from "@/lib/components/ui/Input.svelte";
  import Label from "@/lib/components/ui/Label.svelte";
  import Select from "@/lib/components/ui/Select.svelte";
  import Switch from "@/lib/components/ui/Switch.svelte";
  import Badge from "@/lib/components/ui/Badge.svelte";
  import { TunnelService, HostService, type Host, type Tunnel } from "@/lib/api";
  import { app, refreshTunnels, refreshHosts } from "@/lib/state.svelte";
  import { t } from "@/lib/i18n";
  import { buildTunnelCliCommand, copyText, defaultCommandShell, type CommandShell } from "@/lib/tunnel-command";
  import { buildTunnelRequest } from "@/lib/tunnel-form";

  let {
    open = $bindable(false),
    tunnelToEdit = null,
  }: {
    open?: boolean;
    tunnelToEdit?: Tunnel | null;
  } = $props();

  let name = $state("");
  let hostId = $state("");
  let type = $state("L");
  let localBindHost = $state("");
  let localPort = $state("");
  let localSocket = $state("");
  let remotePort = $state("");
  let remoteSocket = $state("");
  let remoteBindHost = $state("");
  let socksPort = $state("");
  let targetHost = $state("127.0.0.1");
  let targetPort = $state("");
  let targetSocket = $state("");
  let autoStart = $state(false);
  let useCmdMode = $state(false);
  let cmdText = $state("");
  let error = $state("");
  let submitting = $state(false);
  let copied = $state(false);
  let commandShell = $state<CommandShell>(defaultCommandShell());
  const commandShellOptions = [
    { value: "powershell", label: "PowerShell" },
    { value: "posix", label: "POSIX shell (bash / zsh)" },
  ];

  // 新建主机字段（当 hostId === "__new__" 时使用）
  let hName = $state("");
  let hAddr = $state("");
  let hPort = $state("22");
  let hUser = $state("root");
  let hAuth = $state("password");
  let hPassword = $state("");
  let hKeyPath = $state("");
  let hKeyPassphrase = $state("");
  let hAgentSocket = $state("");

  // 编辑现有主机认证展开态
  let showEditAuth = $state(false);

  const isEditing = $derived(!!tunnelToEdit);
  const isCreatingNewHost = $derived(hostId === "__new__" || (app.hosts.length === 0 && !isEditing));

  const typeOptions = $derived([
    { value: "L", label: t(app.language, "tunnel.local") },
    { value: "R", label: t(app.language, "tunnel.remote") },
    { value: "D", label: t(app.language, "tunnel.dynamic") },
  ]);

  const authOptions = $derived([
    { value: "password", label: t(app.language, "host.passwordOption") },
    { value: "key", label: t(app.language, "host.keyOption") },
    { value: "agent", label: t(app.language, "host.agentOption") },
  ]);

  const hostOptions = $derived([
    ...app.hosts.map((h) => ({ value: h.id, label: `${h.name} (${h.user}@${h.host}:${h.port})` })),
    { value: "__new__", label: t(app.language, "tunnel.newHostOption") },
  ]);

  const activeHostAddr = $derived.by(() => {
    if (isCreatingNewHost) return hAddr.trim();
    const h = app.hosts.find((x) => x.id === hostId);
    return h?.host ?? "";
  });

  // 生成真实、完整的 OpenSSH 命令预览
  const commandPreview = $derived.by(() => {
    const hostObj = app.hosts.find((h) => h.id === hostId);
    const previewTunnel = {
      type,
      localBindHost: localBindHost.trim(),
      localPort: Number(localPort) || 8080,
      localSocket: localSocket.trim(),
      targetHost: targetHost.trim() || "127.0.0.1",
      targetPort: Number(targetPort) || (type === "R" ? 8080 : 3306),
      targetSocket: targetSocket.trim(),
      remoteBindHost: remoteBindHost.trim(),
      remotePort: Number(remotePort) || 8080,
      remoteSocket: remoteSocket.trim(),
      socksPort: Number(socksPort) || 1080,
    };
    const previewHost = hostObj
      ? { ...hostObj, auth: showEditAuth ? { method: hAuth, keyPath: hKeyPath.trim() } : hostObj.auth }
      : {
          user: hUser.trim() || "root",
          host: hAddr.trim() || "host",
          port: Number(hPort) || 22,
          auth: { method: hAuth, keyPath: hKeyPath.trim() },
        };
    try {
      return { command: buildTunnelCliCommand(previewTunnel, previewHost, { hosts: app.hosts, shell: commandShell }), error: "" };
    } catch (previewError) {
      return { command: "", error: String(previewError instanceof Error ? previewError.message : previewError) };
    }
  });
  const previewSshCommand = $derived(commandPreview.command);

  async function copyCommand() {
    if (!previewSshCommand) return;
    try {
      if (await copyText(previewSshCommand)) {
        copied = true;
        setTimeout(() => (copied = false), 2000);
      } else error = t(app.language, "tunnel.copyError");
    } catch (copyError) {
      error = copyError instanceof Error ? copyError.message : String(copyError);
    }
  }

  // 当选择已有主机或切换时
  function onHostSelect(selectedId: string) {
    hostId = selectedId;
    showEditAuth = false;
    if (selectedId && selectedId !== "__new__") {
      const selectedHost = app.hosts.find((h) => h.id === selectedId);
      if (selectedHost) loadHostAuth(selectedHost);
    }
  }

  function loadHostAuth(host: Host) {
    hAuth = host.auth?.method || "password";
    hPassword = host.auth?.password ?? "";
    hKeyPath = host.auth?.keyPath ?? "";
    hKeyPassphrase = host.auth?.keyPassphrase ?? "";
    hAgentSocket = host.auth?.agentSocket ?? "";
  }

  const isPortValid = (p: string) => {
    const n = Number(p);
    return Number.isInteger(n) && n >= 1 && n <= 65535;
  };

  const isFormValid = $derived.by(() => {
    if (useCmdMode) return cmdText.trim().length > 0;
    if (!name.trim()) return false;

    // 校验主机
    if (isCreatingNewHost) {
      if (!hName.trim() || !hAddr.trim() || !hUser.trim()) return false;
      const hp = Number(hPort);
      if (!Number.isInteger(hp) || hp < 1 || hp > 65535) return false;
    } else {
      if (!hostId || hostId === "__new__") return false;
    }
    if ((isCreatingNewHost || showEditAuth) && hAuth === "key" && !hKeyPath.trim()) return false;

    // 校验转发参数
    if (type === "L") {
      return (isPortValid(localPort) || localSocket.trim().length > 0) &&
        (isPortValid(targetPort) || targetSocket.trim().length > 0);
    }
    if (type === "R") {
      return (isPortValid(remotePort) || remoteSocket.trim().length > 0) &&
        (isPortValid(targetPort) || targetSocket.trim().length > 0);
    }
    if (type === "D") {
      return isPortValid(socksPort);
    }
    return false;
  });

  function reset() {
    name = localBindHost = localPort = localSocket = remotePort = remoteSocket = remoteBindHost = socksPort = targetPort = targetSocket = cmdText = "";
    commandShell = defaultCommandShell();
    targetHost = "127.0.0.1";
    hName = hAddr = hPassword = hKeyPath = hKeyPassphrase = hAgentSocket = "";
    hPort = "22";
    hUser = "root";
    hAuth = "password";
    showEditAuth = false;
    type = "L";
    autoStart = false;
    useCmdMode = false;
    error = "";
    submitting = false;
    copied = false;
    if (app.hosts.length > 0) {
      hostId = app.hosts[0].id;
      loadHostAuth(app.hosts[0]);
    } else {
      hostId = "__new__";
    }
  }

  function close() { open = false; reset(); }

  $effect(() => {
    const isOpen = open;
    const editedTunnel = tunnelToEdit;
    untrack(() => {
    if (isOpen) {
      if (editedTunnel) {
        // 编辑模式：装载已有配置
        name = editedTunnel.name;
        hostId = editedTunnel.hostId;
        type = editedTunnel.type;
        localBindHost = editedTunnel.localBindHost ?? "";
        localPort = editedTunnel.localPort ? String(editedTunnel.localPort) : "";
        localSocket = editedTunnel.localSocket ?? "";
        remotePort = editedTunnel.remotePort ? String(editedTunnel.remotePort) : "";
        remoteSocket = editedTunnel.remoteSocket ?? "";
        remoteBindHost = editedTunnel.remoteBindHost ?? "";
        socksPort = editedTunnel.socksPort ? String(editedTunnel.socksPort) : "";
        targetHost = editedTunnel.targetHost || "127.0.0.1";
        targetPort = editedTunnel.targetPort ? String(editedTunnel.targetPort) : "";
        targetSocket = editedTunnel.targetSocket ?? "";
        autoStart = editedTunnel.autoStart;
        const h = app.hosts.find((x) => x.id === editedTunnel.hostId);
        if (h) loadHostAuth(h);
      } else {
        // 新建模式
        if (app.hosts.length > 0 && (!hostId || hostId === "__new__")) {
          hostId = app.hosts[0].id;
          loadHostAuth(app.hosts[0]);
        } else if (app.hosts.length === 0) {
          hostId = "__new__";
        }
      }
    } else {
      reset();
    }
    });
  });

  async function submit() {
    if (!isFormValid || submitting) return;
    error = ""; submitting = true;
    try {
      if (useCmdMode) {
        await TunnelService.CreateFromSSHCommandForShell(cmdText.trim(), commandShell);
        await Promise.all([refreshTunnels(), refreshHosts()]);
      } else {
        let finalHostId = hostId;

        // 如果是新建主机，先保存主机
        if (isCreatingNewHost) {
          const newHost: Host = {
            id: "",
            name: hName.trim(),
            host: hAddr.trim(),
            port: Number(hPort) || 22,
            user: hUser.trim(),
            auth: {
              method: hAuth,
              password: hAuth === "password" ? hPassword : "",
              keyPath: hAuth === "key" ? hKeyPath.trim() : "",
              keyPassphrase: hAuth === "key" ? hKeyPassphrase : "",
              agentSocket: hAuth === "agent" ? hAgentSocket.trim() : "",
            },
            keepaliveSeconds: 15,
          };
          await HostService.Save(newHost as any);
          await refreshHosts();
          const created = app.hosts.find(
            (h) => h.name === newHost.name && h.host === newHost.host && h.user === newHost.user
          ) ?? app.hosts[app.hosts.length - 1];
          if (!created) {
            throw new Error("Failed to create SSH host");
          }
          finalHostId = created.id;
        } else if (showEditAuth) {
          // 用户就地修改了已有主机的凭据
          const existing = app.hosts.find((h) => h.id === hostId);
          if (existing) {
            const updatedHost: Host = { ...existing, auth: {
              method: hAuth,
              password: hAuth === "password" ? hPassword : "",
              keyPath: hAuth === "key" ? hKeyPath.trim() : "",
              keyPassphrase: hAuth === "key" ? hKeyPassphrase : "",
              agentSocket: hAuth === "agent" ? hAgentSocket.trim() : "",
            } };
            await HostService.Save(updatedHost);
            await refreshHosts();
          }
        }

        const req = buildTunnelRequest({
          name, hostId: finalHostId, type, autoStart,
          localBindHost, localPort, localSocket,
          targetHost, targetPort, targetSocket,
          remoteBindHost, remotePort, remoteSocket, socksPort,
        });
        if (isEditing && tunnelToEdit) await TunnelService.Update({ ...req, id: tunnelToEdit.id });
        else await TunnelService.Create(req);
        await refreshTunnels();
      }
      close();
    } catch (e: any) { error = typeof e === "string" ? e : (e?.message ?? String(e)); }
    finally { submitting = false; }
  }
</script>

<Dialog
  bind:open={open}
  title={isEditing ? t(app.language, "tunnel.editTitle") : t(app.language, "tunnel.newTitle")}
  description={isEditing ? "" : t(app.language, "tunnel.newDescription")}
>
  <div class="space-y-4">
    {#if !useCmdMode}
      {#if !isEditing}
        <button class="flex items-center gap-1.5 text-xs text-dim transition-colors hover:text-accent" onclick={() => (useCmdMode = true)}>
          <Terminal size={12} /> {t(app.language, "tunnel.import")}
        </button>
      {/if}

      <!-- 隧道基础信息与主机选择 -->
      <div class="grid grid-cols-2 gap-3">
        <div class="space-y-1.5">
          <Label for="t-name">{t(app.language, "tunnel.name")}</Label>
          <Input id="t-name" placeholder={t(app.language, "tunnel.namePlaceholder")} bind:value={name} />
        </div>
        <div class="space-y-1.5">
          <Label for="t-host">{t(app.language, "tunnel.sshHost")}</Label>
          <Select id="t-host" bind:value={hostId} onValueChange={(v) => onHostSelect(v)} options={hostOptions} placeholder={t(app.language, "tunnel.chooseHost")} />
        </div>
      </div>

      <!-- 新建主机内嵌卡片 -->
      {#if isCreatingNewHost}
        <div class="rounded-card border border-accent/30 bg-accent/5 p-3.5 space-y-3">
          <div class="flex items-center gap-1.5 text-xs font-medium text-accent">
            <Server size={13} />
            <span>{t(app.language, "tunnel.configureHost")}</span>
          </div>
          <div class="grid grid-cols-2 gap-2.5">
            <div class="space-y-1">
              <Label for="nh-name" class="text-[11px]">{t(app.language, "host.name")}</Label>
              <Input id="nh-name" placeholder="bastion-prod" bind:value={hName} class="h-7 text-xs" />
            </div>
            <div class="space-y-1">
              <Label for="nh-user" class="text-[11px]">{t(app.language, "host.user")}</Label>
              <Input id="nh-user" placeholder="root" bind:value={hUser} class="h-7 text-xs" />
            </div>
            <div class="space-y-1">
              <Label for="nh-addr" class="text-[11px]">{t(app.language, "host.address")}</Label>
              <Input id="nh-addr" placeholder="64.110.117.189" bind:value={hAddr} class="h-7 font-mono text-xs" />
            </div>
            <div class="space-y-1">
              <Label for="nh-port" class="text-[11px]">{t(app.language, "host.port")}</Label>
              <Input id="nh-port" placeholder="22" bind:value={hPort} class="h-7 font-mono text-xs" inputmode="numeric" />
            </div>
            <div class="space-y-1 col-span-2">
              <Label for="nh-auth" class="text-[11px]">{t(app.language, "host.auth")}</Label>
              <Select id="nh-auth" bind:value={hAuth} options={authOptions} />
            </div>
            {#if hAuth === "password"}
              <div class="space-y-1 col-span-2">
                <Label for="nh-pwd" class="text-[11px]">{t(app.language, "host.password")}</Label>
                <Input id="nh-pwd" type="password" placeholder="SSH Password" bind:value={hPassword} class="h-7 text-xs" />
              </div>
            {:else if hAuth === "key"}
              <div class="space-y-1">
                <Label for="nh-kpath" class="text-[11px]">{t(app.language, "host.keyPath")}</Label>
                <Input id="nh-kpath" placeholder="~/.ssh/id_rsa" bind:value={hKeyPath} class="h-7 font-mono text-xs" />
              </div>
              <div class="space-y-1">
                <Label for="nh-kpass" class="text-[11px]">{t(app.language, "host.passphrase")}</Label>
                <Input id="nh-kpass" type="password" placeholder="Passphrase" bind:value={hKeyPassphrase} class="h-7 text-xs" />
              </div>
            {:else if hAuth === "agent"}
              <div class="space-y-1 col-span-2">
                <Label for="nh-agent" class="text-[11px]">{t(app.language, "host.agentSocket")}</Label>
                <Input id="nh-agent" placeholder="SSH_AUTH_SOCK / named pipe" bind:value={hAgentSocket} class="h-7 font-mono text-xs" />
              </div>
            {/if}
          </div>
        </div>
      {:else}
        <!-- 已选主机认证凭据快捷查看/修改 -->
        <div class="rounded-chip border border-edge bg-base/60 px-3 py-2 text-xs">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <KeyRound size={13} class="text-dim" />
              <span class="text-dim">{t(app.language, "tunnel.hostAuth")}:</span>
              <Badge tone="dim" mono>{hAuth}</Badge>
            </div>
            <button
              type="button"
              class="flex items-center gap-1 text-[11px] text-dim hover:text-accent transition-colors"
              onclick={() => (showEditAuth = !showEditAuth)}
            >
              <span>{showEditAuth ? t(app.language, "tunnel.hideHostAuth") : t(app.language, "tunnel.editHostAuth")}</span>
              {#if showEditAuth}<ChevronUp size={12} />{:else}<ChevronDown size={12} />{/if}
            </button>
          </div>
          {#if showEditAuth}
            <div class="mt-2.5 pt-2.5 border-t border-edge grid grid-cols-2 gap-2">
              <div class="space-y-1 col-span-2">
                <Label for="eh-auth" class="text-[11px]">{t(app.language, "host.auth")}</Label>
                <Select id="eh-auth" bind:value={hAuth} options={authOptions} />
              </div>
              {#if hAuth === "password"}
                <div class="space-y-1 col-span-2">
                  <Label for="eh-pwd" class="text-[11px]">{t(app.language, "host.password")}</Label>
                  <Input id="eh-pwd" type="password" placeholder="SSH Password" bind:value={hPassword} class="h-7 text-xs" />
                </div>
              {:else if hAuth === "key"}
                <div class="space-y-1">
                  <Label for="eh-kpath" class="text-[11px]">{t(app.language, "host.keyPath")}</Label>
                  <Input id="eh-kpath" placeholder="~/.ssh/id_rsa" bind:value={hKeyPath} class="h-7 font-mono text-xs" />
                </div>
                <div class="space-y-1">
                  <Label for="eh-kpass" class="text-[11px]">{t(app.language, "host.passphrase")}</Label>
                  <Input id="eh-kpass" type="password" placeholder="Passphrase" bind:value={hKeyPassphrase} class="h-7 text-xs" />
                </div>
              {:else if hAuth === "agent"}
                <div class="space-y-1 col-span-2">
                  <Label for="eh-agent" class="text-[11px]">{t(app.language, "host.agentSocket")}</Label>
                  <Input id="eh-agent" placeholder="SSH_AUTH_SOCK / named pipe" bind:value={hAgentSocket} class="h-7 font-mono text-xs" />
                </div>
              {/if}
            </div>
          {/if}
        </div>
      {/if}

      <div class="space-y-1.5">
        <Label for="t-type">{t(app.language, "tunnel.type")}</Label>
        <Select id="t-type" bind:value={type} options={typeOptions} />
        <p class="text-[11px] text-dim/70">
          {#if type === "L"}{t(app.language, "tunnel.localHint")}
          {:else if type === "R"}{t(app.language, "tunnel.remoteHint")}
          {:else}{t(app.language, "tunnel.dynamicHint")}{/if}
        </p>
      </div>

      <fieldset class="rounded-chip border border-edge p-3">
        <legend class="px-1 font-mono text-[11px] text-accent">-{type}</legend>
        {#if type === "L"}
          <div class="space-y-3">
            <div class="grid grid-cols-2 gap-3">
              <div class="space-y-1.5">
                <Label for="t-lbindhost">{t(app.language, "tunnel.localBindHost")}</Label>
                <Input id="t-lbindhost" placeholder="127.0.0.1" bind:value={localBindHost} disabled={!!localSocket.trim()} class="font-mono" />
              </div>
              <div class="space-y-1.5">
                <Label for="t-lport">{t(app.language, "tunnel.localPort")}</Label>
                <Input id="t-lport" placeholder="8080" bind:value={localPort} disabled={!!localSocket.trim()} class="font-mono" inputmode="numeric" />
              </div>
              <div class="space-y-1.5">
                <Label for="t-tport">{t(app.language, "tunnel.targetPort")}</Label>
                <Input id="t-tport" placeholder="3306" bind:value={targetPort} disabled={!!targetSocket.trim()} class="font-mono" inputmode="numeric" />
              </div>
            </div>
            <div class="grid grid-cols-2 gap-3">
              <div class="space-y-1.5">
                <Label for="t-lsocket">{t(app.language, "tunnel.localSocket")}</Label>
                <Input id="t-lsocket" placeholder="/tmp/sshnat.sock" bind:value={localSocket} class="font-mono" />
              </div>
              <div class="space-y-1.5">
                <Label for="t-tsocket">{t(app.language, "tunnel.targetSocket")}</Label>
                <Input id="t-tsocket" placeholder="/run/service.sock" bind:value={targetSocket} class="font-mono" />
              </div>
            </div>
            <p class="text-[11px] text-dim/70">{t(app.language, "tunnel.socketHint")}</p>

            <div class="space-y-1.5">
              <div class="flex items-center justify-between">
                <Label for="t-thost">{t(app.language, "tunnel.targetHost")}</Label>
                <div class="flex items-center gap-1.5">
                  <span class="text-[10px] text-dim">{t(app.language, "tunnel.commonPreset")}</span>
                  <button
                    type="button"
                    class={`px-2 py-0.5 text-[10px] rounded font-mono transition-colors border ${
                      targetHost === "127.0.0.1" ? "bg-accent/20 text-accent font-semibold border-accent/40" : "bg-base text-dim hover:text-ink border-edge"
                    }`}
                    onclick={() => (targetHost = "127.0.0.1")}
                  >
                    127.0.0.1 ({t(app.language, "tunnel.localService")})
                  </button>
                  {#if activeHostAddr && activeHostAddr !== "127.0.0.1"}
                    <button
                      type="button"
                      class={`px-2 py-0.5 text-[10px] rounded font-mono transition-colors border max-w-[130px] truncate ${
                        targetHost === activeHostAddr ? "bg-accent/20 text-accent font-semibold border-accent/40" : "bg-base text-dim hover:text-ink border-edge"
                      }`}
                      title={activeHostAddr}
                      onclick={() => (targetHost = activeHostAddr)}
                    >
                      {activeHostAddr}
                    </button>
                  {/if}
                </div>
              </div>
              <Input id="t-thost" placeholder="127.0.0.1" bind:value={targetHost} disabled={!!targetSocket.trim()} class="font-mono" />
            </div>
          </div>
          {#if !targetSocket.trim() && activeHostAddr && !/^(localhost|127\..*|\[?::1\]?)$/i.test(activeHostAddr) && targetHost.trim() === activeHostAddr}
            <div class="mt-2.5 flex items-center justify-between rounded bg-warn/10 border border-warn/30 px-2.5 py-1.5 text-[11px] text-warn">
              <span>{t(app.language, "tunnel.targetHostWarning")}</span>
              <button
                type="button"
                class="ml-2 underline hover:text-ink font-medium shrink-0"
                onclick={() => (targetHost = "127.0.0.1")}
              >
                {t(app.language, "tunnel.fixLoopback")}
              </button>
            </div>
          {/if}
        {:else if type === "R"}
          <div class="space-y-3">
            <div class="grid grid-cols-2 gap-3">
              <div class="space-y-1.5">
                <Label for="t-rbindhost">{t(app.language, "tunnel.remoteBindHost")}</Label>
                <Input id="t-rbindhost" placeholder="127.0.0.1" bind:value={remoteBindHost} disabled={!!remoteSocket.trim()} class="font-mono" />
              </div>
              <div class="space-y-1.5">
                <Label for="t-rport">{t(app.language, "tunnel.remoteBind")}</Label>
                <Input id="t-rport" placeholder="8080" bind:value={remotePort} disabled={!!remoteSocket.trim()} class="font-mono" inputmode="numeric" />
              </div>
            </div>
            <div class="grid grid-cols-2 gap-3">
              <div class="space-y-1.5">
                <Label for="t-rsocket">{t(app.language, "tunnel.remoteSocket")}</Label>
                <Input id="t-rsocket" placeholder="/tmp/sshnat.sock" bind:value={remoteSocket} class="font-mono" />
              </div>
              <div class="space-y-1.5">
                <Label for="t-rtargetsocket">{t(app.language, "tunnel.targetSocket")}</Label>
                <Input id="t-rtargetsocket" placeholder="/tmp/service.sock" bind:value={targetSocket} class="font-mono" />
              </div>
            </div>
            <p class="text-[11px] text-dim/70">{t(app.language, "tunnel.socketHint")}</p>
            <div class="grid grid-cols-2 gap-3">
              <div class="space-y-1.5">
                <div class="flex items-center justify-between">
                  <Label for="t-rhost">{t(app.language, "tunnel.remoteTarget")}</Label>
                  <button
                    type="button"
                    class="px-1.5 py-0.5 text-[10px] rounded font-mono bg-base text-dim hover:text-ink border border-edge"
                    onclick={() => (targetHost = "127.0.0.1")}
                  >
                    127.0.0.1
                  </button>
                </div>
                <Input id="t-rhost" placeholder="127.0.0.1" bind:value={targetHost} disabled={!!targetSocket.trim()} class="font-mono" />
              </div>
              <div class="space-y-1.5">
                <Label for="t-rtarget">{t(app.language, "tunnel.remoteTargetPort")}</Label>
                <Input id="t-rtarget" placeholder="8080" bind:value={targetPort} disabled={!!targetSocket.trim()} class="font-mono" inputmode="numeric" />
              </div>
            </div>
          </div>
        {:else}
          <div class="grid grid-cols-2 gap-3">
            <div class="space-y-1.5">
              <Label for="t-dbindhost">{t(app.language, "tunnel.localBindHost")}</Label>
              <Input id="t-dbindhost" placeholder="127.0.0.1" bind:value={localBindHost} class="font-mono" />
            </div>
            <div class="space-y-1.5">
              <Label for="t-dport">{t(app.language, "tunnel.socksPort")}</Label>
              <Input id="t-dport" placeholder="1080" bind:value={socksPort} class="font-mono" inputmode="numeric" />
            </div>
          </div>
        {/if}

        <!-- OpenSSH command preview quoted for the selected terminal shell. -->
        <div class="mt-3 border-t border-edge/60 pt-2.5">
          <div class="mb-2 max-w-[220px]">
            <Label for="t-command-shell">{t(app.language, "tunnel.commandShell")}</Label>
            <Select id="t-command-shell" value={commandShell} onValueChange={(shell) => (commandShell = shell as CommandShell)} options={commandShellOptions} />
          </div>
          <div class="flex items-start justify-between gap-2">
          <div class={`min-w-0 flex-1 break-all font-mono text-[11px] ${commandPreview.error ? "text-bad" : "text-accent/90"} bg-base/80 rounded px-2.5 py-1.5 border border-edge/40 select-text`}>
            {commandPreview.error || previewSshCommand}
          </div>
          <button
            type="button"
            class="flex items-center gap-1 shrink-0 rounded border border-edge/60 bg-base px-2.5 py-1 text-[11px] text-dim hover:text-ink hover:bg-hover transition-colors"
            title={t(app.language, "tunnel.copyCli")}
            disabled={!previewSshCommand}
            onclick={copyCommand}
          >
            {#if copied}
              <Check size={12} class="text-ok" />
              <span class="text-ok font-medium">{t(app.language, "common.copied")}</span>
            {:else}
              <Copy size={12} />
              <span>{t(app.language, "common.copy")}</span>
            {/if}
          </button>
        </div>
        </div>
      </fieldset>

      <div class="flex items-center justify-between rounded-chip border border-edge px-3 py-2">
        <Label for="t-autostart">{t(app.language, "tunnel.autoStart")}</Label>
        <Switch id="t-autostart" checked={autoStart} onCheckedChange={(v) => (autoStart = v)} />
      </div>
    {:else}
      <button class="text-xs text-dim transition-colors hover:text-accent" onclick={() => (useCmdMode = false)}>
        ← {t(app.language, "tunnel.back")}
      </button>
      <div class="space-y-1.5">
        <Label for="t-cmd">{t(app.language, "tunnel.commandLabel")}</Label>
        <div class="max-w-[220px]">
          <Label for="t-import-shell">{t(app.language, "tunnel.commandShell")}</Label>
          <Select id="t-import-shell" value={commandShell} onValueChange={(shell) => (commandShell = shell as CommandShell)} options={commandShellOptions} />
        </div>
        <textarea
          id="t-cmd"
          bind:value={cmdText}
          rows={3}
          class="w-full resize-none rounded-chip border border-edge bg-base px-2.5 py-2 font-mono text-xs text-ink placeholder:text-dim/60 focus:border-accent focus:outline-none"
          placeholder="ssh -L 8080:db.internal:3306 user@bastion.example.com"
        ></textarea>
        <p class="text-[11px] text-dim/70">{t(app.language, "tunnel.commandHint")}</p>
      </div>
    {/if}

    {#if error}
      <p class="rounded-chip border border-bad/40 bg-bad/10 px-2.5 py-2 text-xs text-bad">{error}</p>
    {/if}
  </div>
  {#snippet footer()}
    <Button variant="ghost" onclick={close}>{t(app.language, "tunnel.cancel")}</Button>
    <Button variant="primary" disabled={!isFormValid || submitting} onclick={submit}>
      {submitting
        ? (isEditing ? t(app.language, "tunnel.updating") : t(app.language, "tunnel.creating"))
        : (isEditing ? t(app.language, "tunnel.update") : t(app.language, "tunnel.create"))}
    </Button>
  {/snippet}
</Dialog>
