package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// webview2MinVersion is the minimum runtime we accept (plan §4: the product
// baseline; WebView2 evergreen auto-updates anyway).
const webview2DownloadURL = "https://developer.microsoft.com/microsoft-edge/webview2/"

// isWebView2Installed checks the evergreen runtime locations per Microsoft's
// documented detection keys (HKLM + HKCU, both views).
func isWebView2Installed() (string, bool) {
	keys := []struct {
		path  string
		wow64 bool
	}{
		{`SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`, true},
		{`SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`, false},
		{`SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`, false},
	}
	views := []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY}
	seen := map[string]bool{}
	for _, k := range keys {
		for _, view := range views {
			id := k.path + fmt.Sprint(view)
			if seen[id] {
				continue
			}
			seen[id] = true
			key, err := registry.OpenKey(registry.LOCAL_MACHINE, k.path, registry.QUERY_VALUE)
			if err != nil {
				// HKCU fallback (per-user runtime installs)
				key, err = registry.OpenKey(registry.CURRENT_USER, k.path, registry.QUERY_VALUE)
				if err != nil {
					continue
				}
			}
			pv, _, err := key.GetStringValue("pv")
			key.Close()
			if err == nil && pv != "" {
				return pv, true
			}
		}
	}
	return "", false
}

// ensureWebView2OrExit shows an explicit user-facing dialog and exits when
// the runtime is missing. Silent download/install is forbidden (plan §6).
func ensureWebView2OrExit() {
	if _, ok := isWebView2Installed(); ok {
		return
	}
	title := "VideoDelite 需要 WebView2 Runtime"
	msg := "检测到本机未安装 Microsoft Edge WebView2 Runtime。\n\n" +
		"VideoDelite 需要该组件才能显示界面（Windows 11 通常已内置）。\n\n" +
		"请从微软官网下载安装“Evergreen Standalone Installer”后重新运行本程序：\n" +
		webview2DownloadURL + "\n\n" +
		"现在打开下载页面吗？"
	res, _ := messageBox(title, msg)
	if res == 1 { // IDYES / IDOK path
		shellOpen(webview2DownloadURL)
	}
	exitProcess(1)
}

var (
	moduser32   = windows.NewLazySystemDLL("user32.dll")
	modshell32  = windows.NewLazySystemDLL("shell32.dll")
	procMsgBoxW = moduser32.NewProc("MessageBoxW")
	procShellOW = modshell32.NewProc("ShellExecuteW")
)

// messageBox shows a Yes/No box; returns 1 when the user chose Yes.
func messageBox(title, text string) (int, error) {
	tp, err := utf16Ptr(title)
	if err != nil {
		return 0, err
	}
	xp, err := utf16Ptr(text)
	if err != nil {
		return 0, err
	}
	r, _, _ := procMsgBoxW.Call(0, uintptr(unsafe.Pointer(xp)), uintptr(unsafe.Pointer(tp)), 0x44 /*MB_YESNO|MB_ICONWARNING*/)
	return int(r), nil
}

func shellOpen(url string) {
	p, err := utf16Ptr(url)
	if err != nil {
		return
	}
	verb, _ := utf16Ptr("open")
	procShellOW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(p)), 0, 0, 1 /*SW_SHOWNORMAL*/)
}

func utf16Ptr(s string) (*uint16, error) {
	return windows.UTF16PtrFromString(s)
}

func exitProcess(code int) {
	windows.ExitProcess(uint32(code))
}
