package logging_test

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"agent-orchestrator/db"
	"agent-orchestrator/logging"
)

// openMetricsDB opens a temporary SQLite database and returns a Collector.
func openMetricsDB(t *testing.T) (*db.Database, *logging.Collector) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "metrics_test.db")
	d, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d, logging.NewCollector(d)
}

// insertMetric is a helper to create a Metric row.
func insertMetric(t *testing.T, d *db.Database, m *db.Metric) {
	t.Helper()
	if err := d.CreateMetric(context.Background(), m); err != nil {
		t.Fatalf("CreateMetric: %v", err)
	}
}

// --- TokenMetrics ---

func TestTokenMetrics_Empty(t *testing.T) {
	_, col := openMetricsDB(t)
	tm, err := col.TokenMetrics(context.Background())
	if err != nil {
		t.Fatalf("TokenMetrics: %v", err)
	}
	if tm.TotalInputTokens != 0 {
		t.Errorf("expected 0 input tokens, got %d", tm.TotalInputTokens)
	}
	if tm.TotalOutputTokens != 0 {
		t.Errorf("expected 0 output tokens, got %d", tm.TotalOutputTokens)
	}
	if tm.TotalTokens != 0 {
		t.Errorf("expected 0 total tokens, got %d", tm.TotalTokens)
	}
	if len(tm.ByProject) != 0 {
		t.Errorf("expected no project rows, got %d", len(tm.ByProject))
	}
	if len(tm.ByAgent) != 0 {
		t.Errorf("expected no agent rows, got %d", len(tm.ByAgent))
	}
}

func TestTokenMetrics_SingleEntry(t *testing.T) {
	d, col := openMetricsDB(t)

	// Create an agent so the agent_id reference is valid.
	agent := &db.Agent{Name: "tester", Roles: []string{"worker"}}
	if err := d.CreateAgent(context.Background(), agent); err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}

	insertMetric(t, d, &db.Metric{
		AgentID:      agent.ID,
		Model:        "gpt-4o",
		InputTokens:  100,
		OutputTokens: 50,
		Success:      true,
	})

	tm, err := col.TokenMetrics(context.Background())
	if err != nil {
		t.Fatalf("TokenMetrics: %v", err)
	}
	if tm.TotalInputTokens != 100 {
		t.Errorf("expected 100 input tokens, got %d", tm.TotalInputTokens)
	}
	if tm.TotalOutputTokens != 50 {
		t.Errorf("expected 50 output tokens, got %d", tm.TotalOutputTokens)
	}
	if tm.TotalTokens != 150 {
		t.Errorf("expected 150 total tokens, got %d", tm.TotalTokens)
	}

	if len(tm.ByAgent) != 1 {
		t.Fatalf("expected 1 agent row, got %d", len(tm.ByAgent))
	}
	row := tm.ByAgent[0]
	if row.AgentID != agent.ID {
		t.Errorf("agent ID mismatch: want %s, got %s", agent.ID, row.AgentID)
	}
	if row.InputTokens != 100 || row.OutputTokens != 50 || row.TotalTokens != 150 {
		t.Errorf("agent token breakdown wrong: %+v", row)
	}
}

func TestTokenMetrics_ByProject(t *testing.T) {
	d, col := openMetricsDB(t)
	ctx := context.Background()

	// Create two projects and tasks.
	p1 := &db.Project{Name: "Project-A"}
	p2 := &db.Project{Name: "Project-B"}
	if err := d.CreateProject(ctx, p1); err != nil {
		t.Fatalf("create p1: %v", err)
	}
	if err := d.CreateProject(ctx, p2); err != nil {
		t.Fatalf("create p2: %v", err)
	}

	task1 := &db.Task{ProjectID: p1.ID, Type: "implement", Role: "worker"}
	task2 := &db.Task{ProjectID: p2.ID, Type: "implement", Role: "worker"}
	if err := d.CreateTask(ctx, task1); err != nil {
		t.Fatalf("create task1: %v", err)
	}
	if err := d.CreateTask(ctx, task2); err != nil {
		t.Fatalf("create task2: %v", err)
	}

	// Metrics tied to each task.
	insertMetric(t, d, &db.Metric{
		TaskID:       task1.ID,
		InputTokens:  200,
		OutputTokens: 80,
		Success:      true,
	})
	insertMetric(t, d, &db.Metric{
		TaskID:       task2.ID,
		InputTokens:  300,
		OutputTokens: 120,
		Success:      true,
	})

	tm, err := col.TokenMetrics(ctx)
	if err != nil {
		t.Fatalf("TokenMetrics: %v", err)
	}

	if tm.TotalInputTokens != 500 {
		t.Errorf("expected 500 input tokens total, got %d", tm.TotalInputTokens)
	}
	if tm.TotalOutputTokens != 200 {
		t.Errorf("expected 200 output tokens total, got %d", tm.TotalOutputTokens)
	}

	if len(tm.ByProject) != 2 {
		t.Fatalf("expected 2 project rows, got %d", len(tm.ByProject))
	}

	// Build a map for easy lookup.
	byProj := make(map[string]logging.ProjectTokenMetrics)
	for _, pr := range tm.ByProject {
		byProj[pr.ProjectID] = pr
	}

	if row, ok := byProj[p1.ID]; !ok {
		t.Errorf("missing project-A in ByProject")
	} else if row.InputTokens != 200 || row.OutputTokens != 80 {
		t.Errorf("project-A wrong tokens: %+v", row)
	}

	if row, ok := byProj[p2.ID]; !ok {
		t.Errorf("missing project-B in ByProject")
	} else if row.InputTokens != 300 || row.OutputTokens != 120 {
		t.Errorf("project-B wrong tokens: %+v", row)
	}
}

