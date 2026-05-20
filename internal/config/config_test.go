package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestStoreUpdatePersistsConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "proxy.yaml")

	data, err := yaml.Marshal(DefaultConfig())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, data, 0o644))

	store, err := NewStore(configPath)
	require.NoError(t, err)

	triggered := make(chan Config, 1)
	store.Subscribe(func(cfg Config) {
		triggered <- cfg
	})

	err = store.Update(func(cfg *Config) error {
		cfg.Access.Rules = append(cfg.Access.Rules, AccessRuleConfig{
			ID:        "dynamic-rule",
			Type:      "allow",
			Value:     "192.168.0.1",
			CreatedAt: time.Now().UTC(),
		})
		return nil
	})
	require.NoError(t, err)

	select {
	case cfg := <-triggered:
		require.Len(t, cfg.Access.Rules, 2)
	case <-time.After(time.Second):
		t.Fatal("expected subscriber to receive updated config")
	}

	reloaded, err := Load(configPath)
	require.NoError(t, err)
	require.Len(t, reloaded.Access.Rules, 2)
}
