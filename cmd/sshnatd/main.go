// sshnatd 是 SSHNat 的 headless CLI 守护进程。
// 它只依赖 core/，用于验证核心库与 UI 层完全解耦：
//
//	sshnatd -config ./config.json -all
//	sshnatd -config ./config.json -tunnel tnl-1
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/sshnat/sshnat/core/config"
	"github.com/sshnat/sshnat/core/stats"
	"github.com/sshnat/sshnat/core/supervisor"
	"github.com/sshnat/sshnat/internal/version"
)

func main() {
	cfgPath := flag.String("config", "", "配置文件路径（默认便携目录或系统用户配置目录）")
	tunnelIDs := flag.String("tunnel", "", "要启动的隧道 ID，逗号分隔")
	startAll := flag.Bool("all", false, "启动配置中的所有隧道")
	autoStart := flag.Bool("auto", true, "启动标记了 autoStart 的隧道")
	createFrom := flag.String("from-ssh-cmd", "", "解析 `ssh -L ... user@host` 命令并写入配置后退出")
	listenAddrHint := flag.String("listen", "", "提示信息用：期望的本地监听地址（不影响行为）")
	statsEvery := flag.Duration("stats-interval", time.Second, "流量统计事件间隔")
	showVersion := flag.Bool("version", false, "打印版本并退出")
	checkConfig := flag.Bool("check", false, "仅校验配置后退出（不联网或启动隧道）")
	flag.Parse()
	if *showVersion {
		fmt.Printf("%s %s\n", version.Name, version.Current())
		return
	}
	if flag.NArg() != 0 {
		log.Fatalf("unexpected positional arguments: %s", strings.Join(flag.Args(), " "))
	}
	if *startAll && strings.TrimSpace(*tunnelIDs) != "" {
		log.Fatal("-all and -tunnel cannot be used together")
	}
	if *checkConfig && *createFrom != "" {
		log.Fatal("-check and -from-ssh-cmd cannot be used together")
	}

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
		args, splitErr := splitShellish(*createFrom)
		if splitErr != nil {
			log.Fatalf("split ssh command: %v", splitErr)
		}
		spec, err := config.ParseSSHCommand(args)
		if err != nil {
			log.Fatalf("parse ssh command: %v", err)
		}
		// OpenSSH uses the local account name when user@ or -l is omitted.
		// Preserve that behavior so imported configs pass structural validation.
		if spec.User == "" {
			current, err := user.Current()
			if err != nil {
				log.Fatalf("resolve SSH user (or specify user@host): %v", err)
			}
			spec.User = current.Username
			if i := strings.LastIndexAny(spec.User, `/\\`); i >= 0 {
				spec.User = spec.User[i+1:]
			}
		}
		host, jumpHosts, tunnels, err := config.SpecToSettingsWithJumps(spec)
		if err != nil {
			log.Fatalf("convert ssh command: %v", err)
		}
		st, err := readDaemonSettings(path)
		if errors.Is(err, os.ErrNotExist) {
			st = &config.Settings{Version: 1}
		} else if err != nil {
			log.Fatalf("load config: %v", err)
		}
		jumpIDs := make([]string, 0, len(jumpHosts))
		for i := range jumpHosts {
			jump := jumpHosts[i]
			if jump.Host == host.Host && jump.Port == host.Port && jump.User == host.User {
				log.Fatalf("ProxyJump chain cannot reference destination host")
			}
			if existing, ok := findHostByAddr(st, jump.Host, jump.Port, jump.User); ok {
				jumpIDs = append(jumpIDs, existing.ID)
			} else {
				st.Hosts = append(st.Hosts, jump)
				jumpIDs = append(jumpIDs, jump.ID)
			}
		}
		host.JumpHostIDs = jumpIDs
		if existing, ok := findHostByAddr(st, host.Host, host.Port, host.User); ok {
			*host = existing
			if len(jumpIDs) > 0 {
				host.JumpHostIDs = jumpIDs
				for i := range st.Hosts {
					if st.Hosts[i].ID == host.ID {
						st.Hosts[i].JumpHostIDs = jumpIDs
						break
					}
				}
			}
		} else {
			st.Hosts = append(st.Hosts, *host)
		}
		for i := range tunnels {
			tunnels[i].HostID = host.ID
			st.Tunnels = append(st.Tunnels, tunnels[i])
		}
		if err := config.Validate(st); err != nil {
			log.Fatalf("validate imported config: %v", err)
		}
		if err := store.Save(st); err != nil {
			log.Fatalf("save config: %v", err)
		}
		fmt.Printf("created host %s (%s@%s:%d) and %d tunnel(s):\n", host.ID, host.User, host.Host, host.Port, len(tunnels))
		for _, t := range tunnels {
			fmt.Printf("  %s  %s\n", t.ID, describeTunnel(t))
		}
		return
	}

	settings, err := readDaemonSettings(path)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if *checkConfig {
		fmt.Printf("config valid: %d host(s), %d tunnel(s)\n", len(settings.Hosts), len(settings.Tunnels))
		return
	}
	toStart, err := selectTunnels(settings, *startAll, *tunnelIDs, *autoStart)
	if err != nil {
		log.Fatalf("select tunnels: %v", err)
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

	started := 0
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
		started++
		log.Printf("started tunnel %s (%s)", id, describeTunnel(*tun))
	}
	if started == 0 {
		log.Fatal("no tunnel could be started; check configured host references")
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

// readDaemonSettings validates a complete, bounded configuration document.
// Missing config is an operational error rather than desktop first-run setup.
func readDaemonSettings(path string) (*config.Settings, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, config.MaxConfigBytes+1))
	if err != nil {
		return nil, err
	}
	return config.ParseSettings(data)
}

