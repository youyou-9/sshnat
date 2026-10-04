<script lang="ts">
  import { ArrowDown, ArrowUp, Check, LoaderCircle, Plus, Server, Pencil, X } from "@lucide/svelte";
  import Badge from "@/lib/components/ui/Badge.svelte";
  import Button from "@/lib/components/ui/Button.svelte";
  import Input from "@/lib/components/ui/Input.svelte";
  import Label from "@/lib/components/ui/Label.svelte";
  import Select from "@/lib/components/ui/Select.svelte";
  import { HostService, type Host, type HostTestResult } from "@/lib/api";
  import { app, refreshHosts, refreshTunnels } from "@/lib/state.svelte";
  import { t } from "@/lib/i18n";

  let showForm = $state(false);
  let error = $state("");
  let editingId = $state("");
  let submitting = $state(false);
  let deletingId = $state("");
  let testingId = $state("");
  let testResults = $state<Record<string, HostTestResult>>({});

  // 表单字段
  let fName = $state("");
  let fAddr = $state("");
  let fPort = $state("22");
  let fUser = $state("");
  let fAuth = $state("password");
  let fPassword = $state("");
  let fKeyPath = $state("");
  let fKeyPassphrase = $state("");
  let fAgentSocket = $state("");
  let fKeepalive = $state("0");
  let fConnectTimeout = $state("15");
  let fHostKeyPolicy = $state("accept-new");
  let fKnownHostsFile = $state("");
  let fJumpHostIds = $state<string[]>([]);
  let fNewJump = $state("");
  let showAdvanced = $state(false);

  const isFormValid = $derived.by(() => {
    if (!fName.trim() || !fAddr.trim() || !fUser.trim()) return false;
    const port = Number(fPort);
    if (!Number.isInteger(port) || port < 1 || port > 65535) return false;
    const keepalive = Number(fKeepalive);
    if (!Number.isInteger(keepalive) || keepalive < -1 || keepalive > 86400) return false;
    const timeout = Number(fConnectTimeout);
    if (!Number.isInteger(timeout) || timeout < 1 || timeout > 300) return false;
    if (fAuth === "key" && !fKeyPath.trim()) return false;
    return true;
  });

  const authOptions = $derived([
    { value: "password", label: t(app.language, "host.passwordOption") },
    { value: "key", label: t(app.language, "host.keyOption") },
    { value: "agent", label: t(app.language, "host.agentOption") },
  ]);
  const hostKeyOptions = $derived([
    { value: "accept-new", label: t(app.language, "host.keyAcceptNew") },
    { value: "strict", label: t(app.language, "host.keyStrict") },
  ]);
  const jumpOptions = $derived(app.hosts
    .filter((host) => host.id !== editingId && !fJumpHostIds.includes(host.id))
    .map((host) => ({ value: host.id, label: `${host.name} (${host.user}@${host.host})` })));

  function resetAdvanced() {
    fKeepalive = "0";
    fConnectTimeout = "15";
    fHostKeyPolicy = "accept-new";
    fKnownHostsFile = "";
    fJumpHostIds = [];
    fNewJump = "";
    showAdvanced = false;
  }

  function addJump() {
    if (fNewJump && !fJumpHostIds.includes(fNewJump)) {
      fJumpHostIds = [...fJumpHostIds, fNewJump];
      fNewJump = "";
    }
  }

  function moveJump(index: number, offset: number) {
    const next = index + offset;
    if (next < 0 || next >= fJumpHostIds.length) return;
    const ids = [...fJumpHostIds];
    [ids[index], ids[next]] = [ids[next], ids[index]];
    fJumpHostIds = ids;
  }

  function startAddHost() {
    editingId = "";
    fName = fAddr = fUser = fPassword = fKeyPath = fKeyPassphrase = fAgentSocket = "";
    fPort = "22";
    resetAdvanced();
    fAuth = "password";
    error = "";
    showForm = true;
  }

  function cancelForm() {
    showForm = false;
    editingId = "";
    fName = fAddr = fUser = fPassword = fKeyPath = fKeyPassphrase = fAgentSocket = "";
    fPort = "22";
    resetAdvanced();
    error = "";
  }

  function editHost(h: Host) {
    editingId = h.id;
    fName = h.name;
    fAddr = h.host;
    fPort = String(h.port || 22);
    fUser = h.user;
    fAuth = h.auth?.method || "password";
    fPassword = h.auth?.password ?? "";
    fKeyPath = h.auth?.keyPath ?? "";
    fKeyPassphrase = h.auth?.keyPassphrase ?? "";
    fAgentSocket = h.auth?.agentSocket ?? "";
    fKeepalive = String(h.keepaliveSeconds ?? 0);
    fConnectTimeout = String(h.connectTimeoutSeconds || 15);
    fHostKeyPolicy = h.hostKeyPolicy || "accept-new";
    fKnownHostsFile = h.knownHostsFile || "";
    fJumpHostIds = [...new Set(h.jumpHostIds || [])];
    fNewJump = "";
    showAdvanced = false;
    showForm = true;
    error = "";
  }

  async function save() {
    if (!isFormValid || submitting) return;
    error = "";
    submitting = true;
    const host = {
      id: editingId,
      name: fName.trim(),
      host: fAddr.trim(),
      port: Number(fPort) || 22,
      user: fUser.trim(),
      auth: {
        method: fAuth,
        password: fAuth === "password" ? fPassword : "",
        keyPath: fAuth === "key" ? fKeyPath.trim() : "",
        keyPassphrase: fAuth === "key" ? fKeyPassphrase : "",
        agentSocket: fAuth === "agent" ? fAgentSocket.trim() : "",
      },
      jumpHostIds: [...fJumpHostIds],
      keepaliveSeconds: Number(fKeepalive),
      connectTimeoutSeconds: Number(fConnectTimeout),
      hostKeyPolicy: fHostKeyPolicy,
      knownHostsFile: fKnownHostsFile.trim(),
    };
    try {
      await HostService.Save(host);
      cancelForm();
      await refreshHosts();
    } catch (e: any) {
      error = typeof e === "string" ? e : (e?.message ?? String(e));
    } finally {
      submitting = false;
    }
  }

  async function removeHost(id: string) {
    if (deletingId) return;
    if (typeof window !== "undefined" && !window.confirm(t(app.language, "hosts.deleteConfirm"))) {
      return;
    }
    deletingId = id;
    try {
      await HostService.Delete(id);
      await Promise.all([refreshHosts(), refreshTunnels()]);
    } catch (e: any) {
      error = typeof e === "string" ? e : (e?.message ?? String(e));
    } finally {
      deletingId = "";
    }
  }

  async function testHost(id: string) {
    if (testingId) return;
    testingId = id;
    error = "";
    try {
      const result = await HostService.TestConnection(id);
      if (result) {
        testResults[id] = result;
      }
    } catch (e: any) {
      testResults[id] = {
        hostId: id,
        success: false,
        durationMs: 0,
        error: typeof e === "string" ? e : (e?.message ?? String(e)),
      };
    } finally {
      testingId = "";
    }
  }
