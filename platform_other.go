//go:build !windows

package main

func systemLanguage() string { return "en" }

func systemTheme() string { return "light" }
