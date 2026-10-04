package main

import (
	"embed"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/sshnat/sshnat/app"
	"github.com/sshnat/sshnat/core/config"
	"github.com/sshnat/sshnat/core/stats"
	"github.com/sshnat/sshnat/core/supervisor"
	"github.com/sshnat/sshnat/internal/version"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

func main() {
	// ---- core 装配（与 UI 无关）----
	cfgPath, err := config.DefaultPath()
	if err != nil {
		log.Fatalf("resolve config path: %v", err)
	}
	store := config.NewStore(cfgPath)

	settings, err := store.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	reg := stats.NewRegistry()
	sup := supervisor.New(store, reg)

	services := app.NewServices(store, sup, reg)
	stopStats := sup.StartStatsTicker(time.Second)
	var trayUpdates nativeUpdateGate
	shutdown := makeShutdownHandler(services, stopStats, trayUpdates.Close)
	defer shutdown()

	// ---- Wails 装配 ----
	var window *application.WebviewWindow
	wailsApp := application.New(application.Options{
		Name:        version.Name,
		Description: "SSH port forwarding client",
		// Native termination, especially NSApp terminate on macOS, must
		// release tunnels before the platform exits without Go defers.
		OnShutdown: shutdown,
		Services: []application.Service{
			application.NewService(services.TunnelService()),
			application.NewService(services.HostService()),
			application.NewService(services.SettingsService()),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Linux:   application.LinuxOptions{DisableQuitOnLastWindowClosed: true},
		Windows: application.WindowsOptions{DisableQuitOnLastWindowClosed: true},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.sshnat.desktop",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				if window != nil {
					window.Show().Focus()
				}
			},
		},
	})

	var tray *application.SystemTray
	lastUsed := make(map[string]time.Time)
	var lastUsedMu sync.RWMutex

	recordTunnelUsage := func(id string) {
		lastUsedMu.Lock()
		lastUsed[id] = time.Now()
		lastUsedMu.Unlock()
	}

	buildTrayMenu := func() *application.Menu {
		menu := application.NewMenu()
		menu.Add("显示 SSHNat").OnClick(func(*application.Context) {
			window.Show().Focus()
		})
		menu.AddSeparator()

		cur, err := store.Load()
		if err == nil && len(cur.Tunnels) > 0 {
			lastUsedMu.RLock()
			type tunWithUsage struct {
				tun  config.Tunnel
				used time.Time
			}
			tList := make([]tunWithUsage, len(cur.Tunnels))
			for i, t := range cur.Tunnels {
				tList[i] = tunWithUsage{tun: t, used: lastUsed[t.ID]}
			}
			lastUsedMu.RUnlock()

			sort.SliceStable(tList, func(i, j int) bool {
				return tList[i].used.After(tList[j].used)
			})

			const maxTrayTunnels = 5
			limit := len(tList)
			showMore := false
			if limit > maxTrayTunnels {
				limit = maxTrayTunnels
				showMore = true
			}

			for i := 0; i < limit; i++ {
				t := tList[i].tun
				st := sup.Status(t.ID)
				isRunning := sup.HasActiveRun(t.ID)
				label := trayTunnelLabel(t, st)
				chk := menu.AddCheckbox(label, isRunning)
				tunID := t.ID
				chk.OnClick(func(*application.Context) {
					recordTunnelUsage(tunID)
					go func() {
						if err := toggleTrayTunnel(tunID, sup.HasActiveRun, services.TunnelService().Start, services.TunnelService().Stop); err != nil {
							log.Printf("tray toggle tunnel %s: %v", tunID, err)
							sup.EmitLog(tunID, fmt.Sprintf("托盘操作失败: %v", err))
						}
					}()
				})
			}

			if showMore {
				menu.Add(fmt.Sprintf("更多隧道 (%d条) 请在主界面查看...", len(cur.Tunnels)-maxTrayTunnels)).OnClick(func(*application.Context) {
					window.Show().Focus()
				})
			}
			menu.AddSeparator()
		}

		menu.Add("全部启动").OnClick(func(*application.Context) {
			cur, err := store.Load()
			if err == nil {
				for _, t := range cur.Tunnels {
					recordTunnelUsage(t.ID)
					_ = services.TunnelService().Start(t.ID)
				}
			}
		})
		menu.Add("全部停止").OnClick(func(*application.Context) {
			sup.StopAll()
		})
		menu.AddSeparator()
		menu.Add("打开配置目录").OnClick(func(*application.Context) {
			if err := openConfigDir(filepath.Dir(cfgPath)); err != nil {
				log.Printf("open config directory: %v", err)
			}
		})
		menu.AddSeparator()
		menu.Add("退出").OnClick(func(*application.Context) {
			wailsApp.Quit()
		})
		return menu
	}

	updateTray := func() {
		trayUpdates.Run(func() {
			if tray == nil {
				return
			}
			tray.SetMenu(buildTrayMenu())

			cur, err := store.Load()
			if err == nil {
				runningCount := 0
				for _, t := range cur.Tunnels {
					if sup.HasActiveRun(t.ID) {
						runningCount++
					}
				}
				tray.SetTooltip(fmt.Sprintf("SSHNat (%d/%d 运行中)", runningCount, len(cur.Tunnels)))
			}
		})
	}

	// 事件转发需要 *application.App，装配完成后接上。
	services.SetEmitter(func(name string, data ...any) {
		wailsApp.Event.Emit(name, data...)
		if !serverMode && (name == app.EventTunnelStatus || name == app.EventConfigChanged) && !trayUpdates.Closed() {
			application.InvokeAsync(updateTray)
		}
	})

	window = wailsApp.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            fmt.Sprintf("%s v%s", version.Name, version.Current()),
		Width:            1280,
		Height:           800,
		MinWidth:         960,
		MinHeight:        600,
		BackgroundColour: application.NewRGB(0x0D, 0x11, 0x17),
		URL:              "/",
	})
	services.SetExportChooser(func() (string, error) {
		path, err := wailsApp.Dialog.SaveFile().SetFilename("sshnat-backup.json").
			AddFilter("JSON configuration", "*.json").AttachToWindow(window).PromptForSingleSelection()
		return normalizeNativeSaveResult(path, err)
	})

	// Closing the window hides it instead of destroying the Wails application.
	// The tray Quit action is the explicit path that stops tunnels and exits.
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		window.Hide()
	})

	tray = wailsApp.SystemTray.New()
	tray.SetIcon(appIcon)
	tray.SetLabel("SSHNat")
	tray.SetTooltip("SSHNat")
	tray.AttachWindow(window).SetMenu(buildTrayMenu()).OnClick(func() { window.Show().Focus() }).Run()
	updateTray()
	// InvokeAsync needs an initialized platform application. Delay autostart
	// until startup, after the window/tray and native dispatcher exist.
	autostart := func() {
		for _, tun := range settings.Tunnels {
			if trayUpdates.Closed() {
				return
			}
			if tun.AutoStart {
				recordTunnelUsage(tun.ID)
				if err := services.TunnelService().Start(tun.ID); err != nil {
					log.Printf("autostart tunnel %s: %v", tun.ID, err)
				}
			}
		}
	}
	if serverMode {
		// Server mode has no native startup event or tray dispatcher.
		autostart()
	} else {
		wailsApp.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) { autostart() })
	}

	if err := wailsApp.Run(); err != nil {
		// log.Fatal exits immediately and skips the fallback shutdown defer.
		shutdown()
		log.Fatal(err)
	}
}

// openConfigDir opens the directory containing the active configuration with
// the platform's default file manager. Keeping this behind a small helper
// avoids invoking the Windows-only explorer command on Unix and macOS builds.
func openConfigDir(dir string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", dir).Start()
	case "darwin":
		return exec.Command("open", dir).Start()
	default:
		return exec.Command("xdg-open", dir).Start()
	}
}
