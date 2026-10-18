// Package update 检查 GitHub Releases 上的最新版本。
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Repo 是发布版本来源（GitHub owner/repo）。
const Repo = "youyou-9/sshnat"

// Reason 标记检查结果的语义，供前端本地化展示（避免用英文文案做判据）。
type Reason string

const (
	ReasonUpToDate   Reason = "up-to-date"
	ReasonAvailable  Reason = "available"
	ReasonNoRelease  Reason = "no-release"
	ReasonDevBuild   Reason = "dev-build"
	ReasonUnparsable Reason = "unparsable"
)

// Info 是一次更新检查的结果。
type Info struct {
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"`
	URL       string `json:"url,omitempty"`
	HasUpdate bool   `json:"hasUpdate"`
	// Reason 是结果分类，前端据此选择本地化文案。
	Reason Reason `json:"reason,omitempty"`
	// Message 为面向用户的英文补充说明（日志/调试用）。
	Message string `json:"message,omitempty"`
}

// Check 查询最新 release 并与 current 比较。网络/解析失败返回 error。
func Check(ctx context.Context, current string) (*Info, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", Repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sshnat-update-check")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query latest release: %w", err)
	}
	defer resp.Body.Close()

	info := &Info{Current: normalize(current)}
	if resp.StatusCode == http.StatusNotFound {
		info.Reason = ReasonNoRelease
		info.Message = "no published releases yet"
		return info, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query latest release: unexpected status %s", resp.Status)
	}

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("parse release response: %w", err)
	}
	info.Latest = normalize(release.TagName)
	info.URL = release.HTMLURL

	if info.Current == "" || info.Current == "dev" {
		info.Reason = ReasonDevBuild
		info.Message = "development build; version comparison unavailable"
		return info, nil
	}
	cmp, err := compareVersions(info.Latest, info.Current)
	if err != nil {
		info.Reason = ReasonUnparsable
		info.Message = "could not parse version numbers"
		return info, nil
	}
	info.HasUpdate = cmp > 0
	if info.HasUpdate {
		info.Reason = ReasonAvailable
	} else {
		info.Reason = ReasonUpToDate
		info.Message = "already up to date"
	}
	return info, nil
}

// normalize 去掉前导 v 与空白。
func normalize(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// compareVersions 按 SemVer 规则比较版本号：先比 major.minor.patch，
// 主版本相同时正式版大于预发布版，预发布标识按 §11 逐段比较。
// 返回 1（a 更新）、0（相等）、-1（b 更新）。
func compareVersions(a, b string) (int, error) {
	pa, preA, err := parseVersion(a)
	if err != nil {
		return 0, err
	}
	pb, preB, err := parseVersion(b)
	if err != nil {
		return 0, err
	}
	for i := 0; i < 3; i++ {
		if pa[i] > pb[i] {
			return 1, nil
		}
		if pa[i] < pb[i] {
			return -1, nil
		}
	}
	switch {
	case preA == "" && preB == "":
		return 0, nil
	case preA == "":
		return 1, nil // 正式版 > 预发布版
	case preB == "":
		return -1, nil
	}
	return comparePrerelease(preA, preB), nil
}

// parseVersion 拆出数字段与预发布标识（忽略 build metadata）。
func parseVersion(v string) ([3]int, string, error) {
	var parts [3]int
	v = normalize(v)
	if plus := strings.IndexByte(v, '+'); plus >= 0 {
		v = v[:plus]
	}
	pre := ""
	if dash := strings.IndexByte(v, '-'); dash >= 0 {
		pre = v[dash+1:]
		v = v[:dash]
	}
	segs := strings.Split(v, ".")
	if len(segs) == 0 || len(segs) > 3 {
		return parts, pre, fmt.Errorf("invalid version %q", v)
	}
	for i, seg := range segs {
		n, err := strconv.Atoi(seg)
		if err != nil || n < 0 {
			return parts, pre, fmt.Errorf("invalid version %q", v)
		}
		parts[i] = n
	}
	return parts, pre, nil
}

// comparePrerelease 比较两个非空的预发布标识串。
func comparePrerelease(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		an, aErr := strconv.Atoi(as[i])
		bn, bErr := strconv.Atoi(bs[i])
		switch {
		case aErr == nil && bErr == nil:
			if an != bn {
				if an > bn {
					return 1
				}
				return -1
			}
		case aErr == nil: // 数字标识优先级低于字母标识
			return -1
		case bErr == nil:
			return 1
		default:
			if cmp := strings.Compare(as[i], bs[i]); cmp != 0 {
				return cmp
			}
		}
	}
	switch {
	case len(as) > len(bs):
		return 1
	case len(as) < len(bs):
		return -1
	}
	return 0
}
