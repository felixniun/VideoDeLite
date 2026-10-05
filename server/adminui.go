package server

import (
	"embed"
	"net/http"
)

//go:embed adminui.html
var adminUIFS embed.FS

// handleAdminUI serves the embedded admin console. It is a static page:
// the admin key is entered in the browser, kept in localStorage, and sent
// as X-Admin-Key to the JSON admin APIs below. No session, no cookies.
//
// Policy note: the account system is fully free in V1 — this console is for
// account hygiene (bans, revocation, audit), NOT for selling licenses.
// License issuance stays automatic for every verified account; a paid
// entitlement flow can be layered in later without UI changes.
func (s *Server) handleAdminUI(w http.ResponseWriter, r *http.Request) {
	data, err := adminUIFS.ReadFile("adminui.html")
	if err != nil {
		http.Error(w, "admin ui missing", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}
