package config

import (
	"fmt"
	"strconv"
	"strings"
)

// SSHSpec 是从 `ssh ...` 命令行解析出的结果。
type SSHSpec struct {
	User     string   // user@host 中的 user，可为空
	Hostname string   // 目标主机
	Port     int      // -p，0 = 默认 22
	KeyPath  string   // -i
	Jumps    []string // -J user@host:port,逗号分隔多级

	Forwards []ForwardSpec
}

// ForwardSpec 是单个 -L/-R/-D 转发声明。
type ForwardSpec struct {
	Mode string // "L" | "R" | "D"

	BindHost string // 监听绑定地址（可空 = localhost）
	BindPort int    // 监听端口
	Socket   string // 监听 unix socket（OpenSSH 语法 socket:target）

	TargetHost string // 目标主机
	TargetPort int    // 目标端口
}

// ParseSSHCommand 解析形如：
//
//	ssh [-p port] [-i key] [-J jump,...] [-L spec]... [-R spec]... [-D port] [user@]host
//
// 的参数列表（不含 argv[0]）。支持 -N/-v 等无参开关的忽略。
func ParseSSHCommand(args []string) (*SSHSpec, error) {
	spec := &SSHSpec{}
	if len(args) > 0 && isSSHProgram(args[0]) {
		args = args[1:]
	}

	valueFlags := map[string]bool{
		"-p": true, "-l": true, "-i": true, "-J": true,
		"-L": true, "-R": true, "-D": true, "-F": true, "-o": true,
	}
	boolIgnore := map[string]bool{
		"-n": true, "-N": true, "-f": true, "-g": true, "-T": true,
		"-v": true, "-q": true, "-C": true, "-4": true, "-6": true,
		"-A": true, "-a": true, "-G": true, "-K": true, "-k": true,
		"-t": true, "-V": true, "-x": true, "-X": true, "-Y": true,
	}

	i := 0
	for i < len(args) {
		arg := args[i]
		switch {
		case valueFlags[arg]:
			if i+1 >= len(args) {
				return nil, fmt.Errorf("config: flag %s requires a value", arg)
			}
			val := args[i+1]
			i += 2
			var err error
			switch arg {
			case "-p":
				spec.Port, err = strconv.Atoi(val)
				if err != nil || spec.Port <= 0 || spec.Port > 65535 {
					return nil, fmt.Errorf("config: invalid -p port %q", val)
				}
			case "-l":
				spec.User = val
			case "-i":
				spec.KeyPath = val
			case "-J":
				for _, j := range strings.Split(val, ",") {
					j = strings.TrimSpace(j)
					if j != "" {
						spec.Jumps = append(spec.Jumps, j)
					}
				}
			case "-L", "-R", "-D":
				fw, perr := parseForward(arg[1], val)
				if perr != nil {
					return nil, perr
				}
				spec.Forwards = append(spec.Forwards, *fw)
			default:
				// -F / -o：本期忽略。
			}
		case boolIgnore[arg]:
			i++
		case strings.HasPrefix(arg, "-") && len(arg) > 1 && !valueFlags[arg]:
			// 联合短开关如 -vN 或未知带值形式，尽力忽略。
			i++
		default:
			// 位置参数：目标主机（可能带 user@），仅在未设置时提取。
			if spec.Hostname == "" {
				target := arg
				if at := strings.LastIndex(target, "@"); at >= 0 {
					spec.User = target[:at]
					target = target[at+1:]
				}
				spec.Hostname = target
			}
			i++
		}
	}

	if spec.Hostname == "" {
		return nil, fmt.Errorf("config: no destination host found")
	}
	return spec, nil
}

// splitBracketed 按冒号切分，但保留方括号 [...] 内的冒号（支持 IPv6）。
func splitBracketed(s string) []string {
	var parts []string
	var cur strings.Builder
	inBracket := false
	for _, r := range s {
		switch r {
		case '[':
			inBracket = true
		case ']':
			inBracket = false
		case ':':
			if !inBracket {
				parts = append(parts, cur.String())
				cur.Reset()
				continue
			}
		}
		cur.WriteRune(r)
	}
	parts = append(parts, cur.String())
	return parts
}

func cleanHost(s string) string {
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	return s
}