</script>

<div class="flex h-full flex-col">
  <header class="flex h-14 shrink-0 items-center justify-between border-b border-edge px-5">
    <div>
      <h1 class="text-[15px] font-semibold text-ink">{t(app.language, "hosts.title")}</h1>
      <p class="text-xs text-dim">{app.hosts.length} {t(app.language, "hosts.count")}</p>
    </div>
    <Button variant="primary" onclick={() => (showForm ? cancelForm() : startAddHost())}>
      <Plus size={14} class="mr-1" />
      {editingId ? t(app.language, "hosts.edit") : t(app.language, "hosts.add")}
    </Button>
  </header>

  <div class="flex-1 space-y-4 overflow-y-auto p-5">
    {#if showForm}
      <form
        class="space-y-3 rounded-card border border-edge bg-panel p-4"
        onsubmit={(e) => {
          e.preventDefault();
          save();
        }}
      >
        <div class="grid grid-cols-2 gap-3">
          <div class="space-y-1.5">
            <Label for="h-name">{t(app.language, "host.name")}</Label>
            <Input id="h-name" placeholder="bastion-prod" bind:value={fName} />
          </div>
          <div class="space-y-1.5">
            <Label for="h-user">{t(app.language, "host.user")}</Label>
            <Input id="h-user" placeholder="root" bind:value={fUser} />
          </div>
          <div class="space-y-1.5">
            <Label for="h-addr">{t(app.language, "host.address")}</Label>
            <Input id="h-addr" placeholder="ssh.example.com" bind:value={fAddr} class="font-mono" />
          </div>
          <div class="space-y-1.5">
            <Label for="h-port">{t(app.language, "host.port")}</Label>
            <Input id="h-port" placeholder="22" bind:value={fPort} class="font-mono" inputmode="numeric" />
          </div>
          <div class="space-y-1.5">
            <Label for="h-auth">{t(app.language, "host.auth")}</Label>
            <Select id="h-auth" bind:value={fAuth} options={authOptions} />
          </div>
          {#if fAuth === "password"}
            <div class="space-y-1.5 col-span-2">
              <Label for="h-password">{t(app.language, "host.password")}</Label>
              <Input id="h-password" type="password" bind:value={fPassword} />
            </div>
          {:else if fAuth === "key"}
            <div class="space-y-1.5">
              <Label for="h-keypath">{t(app.language, "host.keyPath")}</Label>
              <Input id="h-keypath" placeholder="~/.ssh/id_ed25519" bind:value={fKeyPath} class="font-mono" />
            </div>
            <div class="space-y-1.5">
              <Label for="h-passphrase">{t(app.language, "host.passphrase")}</Label>
              <Input id="h-passphrase" type="password" placeholder="Passphrase" bind:value={fKeyPassphrase} />
            </div>
          {:else if fAuth === "agent"}
            <div class="space-y-1.5 col-span-2">
              <Label for="h-agentsocket">{t(app.language, "host.agentSocket")}</Label>
              <Input id="h-agentsocket" placeholder="SSH_AUTH_SOCK / named pipe" bind:value={fAgentSocket} class="font-mono" />
            </div>
          {/if}
        </div>
        <details class="rounded-chip border border-edge bg-base/40" bind:open={showAdvanced}>
          <summary class="cursor-pointer select-none px-3 py-2.5 text-sm font-medium text-ink">
            {t(app.language, "host.advanced")}
          </summary>
          <div class="space-y-4 border-t border-edge p-3">
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div class="space-y-1.5">
                <Label for="h-timeout">{t(app.language, "host.connectTimeout")}</Label>
                <Input id="h-timeout" placeholder="15" bind:value={fConnectTimeout} class="font-mono" inputmode="numeric" />
                <p class="text-[11px] leading-4 text-dim">{t(app.language, "host.timeoutHint")}</p>
              </div>
              <div class="space-y-1.5">
                <Label for="h-keepalive">{t(app.language, "host.keepalive")}</Label>
                <Input id="h-keepalive" placeholder="0" bind:value={fKeepalive} class="font-mono" inputmode="numeric" />
                <p class="text-[11px] leading-4 text-dim">{t(app.language, "host.keepaliveHint")}</p>
              </div>
              <div class="space-y-1.5">
                <Label for="h-keypolicy">{t(app.language, "host.keyPolicy")}</Label>
                <Select id="h-keypolicy" bind:value={fHostKeyPolicy} options={hostKeyOptions} />
                <p class="text-[11px] leading-4 text-dim">{t(app.language, "host.keyPolicyHint")}</p>
              </div>
              <div class="space-y-1.5">
                <Label for="h-knownhosts">{t(app.language, "host.knownHosts")}</Label>
                <Input id="h-knownhosts" placeholder="~/.ssh/known_hosts" bind:value={fKnownHostsFile} class="font-mono" />
                <p class="text-[11px] leading-4 text-dim">{t(app.language, "host.knownHostsHint")}</p>
              </div>
            </div>
            <div class="space-y-2 border-t border-edge pt-3">
              <Label for="h-jump">{t(app.language, "host.jumpChain")}</Label>
              <p class="text-[11px] leading-4 text-dim">{t(app.language, "host.jumpHint")}</p>
              {#if fJumpHostIds.length}
                <ol class="space-y-1.5">
                  {#each fJumpHostIds as jumpId, index (jumpId)}
                    {@const jump = app.hosts.find((host) => host.id === jumpId)}
                    <li class="flex items-center gap-2 rounded-chip border border-edge bg-base px-2.5 py-2">
                      <span class="font-mono text-xs text-dim">{index + 1}</span>
                      <span class="min-w-0 flex-1 truncate text-xs text-ink">{jump?.name || jumpId}</span>
                      <Button type="button" variant="ghost" size="icon" disabled={index === 0} aria-label={t(app.language, "host.jumpUp")} title={t(app.language, "host.jumpUp")} onclick={() => moveJump(index, -1)}><ArrowUp size={13} /></Button>
                      <Button type="button" variant="ghost" size="icon" disabled={index === fJumpHostIds.length - 1} aria-label={t(app.language, "host.jumpDown")} title={t(app.language, "host.jumpDown")} onclick={() => moveJump(index, 1)}><ArrowDown size={13} /></Button>
                      <Button type="button" variant="ghost" size="icon" aria-label={t(app.language, "host.jumpRemove")} title={t(app.language, "host.jumpRemove")} onclick={() => (fJumpHostIds = fJumpHostIds.filter((id) => id !== jumpId))}><X size={13} /></Button>
                    </li>
                  {/each}
                </ol>
              {/if}
              <div class="flex gap-2">
                <div class="min-w-0 flex-1"><Select id="h-jump" bind:value={fNewJump} options={jumpOptions} placeholder={t(app.language, "host.chooseJump")} disabled={!jumpOptions.length} /></div>
                <Button type="button" variant="ghost" disabled={!fNewJump} onclick={addJump}><Plus size={13} class="mr-1" />{t(app.language, "host.jumpAdd")}</Button>
              </div>
            </div>
          </div>
        </details>
        {#if error}
          <p class="rounded-chip border border-bad/40 bg-bad/10 px-2.5 py-2 text-xs text-bad">{error}</p>
        {/if}
        <div class="flex justify-end gap-2">
          <Button variant="ghost" type="button" onclick={cancelForm}>{t(app.language, "hosts.cancel")}</Button>
          <Button variant="primary" type="submit" disabled={!isFormValid || submitting}>
            {submitting ? "..." : editingId ? t(app.language, "hosts.update") : t(app.language, "hosts.save")}
          </Button>
        </div>
      </form>
    {/if}

    {#if app.hosts.length === 0 && !showForm}
      <div class="flex h-full flex-col items-center justify-center gap-3 text-dim">
        <Server size={32} class="opacity-40" />
        <p class="text-sm">{t(app.language, "hosts.empty")}</p>
      </div>
    {:else}
      <div class="grid grid-cols-1 gap-4 xl:grid-cols-2">
        {#each app.hosts as h (h.id)}
          <div class="rounded-card border border-edge bg-panel p-4">
            <div class="flex items-start justify-between">
              <div>
                <div class="flex items-center gap-2">
                  <span class="text-sm font-medium text-ink">{h.name}</span>
                  <Badge tone="dim">{h.auth.method}</Badge>
                  {#if h.hostKeyPolicy === "strict"}<Badge tone="dim">{t(app.language, "host.keyStrict")}</Badge>{/if}
                </div>
                <div class="mt-1 font-mono text-xs text-dim">
                  {h.user}@{h.host}:{h.port}
                </div>
                {#if h.jumpHostIds?.length}
                  <div class="mt-0.5 text-[11px] text-dim/70">
                    {t(app.language, "hosts.jumpHosts")} ×{h.jumpHostIds.length}
                  </div>
                {/if}
              </div>
              <div class="flex items-center gap-2">
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={testingId !== ""}
                  aria-label={t(app.language, "hosts.test")}
                  title={t(app.language, "hosts.test")}
                  onclick={() => testHost(h.id)}
                >
                  {#if testingId === h.id}
                    <LoaderCircle size={13} class="mr-1 animate-spin" />
                    {t(app.language, "hosts.testing")}
                  {:else}
                    {t(app.language, "hosts.test")}
                  {/if}
                </Button>
                <Button variant="ghost" size="icon" aria-label={t(app.language, "hosts.edit")} title={t(app.language, "hosts.edit")} onclick={() => editHost(h)}><Pencil size={14} /></Button>
                <Button variant="danger" size="sm" disabled={deletingId === h.id} onclick={() => removeHost(h.id)}>{t(app.language, "hosts.delete")}</Button>
              </div>
            </div>
            {#if testResults[h.id]}
              {@const probe = testResults[h.id]}
              <div class={`mt-3 flex items-center gap-1.5 rounded-chip border px-2.5 py-2 text-xs ${probe.success ? "border-ok/40 bg-ok/10 text-ok" : "border-bad/40 bg-bad/10 text-bad"}`} role="status">
                {#if probe.success}
                  <Check size={13} />
                  <span>{t(app.language, "hosts.testSuccess")} · {probe.durationMs} ms</span>
                {:else}
                  <X size={13} />
                  <span class="min-w-0 break-words">{t(app.language, "hosts.testFailed")}: {probe.error}</span>
                {/if}
              </div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}

    {#if !showForm && error}
      <p class="rounded-chip border border-bad/40 bg-bad/10 px-2.5 py-2 text-xs text-bad">{error}</p>
    {/if}
  </div>
</div>
