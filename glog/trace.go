package glog

import "github.com/go-stack/stack"

type StackFrame struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Function string `json:"function"`
}

type CallStack []StackFrame

func Trace() CallStack {
	trace := stack.Trace().TrimRuntime()[1:]
	frames := make(CallStack, len(trace))
	for i, call := range trace {
		callFrame := call.Frame()
		frames[i] = StackFrame{
			File:     callFrame.File,
			Line:     callFrame.Line,
			Function: callFrame.Function,
		}
	}

	return frames
}
