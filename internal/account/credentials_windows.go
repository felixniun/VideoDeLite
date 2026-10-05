//go:build windows

package account

import (
	"encoding/json"

	"github.com/danieljoos/wincred"
)

// storeCredentials persists tokens in Windows Credential Manager
// (authorization spec §17: never plaintext, never in SQLite).
func storeCredentials(t *tokens) error {
	raw, err := json.Marshal(t)
	if err != nil {
		return err
	}
	cred := wincred.NewGenericCredential(credName)
	cred.CredentialBlob = raw
	cred.Persist = wincred.PersistLocalMachine
	return cred.Write()
}

func loadCredentials() (*tokens, error) {
	cred, err := wincred.GetGenericCredential(credName)
	if err != nil {
		return nil, err
	}
	var t tokens
	if err := json.Unmarshal(cred.CredentialBlob, &t); err != nil {
		return nil, err
	}
	if t.Access == "" && t.Refresh == "" {
		return nil, errNoCredentials
	}
	return &t, nil
}

func deleteCredentials() error {
	cred := wincred.NewGenericCredential(credName)
	return cred.Delete()
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

const errNoCredentials = simpleError("no stored credentials")
