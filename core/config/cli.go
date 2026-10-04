package config

import (
	"fmt"
	"net"
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

	HostKeyPolicy         string
	ConnectTimeoutSeconds int
	KeepaliveSeconds      int
	KnownHostsFile        string
	AgentSocket           string
	options               map[string]bool // first explicit -o value wins, as in OpenSSH

	Forwards []ForwardSpec
}

// ForwardSpec 是单个 -L/-R/-D 转发声明。
type ForwardSpec struct {
	Mode string // "L" | "R" | "D"

	BindHost     string // 监听绑定地址（可空 = localhost）
	BindPort     int    // 监听端口
	Socket       string // 监听 unix socket（OpenSSH 语法 socket:target）
	TargetSocket string // 目标 unix socket（与 Socket 配对时使用）

	TargetHost string // 目标主机
	TargetPort int    // 目标端口
}

// ParseSSHCommand 解析形如：
//
//	ssh [-p port] [-i key] [-J jump,...] [-L spec]... [-R spec]... [-D port] [user@]host
//
// 的参数列表（不含 argv[0]）。支持 -N/-v 等无参开关的忽略。
func ParseSSHCommand(args []string) (*SSHSpec, error) {
	spec := &SSHSpec{options: make(map[string]bool)}
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
		// OpenSSH accepts both "-p 2222" and compact forms such as
		// "-p2222" / "-L8080:db:3306". Normalize the latter before
		// dispatching so an attached value is not mistaken for an unknown
		// flag (which previously caused the port/forward to be silently lost).
		attached := ""
		if flag, value, ok := splitAttachedOption(arg); ok {
			arg, attached = flag, value
		}
		switch {
		case valueFlags[arg]:
			val := attached
			if val == "" {
				if i+1 >= len(args) {
					return nil, fmt.Errorf("config: flag %s requires a value", arg)
				}
				val = args[i+1]
				i += 2
			} else {
				i++
			}
			if val == "" {
				return nil, fmt.Errorf("config: flag %s requires a value", arg)
			}
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
			case "-o":
				if err := spec.parseOption(val); err != nil {
					return nil, err
				}
			default:
				// -F cannot be resolved without reading a separate SSH config.
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
	if spec.KeyPath != "" && spec.options["identityagent"] {
		return nil, fmt.Errorf("config: combining -i and IdentityAgent is unsupported; choose key or agent authentication")
	}
	return spec, nil
}

func (spec *SSHSpec) parseOption(raw string) error {
	raw = strings.TrimSpace(raw)
	end := strings.IndexAny(raw, "= \t")
	name, setting := raw, ""
	if end >= 0 {
		name = raw[:end]
		setting = strings.TrimSpace(raw[end:])
		setting = strings.TrimSpace(strings.TrimPrefix(setting, "="))
	}
	name = strings.ToLower(name)
	switch name {
	case "stricthostkeychecking", "connecttimeout", "serveraliveinterval", "userknownhostsfile", "identityagent":
	default:
		return nil // Other OpenSSH options do not have a corresponding app setting.
	}
	if spec.options[name] {
		return nil
	}
	setting, err := sshOptionValue(setting)
	if err != nil {
		return fmt.Errorf("config: invalid %s: %w", name, err)
	}
	switch name {
	case "stricthostkeychecking":
		switch strings.ToLower(setting) {
		case "yes", "true":
			spec.HostKeyPolicy = HostKeyStrict
		case "accept-new":
			spec.HostKeyPolicy = HostKeyAcceptNew
		default:
			return fmt.Errorf("config: StrictHostKeyChecking must be yes or accept-new; insecure or interactive policies are unsupported")
		}
	case "connecttimeout":
		seconds, err := strconv.Atoi(setting)
		if err != nil || seconds < 1 || seconds > 300 {
			return fmt.Errorf("config: ConnectTimeout must be between 1 and 300 seconds")
		}
		spec.ConnectTimeoutSeconds = seconds
	case "serveraliveinterval":
		seconds, err := strconv.Atoi(setting)
		if err != nil || seconds < 0 || seconds > 86400 {
			return fmt.Errorf("config: ServerAliveInterval must be between 0 and 86400 seconds")
		}
		if seconds == 0 {
			seconds = -1 // The app's zero means its 15-second default.
		}
		spec.KeepaliveSeconds = seconds
	case "userknownhostsfile":
		if strings.EqualFold(setting, "none") {
			return fmt.Errorf("config: UserKnownHostsFile=none is unsupported")
		}
		spec.KnownHostsFile = setting
	case "identityagent":
		if strings.EqualFold(setting, "none") {
			return fmt.Errorf("config: IdentityAgent=none is unsupported")
		}
		if setting == "SSH_AUTH_SOCK" {
			setting = "" // Use the system's default agent, as OpenSSH does.
		}
		spec.AgentSocket = setting
	}
	spec.options[name] = true
	return nil
}

// sshOptionValue reads one OpenSSH config value after shell quoting has been
// removed. Keep ordinary Windows backslashes; only escaped quotes and escaped
// backslashes inside a quoted config value need decoding.
func sshOptionValue(setting string) (string, error) {
	setting = strings.TrimSpace(setting)
	if setting == "" {
		return "", fmt.Errorf("a value is required")
	}
	if setting[0] != '"' && setting[0] != '\'' {
		if strings.ContainsAny(setting, " \t\r\n\x00") {
			return "", fmt.Errorf("only one value is supported; quote a pathname containing spaces")
		}
		return setting, nil
	}
	quote := setting[0]
	var result strings.Builder
	for i := 1; i < len(setting); i++ {
		if setting[i] == quote {
			if strings.TrimSpace(setting[i+1:]) != "" || result.Len() == 0 {
				return "", fmt.Errorf("only one nonempty value is supported")
			}
			return result.String(), nil
		}
		if setting[i] == '\\' && i+1 < len(setting) && (setting[i+1] == quote || setting[i+1] == '\\') {
			i++
		}
		if setting[i] == 0 || setting[i] == '\r' || setting[i] == '\n' {
			return "", fmt.Errorf("a value must not contain NUL or line breaks")
		}
		result.WriteByte(setting[i])
	}
	return "", fmt.Errorf("unterminated quoted value")
}

// ApplyHostOptions applies only options explicitly present in the command so
// importing another tunnel for an existing host preserves omitted settings.
func (spec *SSHSpec) ApplyHostOptions(host *Host) {
	if spec.KeyPath != "" {
		passphrase := ""
		if host.Auth.Method == AuthMethodKey && host.Auth.KeyPath == spec.KeyPath {
			passphrase = host.Auth.KeyPassphrase
		}
		host.Auth = AuthConfig{Method: AuthMethodKey, KeyPath: spec.KeyPath, KeyPassphrase: passphrase}
	}
	if spec.HostKeyPolicy != "" {
		host.HostKeyPolicy = spec.HostKeyPolicy
	}
	if spec.ConnectTimeoutSeconds != 0 {
		host.ConnectTimeoutSeconds = spec.ConnectTimeoutSeconds
	}
	if spec.KeepaliveSeconds != 0 {
		host.KeepaliveSeconds = spec.KeepaliveSeconds
	}
	if spec.KnownHostsFile != "" {
		host.KnownHostsFile = spec.KnownHostsFile
	}
	if spec.options["identityagent"] || spec.AgentSocket != "" {
		host.Auth = AuthConfig{Method: AuthMethodAgent, AgentSocket: spec.AgentSocket}
	}
}

// splitAttachedOption recognizes OpenSSH's compact single-letter value flags.
// It deliberately leaves unknown options untouched so their existing
// best-effort handling remains unchanged.
func splitAttachedOption(arg string) (flag, value string, ok bool) {
	for _, candidate := range []string{"-p", "-l", "-i", "-J", "-L", "-R", "-D", "-F", "-o"} {
		if strings.HasPrefix(arg, candidate) && len(arg) > len(candidate) {
			return candidate, arg[len(candidate):], true
		}
	}
	return "", "", false
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
	// OpenSSH also supports stream-local forwarding with a pair of Unix
	// socket paths (for example, -L /tmp/local.sock:/run/service.sock and
	// -R /tmp/remote.sock:/tmp/local.sock).  The old parser treated these as
	// invalid numeric ports even though the runtime forwarders support them.
	if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
		if port, err := strconv.Atoi(parts[0]); err == nil {
			// TCP listener/remote bind to a Unix target, for example
			// -L 8080:/run/service.sock. OpenSSH accepts this form and the
			// runtime forwarder can dial a server-side stream-local socket.
			if validPort(port) || (mode == 'R' && port == 0) {
				if looksLikeSocketPath(parts[1]) {
					fw.BindPort = port
					fw.TargetSocket = parts[1]
					return fw, nil
				}
			}
		} else {
			fw.Socket = parts[0]
			fw.TargetSocket = parts[1]
			return fw, nil
		}
	}
	// A TCP listener with an explicit bind host can also target a Unix socket:
	// -R 127.0.0.1:9000:/tmp/service.sock.
	if len(parts) == 3 && parts[0] != "" && parts[1] != "" && parts[2] != "" {
		if port, err := strconv.Atoi(parts[1]); err == nil && (validPort(port) || (mode == 'R' && port == 0)) && !looksLikeSocketPath(parts[0]) && looksLikeSocketPath(parts[2]) {
			fw.BindHost = cleanHost(parts[0])
			fw.BindPort = port
			fw.TargetSocket = parts[2]
			return fw, nil
		}
	}
	// A TCP listener may include an explicit bind address before the port
	// while forwarding to a Unix socket, for example:
	// -R 127.0.0.1:9000:/run/service.sock. This is the three-part analogue
	// of the port:socket form handled above.
	if len(parts) == 3 && looksLikeSocketPath(parts[2]) {
		port, err := strconv.Atoi(parts[1])
		if err == nil && (validPort(port) || (mode == 'R' && port == 0)) {
			fw.BindHost = cleanHost(parts[0])
			fw.BindPort = port
			fw.TargetSocket = parts[2]
			return fw, nil
		}
	}
	// A stream-local listener may also forward to a regular TCP target:
	// -L /tmp/local.sock:db.internal:3306 (and the corresponding -R form).
	if len(parts) == 3 && parts[0] != "" {
		if _, err := strconv.Atoi(parts[0]); err != nil && looksLikeSocketPath(parts[0]) {
			port, perr := strconv.Atoi(parts[2])
			if perr != nil || !validPort(port) || cleanHost(parts[1]) == "" {
				return nil, fmt.Errorf("config: invalid -%c %q", mode, val)
			}
			fw.Socket = parts[0]
			fw.TargetHost = cleanHost(parts[1])
			fw.TargetPort = port
			return fw, nil
		}
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

func looksLikeSocketPath(path string) bool {
	path = strings.TrimSpace(path)
	return strings.ContainsAny(path, `/\\`) || strings.HasSuffix(strings.ToLower(path), ".sock")
}

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
	host, _, tunnels, _ := SpecToSettingsWithJumps(spec)
	return host, tunnels
}

// SpecToSettingsWithJumps converts an SSH command into the destination host,
// any ProxyJump hosts, and tunnel entries. Jump hosts inherit the target's
// selected key authentication when -i is present; passwords and agent
// credentials still need to be filled in by the user after import.
func SpecToSettingsWithJumps(spec *SSHSpec) (*Host, []Host, []Tunnel, error) {
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
	spec.ApplyHostOptions(host)

	jumpHosts := make([]Host, 0, len(spec.Jumps))
	for _, raw := range spec.Jumps {
		jump, err := parseJumpHost(raw, spec.KeyPath)
		if err != nil {
			return nil, nil, nil, err
		}
		if jump.User == "" {
			jump.User = spec.User
		}
		host.JumpHostIDs = append(host.JumpHostIDs, jump.ID)
		jumpHosts = append(jumpHosts, *jump)
	}

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
			t.LocalSocket = f.Socket
			t.TargetHost = f.TargetHost
			t.TargetPort = f.TargetPort
			t.TargetSocket = f.TargetSocket
		case TypeRemote:
			t.RemoteBindHost = f.BindHost
			t.RemotePort = f.BindPort
			t.RemoteSocket = f.Socket
			t.TargetHost = f.TargetHost
			t.TargetPort = f.TargetPort
			t.TargetSocket = f.TargetSocket
		case TypeDynamic:
			t.LocalBindHost = f.BindHost
			t.SocksPort = f.BindPort
		}
		tunnels = append(tunnels, t)
	}
	return host, jumpHosts, tunnels, nil
}

