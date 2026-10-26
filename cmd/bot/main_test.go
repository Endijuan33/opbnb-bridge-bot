package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigFlagRunsBotWithAndWithoutRunSubcommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-mainnet.toml")
	for _, args := range [][]string{
		{"bot", "--config", path},
		{"bot", "run", "--config", path},
	} {
		err := newCli("", "").Run(args)
		require.ErrorContains(t, err, path)
	}
}
