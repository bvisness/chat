package glog

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

var mu sync.Mutex

func lockAndUnlock() func() {
	// TODO(ben): This seems to have caused hangs. Dumb.
	// mu.Lock()
	// return func() {
	// 	mu.Unlock()
	// }
	return func() {}
}

type Logger struct {
	Fields []F
}

func (l *Logger) Log(channel string, msg string, fields ...F) {
	var b strings.Builder
	b.Grow(100) // NOTE(ben): 100 characters should be enough for any body

	fmt.Fprintf(&b, "%s [%s] %s", time.Now().Format("2006-01-02 15:04:05"), channel, msg)
	allFields := append(l.Fields, fields...)
	for i, f := range allFields {
		if i == 0 {
			fmt.Fprintf(&b, " [")
		} else {
			fmt.Fprintf(&b, ", ")
		}
		fmt.Fprintf(&b, "%s=\"%v\"", f.Name, f.Value)
		if i == len(allFields)-1 {
			fmt.Fprintf(&b, "]")
		}
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprint(os.Stderr, b.String())
}

func (l *Logger) Debug(msg string, fields ...F) {
	defer lockAndUnlock()()
	l.Log("DEBUG", msg, fields...)
}

func (l *Logger) DebugErr(msg string, err error) {
	defer lockAndUnlock()()
	l.Debug(msg, F{"err", err})
}

func (l *Logger) Info(msg string, fields ...F) {
	defer lockAndUnlock()()
	l.Log("INFO", msg, fields...)
}

func (l *Logger) InfoErr(msg string, err error) {
	defer lockAndUnlock()()
	l.Info(msg, F{"err", err})
}

func (l *Logger) Warning(msg string, fields ...F) {
	defer lockAndUnlock()()
	l.Log("WARN", msg, fields...)
}

func (l *Logger) WarningErr(msg string, err error) {
	defer lockAndUnlock()()
	l.Warning(msg, F{"err", err})
}

func (l *Logger) Error(msg string, fields ...F) {
	defer lockAndUnlock()()
	l.Log("ERROR", msg, fields...)
}

func (l *Logger) Err(msg string, err error) {
	defer lockAndUnlock()()
	l.Error(msg, F{"err", err})
}

func (l *Logger) Panic(msg string, trim int, fields ...F) {
	defer lockAndUnlock()()
	l.Log("PANIC", msg, fields...)
	l.trace(1 + trim)
}

func (l *Logger) Recover(msg string) {
	if r := recover(); r != nil {
		l.Panic(msg, 1, F{"err", r})
	}
}

func (l *Logger) trace(trim int) {
	var b strings.Builder
	for _, frame := range Trace()[1+trim:] {
		const indent = "                   " // NOTE(ben): lines up with dates real nice
		fmt.Fprintf(&b, "%s %s\n", indent, frame.Function)
		fmt.Fprintf(&b, "%s \t%s:%d\n", indent, frame.File, frame.Line)
	}
	fmt.Fprint(os.Stderr, b.String())
}

func (l *Logger) WithFields(fields ...F) Logger {
	return Logger{
		Fields: append(l.Fields, fields...),
	}
}

type F struct {
	Name  string
	Value any
}