func parseJumpHost(raw, keyPath string) (*Host, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("config: empty ProxyJump host")
	}
	user := ""
	address := raw
	if at := strings.LastIndex(address, "@"); at >= 0 {
		user, address = address[:at], address[at+1:]
	}
	if user == "" {
		user = ""
	}
	hostname, port := address, 22
	if h, p, err := net.SplitHostPort(address); err == nil {
		hostname = strings.Trim(h, "[]")
		port, err = strconv.Atoi(p)
		if err != nil || !validPort(port) {
			return nil, fmt.Errorf("config: invalid ProxyJump port %q", p)
		}
	} else if strings.HasPrefix(address, "[") && strings.HasSuffix(address, "]") {
		hostname = strings.Trim(address, "[]")
	} else if strings.HasPrefix(address, "[") && strings.Contains(address, "]") {
		return nil, fmt.Errorf("config: invalid ProxyJump %q", raw)
	} else if strings.Count(address, ":") == 1 {
		parts := strings.SplitN(address, ":", 2)
		hostname = parts[0]
		var err error
		port, err = strconv.Atoi(parts[1])
		if err != nil || !validPort(port) {
			return nil, fmt.Errorf("config: invalid ProxyJump port %q", parts[1])
		}
	}
	if hostname == "" {
		return nil, fmt.Errorf("config: invalid ProxyJump %q", raw)
	}
	auth := AuthConfig{Method: AuthMethodPassword}
	if keyPath != "" {
		auth.Method = AuthMethodKey
		auth.KeyPath = keyPath
	}
	return &Host{ID: NewID("host"), Name: hostname, Host: hostname, Port: port, User: user, Auth: auth}, nil
}

func forwardName(f *ForwardSpec) string {
	switch f.Mode {
	case TypeLocal:
		if f.Socket != "" {
			if f.TargetSocket != "" {
				return fmt.Sprintf("%s → %s", f.Socket, f.TargetSocket)
			}
			return fmt.Sprintf("%s → %s:%d", f.Socket, f.TargetHost, f.TargetPort)
		}
		return fmt.Sprintf("%d → %s:%d", f.BindPort, f.TargetHost, f.TargetPort)
	case TypeRemote:
		if f.Socket != "" {
			if f.TargetSocket != "" {
				return fmt.Sprintf("%s ← %s", f.Socket, f.TargetSocket)
			}
			return fmt.Sprintf("%s ← %s:%d", f.Socket, f.TargetHost, f.TargetPort)
		}
		return fmt.Sprintf("%d ← %s:%d", f.BindPort, f.TargetHost, f.TargetPort)
	default:
		return fmt.Sprintf("SOCKS :%d", f.BindPort)
	}
}
