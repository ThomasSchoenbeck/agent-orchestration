package server_test

import (
	"net/http"
	"testing"
)

func TestTaskComments_CreateAndList(t *testing.T) {
	srv, _ := newTestServer(t)
	pid := createTestProject(t, srv)

	tw := do(t, srv, http.MethodPost, "/api/tasks", map[string]interface{}{
		"project_id": pid, "role": "worker", "priority": 5,
	})
	if tw.Code != http.StatusCreated {
		t.Fatalf("create task: %d %s", tw.Code, tw.Body.String())
	}
	taskID, _ := decodeMap(t, tw.Body.Bytes())["id"].(string)
	if taskID == "" {
		t.Fatal("task missing id")
	}

	cw := do(t, srv, http.MethodPost, "/api/tasks/"+taskID+"/comments", map[string]interface{}{
		"body": "looks good", "author_type": "agent", "author_role": "worker",
	})
	if cw.Code != http.StatusCreated {
		t.Fatalf("create comment: %d %s", cw.Code, cw.Body.String())
	}
	if w := do(t, srv, http.MethodPost, "/api/tasks/"+taskID+"/comments", map[string]interface{}{"author_type": "agent"}); w.Code != http.StatusBadRequest {
		t.Errorf("comment without body: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/tasks/"+taskID+"/comments", nil); w.Code != http.StatusOK {
		t.Errorf("list comments: %d", w.Code)
	}
}

func TestConversationMessages(t *testing.T) {
	srv, _ := newTestServer(t)
	cw := do(t, srv, http.MethodPost, "/api/conversations", map[string]interface{}{"title": "c"})
	if cw.Code != http.StatusCreated {
		t.Fatalf("create conversation: %d %s", cw.Code, cw.Body.String())
	}
	cid, _ := decodeMap(t, cw.Body.Bytes())["id"].(string)

	if w := do(t, srv, http.MethodPost, "/api/conversations/"+cid+"/messages", map[string]interface{}{
		"role": "user", "content": "hello",
	}); w.Code != http.StatusCreated {
		t.Fatalf("add message: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodPost, "/api/conversations/"+cid+"/messages", map[string]interface{}{"role": "user"}); w.Code != http.StatusBadRequest {
		t.Errorf("message without content: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/conversations/"+cid+"/messages", nil); w.Code != http.StatusOK {
		t.Errorf("list messages: %d", w.Code)
	}
}

func TestLogs_CreateAndList(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodPost, "/api/logs", map[string]interface{}{"level": "info", "message": "hello"}); w.Code != http.StatusCreated {
		t.Fatalf("create log: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodGet, "/api/logs", nil); w.Code != http.StatusOK {
		t.Errorf("list logs: %d", w.Code)
	}
}

func TestChecklistTemplates_CreateAndList(t *testing.T) {
	srv, _ := newTestServer(t)
	if w := do(t, srv, http.MethodPost, "/api/checklist-templates", map[string]interface{}{"name": "default"}); w.Code != http.StatusCreated {
		t.Fatalf("create template: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodPost, "/api/checklist-templates", map[string]interface{}{}); w.Code != http.StatusBadRequest {
		t.Errorf("template without name: expected 400, got %d", w.Code)
	}
	if w := do(t, srv, http.MethodGet, "/api/checklist-templates", nil); w.Code != http.StatusOK {
		t.Errorf("list templates: %d", w.Code)
	}
}

func TestAgentStatsAndOffline(t *testing.T) {
	srv, _ := newTestServer(t)
	rw := do(t, srv, http.MethodPost, "/api/agents/register", map[string]interface{}{
		"name": "a1", "roles": []string{"worker"}, "mode": "remote",
	})
	if rw.Code != http.StatusCreated {
		t.Fatalf("register agent: %d %s", rw.Code, rw.Body.String())
	}
	aid, _ := decodeMap(t, rw.Body.Bytes())["agent_id"].(string)
	if aid == "" {
		t.Fatal("agent missing id")
	}

	if w := do(t, srv, http.MethodGet, "/api/agents/"+aid+"/stats", nil); w.Code != http.StatusOK {
		t.Errorf("agent stats: %d %s", w.Code, w.Body.String())
	}
	if w := do(t, srv, http.MethodPost, "/api/agents/"+aid+"/offline", nil); w.Code != http.StatusOK {
		t.Errorf("agent offline: %d %s", w.Code, w.Body.String())
	}
}