// parseForward 解析单个转发规格。
// -L/-R: [bind_address:]port:host:hostport | [bind_address:]socket:...
// -D:    [bind_address:]port
func parseForward(mode byte, val string) (*ForwardSpec, error) {
	parts := splitBracketed(val)
	fw := &ForwardSpec{Mode: string(mode)}

	switch mode {
	case 'D':
		switch len(parts) {
		case 1:
			port, err := strconv.Atoi(parts[0])
			if err != nil || !validPort(port) {
				return nil, fmt.Errorf("config: invalid -D %q", val)
			}
			fw.BindPort = port
		case 2:
			fw.BindHost = cleanHost(parts[0])
			port, err := strconv.Atoi(parts[1])
			if err != nil || !validPort(port) {
				return nil, fmt.Errorf("config: invalid -D %q", val)
			}
			fw.BindPort = port
		default:
			return nil, fmt.Errorf("config: invalid -D %q", val)
		}
		return fw, nil
	}

	isPortAllowed := func(port int) bool {
		if mode == 'R' && port == 0 {
			return true // -R 0:host:port 支持动态服务端端口分配
		}
		return validPort(port)
	}

	switch len(parts) {
	case 3: // port:host:port
		port, err1 := strconv.Atoi(parts[0])
		tport, err2 := strconv.Atoi(parts[2])
		targetHost := cleanHost(parts[1])
		if err1 != nil || err2 != nil || !isPortAllowed(port) || !validPort(tport) || targetHost == "" {
			return nil, fmt.Errorf("config: invalid -%c %q", mode, val)
		}
		fw.BindPort, fw.TargetHost, fw.TargetPort = port, targetHost, tport
	case 4: // bind:port:host:port
		port, err1 := strconv.Atoi(parts[1])
		tport, err2 := strconv.Atoi(parts[3])
		bindHost := cleanHost(parts[0])
		targetHost := cleanHost(parts[2])
		if err1 != nil || err2 != nil || !isPortAllowed(port) || !validPort(tport) || targetHost == "" {
			return nil, fmt.Errorf("config: invalid -%c %q", mode, val)
		}
		fw.BindHost, fw.BindPort, fw.TargetHost, fw.TargetPort = bindHost, port, targetHost, tport
	default:
		return nil, fmt.Errorf("config: invalid -%c %q", mode, val)
	}
	return fw, nil
}

func validPort(port int) bool { return port > 0 && port <= 65535 }

func isSSHProgram(arg string) bool {
	arg = strings.ToLower(arg)
	if i := strings.LastIndexAny(arg, `/\\`); i >= 0 {
		arg = arg[i+1:]
	}
	return arg == "ssh" || arg == "ssh.exe"
}

// SpecToSettings 把解析结果转换为配置条目。
// 返回一个 Host 与若干 Tunnel（已建立 ID 并互相引用）。
func SpecToSettings(spec *SSHSpec) (*Host, []Tunnel) {
	host := &Host{
		ID:   NewID("host"),
		Name: spec.Hostname,
		Host: spec.Hostname,
		Port: spec.Port,
		User: spec.User,
		Auth: AuthConfig{Method: AuthMethodPassword},
	}
	if host.Port == 0 {
		host.Port = 22
	}
	if spec.KeyPath != "" {
		host.Auth.Method = AuthMethodKey
		host.Auth.KeyPath = spec.KeyPath
	}
	// 注意：-J 跳板仅保留在 spec.Jumps 原始串中，
	// 需要用户补全跳板凭据后手动创建 Host 并关联。
	_ = spec.Jumps

	var tunnels []Tunnel
	for _, f := range spec.Forwards {
		t := Tunnel{
			ID:     NewID("tnl"),
			Name:   forwardName(&f),
			HostID: host.ID,
			Type:   f.Mode,
		}
		switch f.Mode {
		case TypeLocal:
			t.LocalBindHost = f.BindHost
			t.LocalPort = f.BindPort
			t.TargetHost = f.TargetHost
			t.TargetPort = f.TargetPort
		case TypeRemote:
			t.RemoteBindHost = f.BindHost
			t.RemotePort = f.BindPort
			t.TargetHost = f.TargetHost
			t.TargetPort = f.TargetPort
		case TypeDynamic:
			t.LocalBindHost = f.BindHost
			t.SocksPort = f.BindPort
		}
		tunnels = append(tunnels, t)
	}
	return host, tunnels
}

func forwardName(f *ForwardSpec) string {
	switch f.Mode {
	case TypeLocal:
		return fmt.Sprintf("%d → %s:%d", f.BindPort, f.TargetHost, f.TargetPort)
	case TypeRemote:
		return fmt.Sprintf("%d ← %s:%d", f.BindPort, f.TargetHost, f.TargetPort)
	default:
		return fmt.Sprintf("SOCKS :%d", f.BindPort)
	}
}
