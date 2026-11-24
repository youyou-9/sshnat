package startup

import "fmt"

// Report describes the environment a desktop start-up was about to run in.
type Report struct {
	// Exe is the full path of the running executable.
	Exe string
	// ExeDir is the folder that contains it, which is what carries the
	// mandatory label on Windows.
	ExeDir string
	// Level is the integrity level the process actually runs at.
	Level Level
	// LogPath is the diagnostic log the user can attach to a bug report. It is
	// empty when start-up could not open a log at all, which is exactly what
	// happens for a low-integrity process: Windows does not let it write into
	// %LOCALAPPDATA%.
	LogPath string
}

func (r Report) exeDirOrUnknown(lang string) string {
	if r.ExeDir != "" {
		return r.ExeDir
	}
	if lang == "en" {
		return "(unknown folder)"
	}
	return "（未知文件夹）"
}

func (r Report) logPathOrUnknown(lang string) string {
	if r.LogPath != "" {
		return r.LogPath
	}
	if lang == "en" {
		return "(unavailable: this integrity level cannot write to the log folder)"
	}
	return "（不可用：当前完整性级别不允许写入日志目录）"
}

// AbortMessage returns the title and body of the dialog shown when the
// environment cannot host the WebView2 UI. The folder is named explicitly
// because the fix depends on it, and the log path is included so a user who
// still cannot start the app has something to send.
func AbortMessage(lang string, r Report) (title, body string) {
	dir := r.exeDirOrUnknown(lang)
	logPath := r.logPathOrUnknown(lang)
	if lang == "en" {
		return "SSHNat cannot start from this folder", fmt.Sprintf(
			`SSHNat's window is hosted by WebView2, and WebView2 refuses to attach to a process running below medium integrity. This launch ran at %s integrity, so the window failed while it was being created and the process exited without showing anything.

Why: Windows starts a program at low integrity when the folder that contains it carries a low mandatory label, and this folder does:
  %s

Fixes (pick one)
1. Move SSHNat.exe out of that folder and run it from a normal location, for example
   %%LOCALAPPDATA%%\Programs\SSHNat\ or a new folder on the Desktop.
2. Open PowerShell as administrator and run the command below, replacing the path with the folder that actually holds SSHNat.exe:
   icacls "%s" /setintegritylevel (OI)(CI)Medium

Diagnostic log: %s`,
			r.Level, dir, dir, logPath)
	}
	return "SSHNat 无法在这个文件夹里启动", fmt.Sprintf(
		`SSHNat 的窗口由 WebView2 承载，而 WebView2 不会附加到「低完整性」进程上。本次启动的运行级别是 %s，所以窗口在创建阶段就失败了，进程随即退出，界面上什么都不会显示。

原因：Windows 会把带有「低完整性」标记的文件夹里的程序降权运行，而下面这个文件夹带有该标记：
  %s

解决方法（任选其一）
1. 把 SSHNat.exe 从这个文件夹移到普通位置再运行，例如
   %%LOCALAPPDATA%%\Programs\SSHNat\ 或者桌面上新建的文件夹。
2. 用「以管理员身份运行」打开 PowerShell，执行下面这条命令（路径换成你实际的文件夹）：
   icacls "%s" /setintegritylevel (OI)(CI)Medium

诊断日志：%s`,
		r.Level, dir, dir, logPath)
}
