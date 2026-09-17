package logger

import (
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"slices"
	"time"
)

type LogLevel int

const (
	NOTSET LogLevel = iota
	TRACE
	DEBUG
	INFO
	WARNING
	ERROR
	CRITICAL
)

var currentLogLevel = INFO
var logLevelNames = []string{
	"NOTSET",
	"TRACE",
	"DEBUG",
	"INFO",
	"WARNING",
	"ERROR",
}

func (l LogLevel) String() string {
	return logLevelNames[l]
}

func (l LogLevel) MarshalText() ([]byte, error) {
	return []byte(l.String()), nil
}

type LogFormat int

const (
	// JSON displays the logs as JSON dicts
	JSON LogFormat = iota
	// HUMAN displays the logs in a way that's nice for humans to read
	HUMAN
)

var currentLogFormat = HUMAN
var logFormatNames = []string{
	"JSON",
	"HUMAN",
}

func (l LogFormat) String() string {
	return logFormatNames[l]
}

type Entry struct {
	Time     string   `json:"time"`
	Severity LogLevel `json:"severity"`
	Message  string   `json:"message"`
}

func (e Entry) String() string {
	if e.Severity == NOTSET {
		e.Severity = INFO
	}

	switch currentLogFormat {
	case HUMAN:
		return fmt.Sprintf("[%s] %s", e.Severity, e.Message)
	case JSON:
		out, err := json.Marshal(e)

		if err != nil {
			log.Printf("json.Marshal: %v", err)
		}

		return string(out)
	default:
		return e.Message
	}
}

func init() {
	// Disable log prefixes such as the default timestamp.
	// Prefix text prevents the message from being parsed as JSON.
	// A timestamp is added when shipping logs to Cloud Logging.
	log.SetFlags(0)
}

// Returns the underlying slog.Logger
func SLogger() *slog.Logger {
	// TODO: swap out the logging code with slog.Logger based calls
	return slog.Default()
}

func SetLoggerFormat(logFormat LogFormat) error {
	switch logFormat {
	case JSON:
		currentLogFormat = JSON
	case HUMAN:
		currentLogFormat = HUMAN
	default:
		return fmt.Errorf("invalid log format: log_format=%v", logFormat)
	}

	return nil
}

func SetLoggerLevel(levelName string) error {
	i := slices.Index(logLevelNames, levelName)
	if i < 1 {
		return fmt.Errorf("invalid log level: level=%q", levelName)
	}
	currentLogLevel = LogLevel(i)
	return nil
}

func GetLoggerLevel() LogLevel {
	return currentLogLevel
}

func Trace(msg string, a ...any) *Entry {
	if currentLogLevel > TRACE {
		return nil
	}
	entry := Entry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Severity: TRACE,
		Message:  fmt.Sprintf(msg, a...),
	}
	log.Println(entry)

	return &entry
}

func Debug(msg string, a ...any) *Entry {
	if currentLogLevel > DEBUG {
		return nil
	}
	entry := Entry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Severity: DEBUG,
		Message:  fmt.Sprintf(msg, a...),
	}
	log.Println(entry)

	return &entry
}

func Info(msg string, a ...any) *Entry {
	if currentLogLevel > INFO {
		return nil
	}
	entry := Entry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Severity: INFO,
		Message:  fmt.Sprintf(msg, a...),
	}
	log.Println(entry)

	return &entry
}

func Warning(msg string, a ...any) *Entry {
	if currentLogLevel > WARNING {
		return nil
	}
	entry := Entry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Severity: WARNING,
		Message:  fmt.Sprintf(msg, a...),
	}
	log.Println(entry)

	return &entry
}

func Error(msg string, a ...any) *Entry {
	if currentLogLevel > ERROR {
		return nil
	}
	entry := Entry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Severity: ERROR,
		Message:  fmt.Errorf(msg, a...).Error(),
	}
	log.Println(entry)

	return &entry
}

func Critical(msg string, a ...any) *Entry {
	if currentLogLevel > CRITICAL {
		return nil
	}
	entry := Entry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Severity: CRITICAL,
		Message:  fmt.Errorf(msg, a...).Error(),
	}
	log.Println(entry)

	return &entry
}

func Fatal(msg string, a ...any) {
	log.Fatal(Entry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Severity: CRITICAL,
		Message:  fmt.Errorf(msg, a...).Error(),
	})
}
