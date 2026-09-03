// sshnatd 是 SSHNat 的 headless CLI 守护进程。
// 它只依赖 core/，用于验证核心库与 UI 层完全解耦：
//
//	sshnatd -config ./config.json -all
//	sshnatd -config ./config.json -tunnel tnl-1 -once
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/sshnat/sshnat/core/config"
	"github.com/sshnat/sshnat/core/stats"
	"github.com/sshnat/sshnat/core/supervisor"
)

func main() {
	cfgPath := flag.String("config", "", "配置文件路径（默认 ~/.config/sshnat/config.json）")
	tunnelIDs := flag.String("tunnel", "", "要启动的隧道 ID，逗号分隔")
	startAll := flag.Bool("all", false, "启动配置中的所有隧道")
	autoStart := flag.Bool("auto", true, "启动标记了 autoStart 的隧道")
	createFrom := flag.String("from-ssh-cmd", "", "解析 `ssh -L ... user@host` 命令并写入配置后退出")
	listenAddrHint := flag.String("listen", "", "提示信息用：期望的本地监听地址（不影响行为）")
	statsEvery := flag.Duration("stats-interval", time.Second, "流量统计事件间隔")
	flag.Parse()

	if *listenAddrHint != "" {
		log.Printf("listen hint: %s", *listenAddrHint)
	}

	path := *cfgPath
	if path == "" {
		p, err := config.DefaultPath()
		if err != nil {
			log.Fatalf("resolve default config path: %v", err)
		}
		path = p
	}
	store := config.NewStore(path)

	// --from-ssh-cmd：粘贴 OpenSSH 命令行生成配置（演示 config 解析器能力）。
	if *createFrom != "" {
		args := splitShellish(*createFrom)
		spec, err := config.ParseSSHCommand(args)
		if err != nil {
			log.Fatalf("parse ssh command: %v", err)
		}
		host, tunnels := config.SpecToSettings(spec)
		st, err := store.Load()
		if err != nil {
			log.Fatalf("load config: %v", err)
		}
		st.Hosts = append(st.Hosts, *host)
		st.Tunnels = append(st.Tunnels, tunnels...)
		if err := store.Save(st); err != nil {
			log.Fatalf("save config: %v", err)
		}
		fmt.Printf("created host %s (%s@%s:%d) and %d tunnel(s):\n", host.ID, host.User, host.Host, host.Port, len(tunnels))
		for _, t := range tunnels {
			fmt.Printf("  %s  %s\n", t.ID, describeTunnel(t))
		}
		return
	}

	settings, err := store.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	reg := stats.NewRegistry()
	sup := supervisor.New(store, reg)

	// 订阅事件并打印到 stdout。
	evCh, cancel := sup.Subscribe()
	defer cancel()
	go func() {
		for ev := range evCh {
			switch ev.Type {
			case supervisor.EventStatus:
				line := fmt.Sprintf("[status] tunnel %s: %s -> %s", ev.TunnelID, ev.Previous, ev.Status)
				if ev.Error != "" {
					line += fmt.Sprintf(" (%s)", ev.Error)
				}
				if ev.Attempt > 0 {
					line += fmt.Sprintf(" attempt=%d retry-in=%dms", ev.Attempt, ev.NextInMs)
				}
				log.Print(line)
			case supervisor.EventStats:
				log.Printf("[stats] tunnel %s tx=%dB rx=%dB conns=%d total-conns=%d",
					ev.TunnelID, ev.Tx, ev.Rx, ev.Conns, ev.TotalConns)
			case supervisor.EventLog:
				log.Printf("[log] tunnel %s: %s", ev.TunnelID, ev.Message)
			}
		}
	}()

	stopStats := sup.StartStatsTicker(*statsEvery)
	defer stopStats()

	// 决定要启动的隧道集合。
	var toStart []string
	switch {
	case *startAll:
		for _, t := range settings.Tunnels {
			toStart = append(toStart, t.ID)
		}
	case strings.TrimSpace(*tunnelIDs) != "":
		for _, id := range strings.Split(*tunnelIDs, ",") {
			toStart = append(toStart, strings.TrimSpace(id))
		}
	default:
		if *autoStart {
			for _, t := range settings.Tunnels {
				if t.AutoStart {
					toStart = append(toStart, t.ID)
				}
			}
		}
	}

	if len(toStart) == 0 {
		log.Print("warning: no tunnels to start (use -all, -tunnel <id>, or configure autoStart in config)")
	}

	for _, id := range toStart {
		tun, ok := settings.FindTunnel(id)
		if !ok {
			log.Printf("tunnel %s not found, skipping", id)
			continue
		}
		if err := sup.Start(id); err != nil {
			log.Printf("start tunnel %s (%s): %v", id, describeTunnel(*tun), err)
			continue
		}
		log.Printf("started tunnel %s (%s)", id, describeTunnel(*tun))
	}

	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	log.Printf("sshnatd running (config=%s). Ctrl+C to stop.", path)
	<-sig

	log.Print("shutting down (press Ctrl+C again to force exit)...")
	go func() {
		<-sig
		log.Print("force exit requested.")
		os.Exit(1)
	}()

	sup.StopAllWithTimeout(10 * time.Second)
}

// splitShellish 极简 shell 分词：支持双引号。仅用于 --from-ssh-cmd 输入。
func splitShellish(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	var quote rune
	for _, r := range s {
		switch {
		case (r == '"' || r == '\'') && !inQuote:
			inQuote = !inQuote
			quote = r
		case r == quote && inQuote:
			inQuote = false
			quote = 0
		case unicode.IsSpace(r) && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func describeTunnel(t config.Tunnel) string {
	switch t.Type {
	case config.TypeLocal:
		return fmt.Sprintf("-L %s:%d -> %s:%d", bindHost(t.LocalBindHost), t.LocalPort, t.TargetHost, t.TargetPort)
	case config.TypeRemote:
		return fmt.Sprintf("-R :%d <- %s:%d", t.RemotePort, t.TargetHost, t.TargetPort)
	case config.TypeDynamic:
		return fmt.Sprintf("-D socks5://%s:%d", bindHost(t.LocalBindHost), t.SocksPort)
	default:
		return t.Type
	}
}

func bindHost(h string) string {
	if h == "" {
		return "127.0.0.1"
	}
	return h
}
