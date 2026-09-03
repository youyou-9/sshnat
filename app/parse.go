package app

import (
	"strings"
)

// splitCommand 把用户粘贴的 ssh 命令拆成参数列表。
// 支持单双引号与多余空白。
func splitCommand(cmd string) ([]string, error) {
	var args []string
	var cur strings.Builder
	var quote rune

	flush := func() {
		if cur.Len() > 0 {
			args = append(args, cur.String())
			cur.Reset()
		}
	}

	for _, r := range strings.TrimSpace(cmd) {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
		case r == ' ' || r == '\t' || r == '\n':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()

	if quote != 0 {
		return nil, errUnbalancedQuote
	}

	// 去掉开头的 "ssh" 与可能的 "ssh.exe"/完整路径。
	if len(args) > 0 {
		base := args[0]
		if i := strings.LastIndexAny(base, `/\`); i >= 0 {
			base = base[i+1:]
		}
		base = strings.TrimSuffix(base, ".exe")
		if strings.EqualFold(base, "ssh") {
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
)