func TestTokenMetrics_MultipleAgents(t *testing.T) {
	d, col := openMetricsDB(t)
	ctx := context.Background()

	agents := make([]*db.Agent, 3)
	for i := range agents {
		a := &db.Agent{Name: fmt.Sprintf("agent-%d", i), Roles: []string{"worker"}}
		if err := d.CreateAgent(ctx, a); err != nil {
			t.Fatalf("CreateAgent: %v", err)
		}
		agents[i] = a
	}

	inputs := []int{10, 20, 30}
	outputs := []int{5, 10, 15}
	for i, a := range agents {
		insertMetric(t, d, &db.Metric{
			AgentID:      a.ID,
			InputTokens:  inputs[i],
			OutputTokens: outputs[i],
		})
	}

	tm, err := col.TokenMetrics(ctx)
	if err != nil {
		t.Fatalf("TokenMetrics: %v", err)
	}
	if tm.TotalInputTokens != 60 {
		t.Errorf("expected 60 input tokens, got %d", tm.TotalInputTokens)
	}
	if len(tm.ByAgent) != 3 {
		t.Errorf("expected 3 agent rows, got %d", len(tm.ByAgent))
	}
}

// --- CostMetrics ---

func TestCostMetrics_Empty(t *testing.T) {
	_, col := openMetricsDB(t)
	cm, err := col.CostMetrics(context.Background())
	if err != nil {
		t.Fatalf("CostMetrics: %v", err)
	}
	if cm.TotalCost != 0.0 {
		t.Errorf("expected 0 cost, got %f", cm.TotalCost)
	}
	if len(cm.ByProject) != 0 {
		t.Errorf("expected no project rows, got %d", len(cm.ByProject))
	}
	if len(cm.ByAgent) != 0 {
		t.Errorf("expected no agent rows, got %d", len(cm.ByAgent))
	}
}

func TestCostMetrics_MultipleAgents(t *testing.T) {
	d, col := openMetricsDB(t)
	ctx := context.Background()

	costs := []float64{0.10, 0.25, 0.05}
	for i, c := range costs {
		agent := &db.Agent{Name: fmt.Sprintf("cost-agent-%d", i), Roles: []string{"worker"}}
		if err := d.CreateAgent(ctx, agent); err != nil {
			t.Fatalf("CreateAgent[%d]: %v", i, err)
		}
		insertMetric(t, d, &db.Metric{
			AgentID: agent.ID,
			Cost:    c,
			Success: true,
		})
	}

	cm, err := col.CostMetrics(ctx)
	if err != nil {
		t.Fatalf("CostMetrics: %v", err)
	}

	const wantTotal = 0.40
	if cm.TotalCost < wantTotal-0.001 || cm.TotalCost > wantTotal+0.001 {
		t.Errorf("expected total cost %.4f, got %.4f", wantTotal, cm.TotalCost)
	}

	if len(cm.ByAgent) != 3 {
		t.Errorf("expected 3 agent rows, got %d", len(cm.ByAgent))
	}

	// Verify descending order (highest cost first).
	if cm.ByAgent[0].Cost < cm.ByAgent[1].Cost {
		t.Errorf("expected descending order: first cost %.4f < second cost %.4f",
			cm.ByAgent[0].Cost, cm.ByAgent[1].Cost)
	}
}

func TestCostMetrics_ByProject(t *testing.T) {
	d, col := openMetricsDB(t)
	ctx := context.Background()

	p := &db.Project{Name: "CostProject"}
	if err := d.CreateProject(ctx, p); err != nil {
		t.Fatalf("create project: %v", err)
	}
	task := &db.Task{ProjectID: p.ID, Type: "implement", Role: "worker"}
	if err := d.CreateTask(ctx, task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	// Two metrics for the same project task.
	insertMetric(t, d, &db.Metric{TaskID: task.ID, Cost: 0.15})
	insertMetric(t, d, &db.Metric{TaskID: task.ID, Cost: 0.10})

	cm, err := col.CostMetrics(ctx)
	if err != nil {
		t.Fatalf("CostMetrics: %v", err)
	}

	if len(cm.ByProject) != 1 {
		t.Fatalf("expected 1 project row, got %d", len(cm.ByProject))
	}
	pr := cm.ByProject[0]
	if pr.ProjectID != p.ID {
		t.Errorf("project ID mismatch: want %s, got %s", p.ID, pr.ProjectID)
	}
	const wantCost = 0.25
	if pr.Cost < wantCost-0.001 || pr.Cost > wantCost+0.001 {
		t.Errorf("expected project cost %.4f, got %.4f", wantCost, pr.Cost)
	}
	if pr.Tasks != 2 {
		t.Errorf("expected 2 tasks, got %d", pr.Tasks)
	}
}
