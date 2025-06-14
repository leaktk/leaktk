package logger

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	glog "github.com/zricethezav/gitleaks/v8/logging"
)

func init() {
	// Disable logging by default to make sure that gitleaks can't produce logs
	// without being specifically configured
	glog.Logger.Level(zerolog.Disabled)
	// Provide a custom handler to map to this logging framework
	glog.Logger = zlog.Output(zerologMapper{})
}

// zerologMapper helps translate logs from subsystems that use zerolog
type zerologMapper struct {
}

// Write implements an io.Writer interface
func (m zerologMapper) Write(data []byte) (int, error) {
	var event struct {
		Level   string `json:"level"`
		Message string `json:"message"`
	}

	if err := json.Unmarshal(data, &event); err != nil {
		Error("could not decode zerolog event: error=%q", err)

		return 0, nil
	}

	switch event.Level {
	case "info":
		Info("gitleaks: %s", event.Message)
	case "warn":
		Warning("gitleaks: %s", event.Message)
	case "error":
		Error("gitleaks: %s", event.Message)
	case "fatal":
		Critical("gitleaks: %s", event.Message)
	case "panic":
		Critical("gitleaks: %s", event.Message)
	default:
		Debug("gitleaks: %s", event.Message)
	}

	return len(data), nil
}

// LogLevel is used to determine which log severities should actually log
type LogLevel int

// LogFormat is used to set the how the log messages should be displayed
type LogFormat int

const (
	// NOTSET will log everything
	NOTSET LogLevel = 0
	// DEBUG will enable these logs and higher
	DEBUG LogLevel = 10
	// INFO will enable these logs and higher
	INFO LogLevel = 20
	// WARNING will enable these logs and higher
	WARNING LogLevel = 30
	// ERROR will enable these logs and higher
	ERROR LogLevel = 40
	// CRITICAL will enable these logs and higher
	CRITICAL LogLevel = 50
)

const (
	// JSON displays the logs as JSON dicts
	JSON LogFormat = 0
	// HUMAN displays the logs in a way that's nice for humans to read
	HUMAN LogFormat = 1
)

// String renders a LogLevel as its string value
func (l LogLevel) String() string {
	switch l {
	case NOTSET:
		return "NOTSET"
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARNING:
		return "WARNING"
	case ERROR:
		return "WARNING"
	case CRITICAL:
		return "CRITICAL"
	default:
		return "INVALID"
	}
}

var currentLogLevel = INFO
var currentLogFormat = HUMAN

// Entry defines a log entry
type Entry struct {
	Time     string `json:"time"`
	Severity string `json:"severity"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message"`
}

// String renders a log entry structure to the JSON format
func (e Entry) String() string {
	if e.Severity == "" {
		e.Severity = "INFO"
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

// SetLoggerFormat adjusts the format Entry uses when calling String() on it
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

// SetLoggerLevel takes the string version of the name and sets the current level
func SetLoggerLevel(levelName string) error {
	switch levelName {
	case "DEBUG":
		currentLogLevel = DEBUG
		glog.Logger.Level(zerolog.DebugLevel)
	case "INFO":
		currentLogLevel = INFO
		glog.Logger.Level(zerolog.InfoLevel)
	case "WARNING":
		currentLogLevel = WARNING
		glog.Logger.Level(zerolog.WarnLevel)
	case "ERROR":
		currentLogLevel = ERROR
		glog.Logger.Level(zerolog.ErrorLevel)
	case "CRITICAL":
		currentLogLevel = CRITICAL
		glog.Logger.Level(zerolog.FatalLevel)
	default:
		return fmt.Errorf("invalid log level: level=%q", levelName)
	}

	return nil
}

// GetLoggerLevel returns the current logger level
func GetLoggerLevel() LogLevel {
	return currentLogLevel
}

// Debug emits an DEBUG level log
func Debug(msg string, a ...any) *Entry {
	if currentLogLevel > DEBUG {
		return nil
	}
	entry := Entry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Severity: "DEBUG",
		Message:  fmt.Sprintf(msg, a...),
	}
	log.Println(entry)

	return &entry
}

// Info emits an INFO level log
func Info(msg string, a ...any) *Entry {
	if currentLogLevel > INFO {
		return nil
	}
	entry := Entry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Severity: "INFO",
		Message:  fmt.Sprintf(msg, a...),
	}
	log.Println(entry)

	return &entry
}

// Warning emits an WARNING level log
func Warning(msg string, a ...any) *Entry {
	if currentLogLevel > WARNING {
		return nil
	}
	entry := Entry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Severity: "WARNING",
		Message:  fmt.Sprintf(msg, a...),
	}
	log.Println(entry)

	return &entry
}

// Error emits an ERROR level log
func Error(msg string, a ...any) *Entry {
	if currentLogLevel > ERROR {
		return nil
	}
	entry := Entry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Severity: "ERROR",
		Message:  fmt.Errorf(msg, a...).Error(),
	}
	log.Println(entry)

	return &entry
}

// Critical emits an CRITICAL level log
func Critical(msg string, a ...any) *Entry {
	if currentLogLevel > CRITICAL {
		return nil
	}
	entry := Entry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Severity: "CRITICAL",
		Message:  fmt.Errorf(msg, a...).Error(),
	}
	log.Println(entry)

	return &entry
}

// Fatal emits an CRITICAL level log and stops the program
func Fatal(msg string, a ...any) {
	log.Fatal(Entry{
		Time:     time.Now().UTC().Format(time.RFC3339),
		Severity: "CRITICAL",
		Message:  fmt.Errorf(msg, a...).Error(),
	})
}
