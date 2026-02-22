package glog

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type Logger struct {
	Fields []F
}

func (l *Logger) Log(channel string, msg string, fields ...F) {
	var b strings.Builder
	b.Grow(100) // NOTE(ben): 100 characters should be enough for any body

	fmt.Fprintf(&b, "%s [%s] %s", time.Now().Format("2006-01-02 15:04:05"), channel, msg)
	for i, f := range fields {
		if i == 0 {
			fmt.Fprintf(&b, " [")
		} else {
			fmt.Fprintf(&b, ", ")
		}
		fmt.Fprintf(&b, "%s=\"%v\"", f.Name, f.Value)
		if i == len(fields)-1 {
			fmt.Fprintf(&b, "]")
		}
	}
	fmt.Fprintf(&b, "\n")

	fmt.Fprint(os.Stderr, b.String())
}

func (l *Logger) Debug(msg string, fields ...F) {
	l.Log("DEBUG", msg, fields...)
}

func (l *Logger) Info(msg string, fields ...F) {
	l.Log("INFO", msg, fields...)
}

func (l *Logger) Warning(msg string, fields ...F) {
	l.Log("WARN", msg, fields...)
}

func (l *Logger) Error(msg string, fields ...F) {
	l.Log("ERROR", msg, fields...)
}

func (l *Logger) Err(msg string, err error) {
	l.Error(msg, F{"err", err})
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
