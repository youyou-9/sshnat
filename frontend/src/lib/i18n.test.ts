import { describe, it, expect } from "vitest";
import { t } from "./i18n";

describe("i18n translation system", () => {
  it("translates common keys into Chinese and English", () => {
    expect(t("zh", "nav.dashboard")).toBe("仪表盘");
    expect(t("en", "nav.dashboard")).toBe("Dashboard");

    expect(t("zh", "dashboard.new")).toBe("新建隧道");
    expect(t("en", "dashboard.new")).toBe("New Tunnel");

    expect(t("zh", "status.connected")).toBe("已连接");
    expect(t("en", "status.connected")).toBe("Connected");
  });

  it("falls back to English or key itself when translation is missing", () => {
    expect(t("zh", "nonexistent.key.test")).toBe("nonexistent.key.test");
  });

  it("has consistent keys between Chinese and English dictionaries", () => {
    // Check known critical keys
    const criticalKeys = [
      "nav.dashboard", "nav.hosts", "nav.tunnels", "nav.logs", "nav.settings",
      "hosts.add", "hosts.edit", "hosts.delete", "hosts.save", "hosts.cancel", "hosts.jumpHosts",
      "tunnel.newTitle", "tunnel.local", "tunnel.remote", "tunnel.dynamic",
      "status.connected", "status.reconnecting", "status.error", "status.stopped",
    ];

    for (const key of criticalKeys) {
      expect(t("zh", key)).not.toBe(key);
      expect(t("en", key)).not.toBe(key);
    }
  });
});
