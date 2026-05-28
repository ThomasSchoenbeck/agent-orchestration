package agent

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"testing"
)

// captureLog redirects the default logger to a buffer for the duration of f.
func captureLog(f func()) string {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	f()
	return buf.String()
}

func TestAgentLogger_Debug_writesToStdout(t *testing.T) {
	l := newLogger("agent-1", nil)
	out := captureLog(func() {
		l.Debug("hello %s", "world")
	})
	if !strings.Contains(out, "[debug]") {
		t.Errorf("expected [debug] prefix, got: %s", out)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected message in output, got: %s", out)
	}
}

func TestAgentLogger_Info_includesAgentID(t *testing.T) {
	l := newLogger("agent-42", nil) // nil client — no PostLog
	out := captureLog(func() {
		l.Info("something happened")
	})
	if !strings.Contains(out, "agent=agent-42") {
		t.Errorf("expected agent id in output, got: %s", out)
	}
	if !strings.Contains(out, "[info]") {
		t.Errorf("expected [info] prefix, got: %s", out)
	}
}

func TestAgentLogger_ForTask_includesTaskID(t *testing.T) {
	l := newLogger("a1", nil).ForTask("task-99")
	out := captureLog(func() {
		l.Warn("something")
	})
	if !strings.Contains(out, "task=task-99") {
		t.Errorf("expected task id in output, got: %s", out)
	}
}

func TestAgentLogger_Error_writesToStdout(t *testing.T) {
	l := newLogger("a1", nil)
	out := captureLog(func() {
		l.Error("bad: %v", "oops")
	})
	if !strings.Contains(out, "[error]") {
		t.Errorf("expected [error] prefix, got: %s", out)
	}
	if !strings.Contains(out, "bad: oops") {
		t.Errorf("expected message in output, got: %s", out)
	}
}

func TestAgentLogger_ForProject_includesNeitherTaskNorAgent(t *testing.T) {
	l := (&AgentLogger{}).ForProject("proj-7")
	out := captureLog(func() {
		l.Debug("test")
	})
	// agentID and taskID are empty; only message should appear
	if strings.Contains(out, "agent=") {
		t.Errorf("unexpected agent= in output: %s", out)
	}
	if strings.Contains(out, "task=") {
		t.Errorf("unexpected task= in output: %s", out)
	}
}

func TestAgentLogger_InfoCtx_nilClient(t *testing.T) {
	// Should not panic when client is nil.
	l := newLogger("a1", nil)
	l.InfoCtx(context.Background(), "msg %d", 1)
}
