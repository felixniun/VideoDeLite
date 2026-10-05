package main

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	modkernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	procGetUserDefaultLocaleName = modkernel32.NewProc("GetUserDefaultLocaleName")
)

// systemLanguage maps the Windows UI locale to the two first-run languages
// (plan §9): zh* → 简体中文, everything else → English.
func systemLanguage() string {
	buf := make([]uint16, 85)
	r, _, _ := procGetUserDefaultLocaleName.Call(
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if r == 0 {
		return "en"
	}
	locale := strings.ToLower(windows.UTF16ToString(buf))
	if strings.HasPrefix(locale, "zh") {
		return "zh-CN"
	}
	return "en"
}

// systemTheme reads AppsUseLightTheme from personalization (plan §10).
func systemTheme() string {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`,
		registry.QUERY_VALUE)
	if err != nil {
		return "light"
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue("AppsUseLightTheme")
	if err != nil || v == 0 {
		return "dark"
	}
	return "light"
}
