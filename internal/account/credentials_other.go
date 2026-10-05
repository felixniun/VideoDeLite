//go:build !windows

package account

import "errors"

func storeCredentials(t *tokens) error { return errors.New("credential storage not supported on this platform") }

func loadCredentials() (*tokens, error) { return nil, errors.New("credential storage not supported") }

func deleteCredentials() error { return nil }
