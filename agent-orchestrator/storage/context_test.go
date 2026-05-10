package storage

import (
	"context"
	"os"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/llm"
)

// openTestDB opens a fresh SQLite database for each test.
func openTestDB(t *testing.T) *db.Database {
	t.Helper()
	f, err := os.CreateTemp("", "storage_test_*.db")
	if err != nil {
		t.Fatalf("create temp db: %v", err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })
	f.Close()

	d, err := db.Open(f.Name())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// createProject inserts a test project and returns its ID.
func createProject(t *testing.T, d *db.Database, name string) string {
	t.Helper()
	p := &db.Project{Name: name, Status: "planned"}
	if err := d.CreateProject(context.Background(), p); err != nil {
		t.Fatalf("create project: %v", err)
	}
	return p.ID
}

// mockEmbedder is a fake LLM provider that returns pre-set embeddings.
type mockEmbedder struct {
	embeddings [][]float32
	callCount  int
	err        error
}

func (m *mockEmbedder) Name() string { return "mock-embedder" }
func (m *mockEmbedder) Close() error { return nil }
func (m *mockEmbedder) Chat(_ context.Context, _ llm.ChatRequest) (llm.ChatResponse, error) {
	return llm.ChatResponse{}, nil
}
func (m *mockEmbedder) Rerank(_ context.Context, _ llm.RerankRequest) (llm.RerankResponse, error) {
	return llm.RerankResponse{}, nil
}
func (m *mockEmbedder) Embed(_ context.Context, req llm.EmbedRequest) (llm.EmbedResponse, error) {
	if m.err != nil {
		return llm.EmbedResponse{}, m.err
	}
	m.callCount++
	idx := m.callCount - 1
	if idx >= len(m.embeddings) {
		idx = len(m.embeddings) - 1
	}
	return llm.EmbedResponse{Embeddings: [][]float32{m.embeddings[idx]}}, nil
}

// --- ContextStore without embedder (keyword mode) ---

func TestContextStore_Save_NoEmbedder(t *testing.T) {
	d := openTestDB(t)
	projectID := createProject(t, d, "P1")
	cs := NewContextStore(d, nil, 100)

	err := cs.Save(context.Background(), &db.ContextEntry{
		ProjectID: projectID,
		Type:      "note",
		Content:   "some content",
		Metadata:  map[string]interface{}{},
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	entries, err := cs.Query(context.Background(), projectID, "content", 10)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

func TestContextStore_Query_KeywordFallback(t *testing.T) {
	d := openTestDB(t)
	projectID := createProject(t, d, "P2")
	cs := NewContextStore(d, nil, 100)

	for _, content := range []string{"apple pie", "banana cake", "apple sauce"} {
		if err := cs.Save(context.Background(), &db.ContextEntry{
			ProjectID: projectID,
			Type:      "note",
			Content:   content,
			Metadata:  map[string]interface{}{},
		}); err != nil {
			t.Fatalf("Save: %v", err)
		}
	}

	results, err := cs.Query(context.Background(), projectID, "apple", 10)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 apple entries, got %d", len(results))
	}
}

// --- ContextStore with embedder (semantic mode) ---

func TestContextStore_Save_WithEmbedder(t *testing.T) {
	d := openTestDB(t)
	projectID := createProject(t, d, "P3")

	embedder := &mockEmbedder{
		embeddings: [][]float32{{1, 0, 0}},
	}
	cs := NewContextStore(d, embedder, 100)

	if err := cs.Save(context.Background(), &db.ContextEntry{
		ProjectID: projectID,
		Type:      "note",
		Content:   "hello world",
		Metadata:  map[string]interface{}{},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if embedder.callCount != 1 {
		t.Errorf("expected 1 embed call, got %d", embedder.callCount)
	}

	// Retrieve and verify embedding was stored.
	embedded, err := d.GetContextEntriesWithEmbeddings(context.Background(), projectID)
	if err != nil {
		t.Fatalf("GetContextEntriesWithEmbeddings: %v", err)
	}
	if len(embedded) != 1 {
		t.Fatalf("expected 1 embedded entry, got %d", len(embedded))
	}
	if len(embedded[0].Embedding) == 0 {
		t.Error("expected non-empty embedding")
	}
}

func TestContextStore_Query_SemanticSearch(t *testing.T) {
	d := openTestDB(t)
	projectID := createProject(t, d, "P4")

	// Embeddings: call 1 = save "doc A", call 2 = save "doc B", call 3 = query
	embedder := &mockEmbedder{
		embeddings: [][]float32{
			{1, 0, 0}, // doc A embedding
			{0, 1, 0}, // doc B embedding
			{1, 0, 0}, // query embedding (most similar to doc A)
		},
	}
	cs := NewContextStore(d, embedder, 100)

	for _, c := range []string{"doc A", "doc B"} {
		if err := cs.Save(context.Background(), &db.ContextEntry{
			ProjectID: projectID,
			Type:      "note",
			Content:   c,
			Metadata:  map[string]interface{}{},
		}); err != nil {
			t.Fatalf("Save(%q): %v", c, err)
		}
	}

	results, err := cs.Query(context.Background(), projectID, "query text", 1)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Content != "doc A" {
		t.Errorf("expected top result %q, got %q", "doc A", results[0].Content)
	}
}

// --- Pruning ---

func TestContextStore_Prune(t *testing.T) {
	d := openTestDB(t)
	projectID := createProject(t, d, "P5")
	cs := NewContextStore(d, nil, 3) // max 3 items

	for i := 0; i < 5; i++ {
		if err := cs.Save(context.Background(), &db.ContextEntry{
			ProjectID: projectID,
			Type:      "note",
			Content:   "entry",
			Metadata:  map[string]interface{}{},
		}); err != nil {
			t.Fatalf("Save[%d]: %v", i, err)
		}
	}

	entries, err := d.QueryContext(context.Background(), projectID, "", 100)
	if err != nil {
		t.Fatalf("QueryContext: %v", err)
	}
	if len(entries) > 3 {
		t.Errorf("expected at most 3 entries after pruning, got %d", len(entries))
	}
}

// --- Project memory ---

func TestContextStore_SaveProjectMemory(t *testing.T) {
	d := openTestDB(t)
	projectID := createProject(t, d, "P6")
	cs := NewContextStore(d, nil, 100)

	if err := cs.SaveProjectMemory(context.Background(), projectID, "architecture",
		"Use hexagonal architecture", nil); err != nil {
		t.Fatalf("SaveProjectMemory: %v", err)
	}

	entries, err := cs.GetProjectContext(context.Background(), projectID, 10)
	if err != nil {
		t.Fatalf("GetProjectContext: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Type != "architecture" {
		t.Errorf("expected type %q, got %q", "architecture", entries[0].Type)
	}
}

func TestContextStore_BuildProjectSystemPrompt(t *testing.T) {
	d := openTestDB(t)
	projectID := createProject(t, d, "P7")
	cs := NewContextStore(d, nil, 100)

	cs.SaveProjectMemory(context.Background(), projectID, "architecture", "Use clean arch", nil)
	cs.SaveProjectMemory(context.Background(), projectID, "design_note", "REST API with JSON", nil)

	prompt, err := cs.BuildProjectSystemPrompt(context.Background(), projectID, 10)
	if err != nil {
		t.Fatalf("BuildProjectSystemPrompt: %v", err)
	}
	if prompt == "" {
		t.Error("expected non-empty system prompt")
	}
	if len(prompt) < 20 {
		t.Errorf("prompt too short: %q", prompt)
	}
}

func TestContextStore_BuildProjectSystemPrompt_Empty(t *testing.T) {
	d := openTestDB(t)
	projectID := createProject(t, d, "P8")
	cs := NewContextStore(d, nil, 100)

	prompt, err := cs.BuildProjectSystemPrompt(context.Background(), projectID, 10)
	if err != nil {
		t.Fatalf("BuildProjectSystemPrompt: %v", err)
	}
	if prompt != "" {
		t.Errorf("expected empty prompt for empty project, got %q", prompt)
	}
}
