package logger

import (
    "fmt"
    "log"
    "os"
    "strings"
    "sync"
    "time"
)

var (
    mu     sync.Mutex
    level  = InfoLevel
    format = "text" // text or json
)

type LogLevel int

const (
    DebugLevel LogLevel = iota
    InfoLevel
    WarnLevel
    ErrorLevel
)

func Init(lvl string, fmtStr string) {
    mu.Lock()
    defer mu.Unlock()
    switch strings.ToLower(lvl) {
    case "debug":
        level = DebugLevel
    case "info":
        level = InfoLevel
    case "warn":
        level = WarnLevel
    case "error":
        level = ErrorLevel
    default:
        level = InfoLevel
    }
    if fmtStr == "json" {
        format = "json"
    } else {
        format = "text"
    }
}

func logMessage(lvl LogLevel, msg string, args ...any) {
    mu.Lock()
    defer mu.Unlock()
    if lvl < level {
        return
    }
    now := time.Now().UTC().Format("2006-01-02T15:04:05Z")
    var prefix string
    switch lvl {
    case DebugLevel:
        prefix = "DEBUG"
    case InfoLevel:
        prefix = "INFO"
    case WarnLevel:
        prefix = "WARN"
    case ErrorLevel:
        prefix = "ERROR"
    }
    // build message with args
    if len(args) > 0 {
        // simple key=value formatting
        var pairs []string
        for i := 0; i+1 < len(args); i += 2 {
            key := args[i]
            val := args[i+1]
            pairs = append(pairs, fmt.Sprintf("%v=%v", key, val))
        }
        if len(pairs) > 0 {
            msg = msg + " " + strings.Join(pairs, " ")
        }
    }
    if format == "json" {
        // simplistic JSON – enough for basic use
        fmt.Fprintf(os.Stderr, `{"time":"%s","level":"%s","msg":"%s"}`, now, prefix, msg)
    } else {
        fmt.Fprintf(os.Stderr, "[%s] %s %s\n", now, prefix, msg)
    }
}

func Debug(msg string, args ...any) { logMessage(DebugLevel, msg, args...) }
func Info(msg string, args ...any)  { logMessage(InfoLevel, msg, args...) }
func Warn(msg string, args ...any)  { logMessage(WarnLevel, msg, args...) }
func Error(msg string, args ...any) { logMessage(ErrorLevel, msg, args...) }
func Fatal(msg string, args ...any) { logMessage(ErrorLevel, msg, args...); os.Exit(1) }
