package logger_test

import (
	"testing"

	"ProxyService2/pkg/logger"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"
)

func TestNewAsyncLogger_DefaultLevel(t *testing.T) {
	l, err := logger.NewAsyncLogger("info")
	require.NoError(t, err)
	require.NotNil(t, l)
	defer l.Close()
}

func TestNewAsyncLogger_InvalidLevel(t *testing.T) {
	l, err := logger.NewAsyncLogger("not-a-level")
	require.NoError(t, err)
	require.NotNil(t, l)
	defer l.Close()
}

func TestAsyncLogger_LogLevels(t *testing.T) {
	l, err := logger.NewAsyncLogger("debug")
	require.NoError(t, err)
	defer l.Close()

	l.Info("info message", map[string]any{"key": "value"})
	l.Warn("warn message", nil)
	l.Error("error message", map[string]any{"err": "something"})
	l.Log(zerolog.DebugLevel, "debug message", map[string]any{"x": 1})
}

func TestAsyncLogger_SetLevel(t *testing.T) {
	l, err := logger.NewAsyncLogger("debug")
	require.NoError(t, err)
	defer l.Close()

	l.SetLevel("warn")
	l.Info("this should be filtered", nil)
	l.Warn("this should pass", nil)
}

func TestAsyncLogger_SetLevel_Invalid(t *testing.T) {
	l, err := logger.NewAsyncLogger("info")
	require.NoError(t, err)
	defer l.Close()

	l.SetLevel("garbage")
	l.Info("still works", nil)
}

func TestAsyncLogger_Close(t *testing.T) {
	l, err := logger.NewAsyncLogger("info")
	require.NoError(t, err)
	l.Info("before close", nil)
	l.Close()
}
