package db

import (
	"context"
	"time"
)

// StateTransition is one recorded state change for a task.
type StateTransition struct {
	ID           string    `json:"id"`
	TaskID       string    `json:"task_id"`
	FromState    string    `json:"from_state"`
	ToState      string    `json:"to_state"`
	ActorAgentID string    `json:"actor_agent_id,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// ListStateTransitions returns all state transitions for a task, oldest first.
func (d *Database) ListStateTransitions(ctx context.Context, taskID string) ([]*StateTransition, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT id, task_id, COALESCE(from_state,''), to_state,
		        COALESCE(actor_agent_id,''), COALESCE(reason,''), created_at
		 FROM task_state_transitions
		 WHERE task_id=? ORDER BY created_at ASC`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*StateTransition
	for rows.Next() {
		var st StateTransition
		var createdAt string
		if err := rows.Scan(
			&st.ID, &st.TaskID, &st.FromState, &st.ToState,
			&st.ActorAgentID, &st.Reason, &createdAt,
		); err != nil {
			return nil, err
		}
		st.CreatedAt = parseTime(createdAt)
		out = append(out, &st)
	}
	return out, rows.Err()
}
