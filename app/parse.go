package app

import (
	"fmt"
	"runtime"
	"strings"
	"unicode"
)

// splitCommand keeps the existing RPC's platform-specific default. Callers
// importing a command copied for another shell should use splitCommandForShell.
func splitCommand(cmd string) ([]string, error) {
	return splitCommandForShell(cmd, "")
}

// splitCommandForShell only tokenizes arguments; it never executes a shell,
// expands variables, or evaluates command substitution. Explicit shell names
// avoid the ambiguity of 'a”b' (POSIX: ab; PowerShell: a'b).
func splitCommandForShell(cmd, shell string) ([]string, error) {
	shell = strings.ToLower(strings.TrimSpace(shell))
	if shell == "" {
		if runtime.GOOS == "windows" {
			shell = "powershell"
		} else {
			shell = "posix"
		}
	}
	if shell != "posix" && shell != "powershell" {
		return nil, fmt.Errorf("unsupported command shell %q", shell)
	}

	var args []string
	var cur strings.Builder
	var quote rune
	started := false
	flush := func() {
		if started {
			args = append(args, cur.String())
			cur.Reset()
			started = false
		}
	}
	runes := []rune(strings.TrimSpace(cmd))
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if quote == '\'' {
			if r == '\'' {
				if shell == "powershell" && i+1 < len(runes) && runes[i+1] == '\'' {
					cur.WriteRune('\'')
					i++
				} else {
					quote = 0
				}
			} else {
				cur.WriteRune(r)
			}
			continue
		}
		if quote == '"' && r == '"' {
			quote = 0
			continue
		}
		if shell == "posix" && r == '\\' {
			if i+1 == len(runes) {
				return nil, errDanglingEscape
			}
			next := runes[i+1]
			if quote == 0 || strings.ContainsRune("\\\"$`\n\r", next) {
				i++
				if next == '\n' || next == '\r' {
					if next == '\r' && i+1 < len(runes) && runes[i+1] == '\n' {
						i++
					}
					continue
				}
				cur.WriteRune(next)
				started = true
				continue
			}
		}
		if shell == "powershell" && r == '`' {
			if i+1 == len(runes) {
				return nil, errDanglingEscape
			}
			i++
			next := runes[i]
			if next == '\n' || next == '\r' {
				if next == '\r' && i+1 < len(runes) && runes[i+1] == '\n' {
					i++
				}
				continue
			}
			cur.WriteRune(next)
			started = true
			continue
		}
		if quote != 0 {
			cur.WriteRune(r)
			continue
		}
		switch {
		case r == '"' || r == '\'':
			quote = r
			started = true
		case unicode.IsSpace(r):
			flush()
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if quote != 0 {
		return nil, errUnbalancedQuote
	}
	flush()

	// 去掉开头的 "ssh" 与可能的 "ssh.exe"/完整路径。
	if len(args) > 0 {
		base := args[0]
		if i := strings.LastIndexAny(base, `/\`); i >= 0 {
			base = base[i+1:]
		}
		if strings.EqualFold(base, "ssh") || strings.EqualFold(base, "ssh.exe") {
			args = args[1:]
		}
	}
	if len(args) == 0 {
		return nil, errEmptyCommand
	}
	return args, nil
}

type sentinelError string

func (e sentinelError) Error() string { return string(e) }

const (
	errUnbalancedQuote = sentinelError("unbalanced quote in command")
	errEmptyCommand    = sentinelError("empty command")
	errDanglingEscape  = sentinelError("dangling escape in command")
)
