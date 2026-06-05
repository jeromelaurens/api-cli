//go:build apicli_e2e_mock_keyring

package main

import "github.com/zalando/go-keyring"

func init() {
	keyring.MockInit()
	_ = keyring.Set("api-cli", "e2e/api-token", "e2e-bearer-token")
}
