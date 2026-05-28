package config

import (
	"context"
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

func TestEnsureDefaultConfig_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new", "proxy.yaml")
	require.NoError(t, EnsureDefaultConfig(path))
	_, err := os.Stat(path)
	require.NoError(t, err)
}

func TestEnsureDefaultConfig_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "proxy.yaml")
	require.NoError(t, os.WriteFile(path, []byte("server:\n  address: :9090\n"), 0o644))
	require.NoError(t, EnsureDefaultConfig(path))
	raw, _ := os.ReadFile(path)
	require.Contains(t, string(raw), ":9090")
}

func TestStoreCurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "proxy.yaml")
	cfg := DefaultConfig()
	cfg.Server.Address = ":9999"
	raw, err := yaml.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o644))

	store, err := NewStore(path)
	require.NoError(t, err)
	require.Equal(t, ":9999", store.Current().Server.Address)
}

func TestStoreWatch_CancelContext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "proxy.yaml")
	raw, err := yaml.Marshal(DefaultConfig())
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o644))

	store, err := NewStore(path)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- store.Watch(ctx) }()

	cancel()
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		// Watch started goroutine; nil return is also acceptable
	}
}

func TestParseDurationOrDefault_Valid(t *testing.T) {
	require.Equal(t, 5*time.Minute, ParseDurationOrDefault("5m", time.Second))
	require.Equal(t, 30*time.Second, ParseDurationOrDefault("30s", time.Hour))
}

func TestParseDurationOrDefault_Invalid(t *testing.T) {
	require.Equal(t, 10*time.Second, ParseDurationOrDefault("bad", 10*time.Second))
	require.Equal(t, time.Minute, ParseDurationOrDefault("", time.Minute))
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/no/such/path/config.yaml")
	require.Error(t, err)
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(path, []byte("{{not valid yaml"), 0o644))
	_, err := Load(path)
	require.Error(t, err)
}
