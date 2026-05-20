package logger

import (
	"os"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
)

type AsyncLogger struct {
	level atomic.Int32
	ch    chan logEvent
	wg    sync.WaitGroup
}

type logEvent struct {
	level  zerolog.Level
	msg    string
	fields map[string]any
}

func NewAsyncLogger(level string) (*AsyncLogger, error) {
	parsed, err := zerolog.ParseLevel(level)
	if err != nil {
		parsed = zerolog.InfoLevel
	}

	logger := &AsyncLogger{
		ch: make(chan logEvent, 2048),
	}
	logger.level.Store(int32(parsed))

	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	base := zerolog.New(os.Stdout).With().Timestamp().Logger()

	logger.wg.Add(1)
	go func() {
		defer logger.wg.Done()
		for event := range logger.ch {
			if event.level < zerolog.Level(logger.level.Load()) {
				continue
			}
			entry := base.WithLevel(event.level)
			for key, value := range event.fields {
				entry = entry.Interface(key, value)
			}
			entry.Msg(event.msg)
		}
	}()

	return logger, nil
}

func (l *AsyncLogger) SetLevel(level string) {
	parsed, err := zerolog.ParseLevel(level)
	if err != nil {
		return
	}
	l.level.Store(int32(parsed))
}

func (l *AsyncLogger) Log(level zerolog.Level, msg string, fields map[string]any) {
	select {
	case l.ch <- logEvent{level: level, msg: msg, fields: fields}:
	default:
	}
}

func (l *AsyncLogger) Info(msg string, fields map[string]any) {
	l.Log(zerolog.InfoLevel, msg, fields)
}

func (l *AsyncLogger) Warn(msg string, fields map[string]any) {
	l.Log(zerolog.WarnLevel, msg, fields)
}

func (l *AsyncLogger) Error(msg string, fields map[string]any) {
	l.Log(zerolog.ErrorLevel, msg, fields)
}

func (l *AsyncLogger) Close() {
	close(l.ch)
	l.wg.Wait()
}