// selectTunnels resolves the complete startup set before any side effects.
// Explicit IDs are all-or-nothing, and duplicate IDs only start one loop.
func selectTunnels(settings *config.Settings, all bool, requested string, auto bool) ([]string, error) {
	ids := []string{}
	seen := map[string]bool{}
	add := func(id string) error {
		id = strings.TrimSpace(id)
		if id == "" {
			return errors.New("empty tunnel ID in -tunnel list")
		}
		if _, found := settings.FindTunnel(id); !found {
			return fmt.Errorf("tunnel %s not found", id)
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
		return nil
	}
	if all && strings.TrimSpace(requested) != "" {
		return nil, errors.New("-all and -tunnel cannot be used together")
	}
	if strings.TrimSpace(requested) != "" {
		for _, id := range strings.Split(requested, ",") {
			if err := add(id); err != nil {
				return nil, err
			}
		}
	} else {
		for _, tunnel := range settings.Tunnels {
			if all || (auto && tunnel.AutoStart) {
				if err := add(tunnel.ID); err != nil {
					return nil, err
				}
			}
		}
	}
	if len(ids) == 0 {
		return nil, errors.New("no tunnels to start (use -all, -tunnel <id>, or configure autoStart)")
	}
	return ids, nil
}

// splitShellish 极简 shell 分词：支持双引号。仅用于 --from-ssh-cmd 输入。
func splitShellish(s string) ([]string, error) {
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
	if inQuote {
		return nil, errors.New("unbalanced quote in command")
	}
	return out, nil
}

func describeTunnel(t config.Tunnel) string {
	target := t.TargetSocket
	if target == "" {
		target = net.JoinHostPort(bindHost(t.TargetHost), strconv.Itoa(t.TargetPort))
	}
	switch t.Type {
	case config.TypeLocal:
		listener := net.JoinHostPort(bindHost(t.LocalBindHost), strconv.Itoa(t.LocalPort))
		if t.LocalSocket != "" {
			listener = t.LocalSocket
		}
		return fmt.Sprintf("-L %s -> %s", listener, target)
	case config.TypeRemote:
		listener := net.JoinHostPort(bindHost(t.RemoteBindHost), strconv.Itoa(t.RemotePort))
		if t.RemoteSocket != "" {
			listener = t.RemoteSocket
		}
		return fmt.Sprintf("-R %s <- %s", listener, target)
	case config.TypeDynamic:
		return fmt.Sprintf("-D socks5://%s", net.JoinHostPort(bindHost(t.LocalBindHost), strconv.Itoa(t.SocksPort)))
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

func findHostByAddr(settings *config.Settings, host string, port int, user string) (config.Host, bool) {
	for _, h := range settings.Hosts {
		if h.Host == host && (h.Port == port || (port == 22 && h.Port == 0)) && h.User == user {
			return h, true
		}
	}
	return config.Host{}, false
}
