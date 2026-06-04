package db

import (
	"context"
	"database/sql"
)

// RoleIDByName returns the role-definition id for a role name, or "" if no
// role definition exists with that name.
func (d *Database) RoleIDByName(ctx context.Context, name string) (string, error) {
	var id string
	err := d.db.QueryRowContext(ctx,
		`SELECT id FROM agent_role_definitions WHERE name=?`, name).Scan(&id)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return id, err
}

// RoleNameByID returns the role name for a role-definition id, or "" if none.
func (d *Database) RoleNameByID(ctx context.Context, id string) (string, error) {
	var name string
	err := d.db.QueryRowContext(ctx,
		`SELECT name FROM agent_role_definitions WHERE id=?`, id).Scan(&name)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return name, err
}

// replaceInSlice returns a copy of arr with every occurrence of old replaced by
// neu, plus whether any replacement happened.
func replaceInSlice(arr []string, old, neu string) ([]string, bool) {
	changed := false
	out := make([]string, len(arr))
	for i, v := range arr {
		if v == old {
			out[i] = neu
			changed = true
		} else {
			out[i] = v
		}
	}
	return out, changed
}

// RenameRoleReferences rewrites every stored reference to a role name from
// oldName to newName across providers (roles + per-model roles), agents
// (roles + start_roles), agent templates (roles), and tasks (role +
// review_role). Surgical, column-only updates (no full-row round-trips).
//
// Intended to run right after the role definition itself is renamed so the
// whole reference graph stays consistent. Sequential (not transactional) —
// acceptable for this local single-writer DB.
func (d *Database) RenameRoleReferences(ctx context.Context, oldName, newName string) error {
	if oldName == newName || oldName == "" || newName == "" {
		return nil
	}

	// Providers: roles array + each model's roles array.
	rows, err := d.db.QueryContext(ctx, `SELECT id, roles, models FROM providers`)
	if err != nil {
		return err
	}
	type provUpdate struct {
		id     string
		roles  []string
		models []ProviderModel
	}
	var provUpdates []provUpdate
	for rows.Next() {
		var id, rolesJSON, modelsJSON string
		if err := rows.Scan(&id, &rolesJSON, &modelsJSON); err != nil {
			rows.Close()
			return err
		}
		roles := unmarshalJSONStringSlice(rolesJSON)
		models := unmarshalProviderModels(modelsJSON)
		newRoles, dirty := replaceInSlice(roles, oldName, newName)
		for i := range models {
			if mr, ch := replaceInSlice(models[i].Roles, oldName, newName); ch {
				models[i].Roles = mr
				dirty = true
			}
		}
		if dirty {
			provUpdates = append(provUpdates, provUpdate{id: id, roles: newRoles, models: models})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, u := range provUpdates {
		if _, err := d.db.ExecContext(ctx,
			`UPDATE providers SET roles=?, models=? WHERE id=?`,
			marshalJSONArray(u.roles), marshalJSONArray(u.models), u.id); err != nil {
			return err
		}
	}

	// Agents: roles + start_roles.
	rows, err = d.db.QueryContext(ctx, `SELECT id, roles, start_roles FROM agents`)
	if err != nil {
		return err
	}
	type agentUpdate struct {
		id         string
		roles      []string
		startRoles []string
	}
	var agentUpdates []agentUpdate
	for rows.Next() {
		var id, rolesJSON, startRolesJSON string
		if err := rows.Scan(&id, &rolesJSON, &startRolesJSON); err != nil {
			rows.Close()
			return err
		}
		roles, d1 := replaceInSlice(unmarshalJSONStringSlice(rolesJSON), oldName, newName)
		startRoles, d2 := replaceInSlice(unmarshalJSONStringSlice(startRolesJSON), oldName, newName)
		if d1 || d2 {
			agentUpdates = append(agentUpdates, agentUpdate{id: id, roles: roles, startRoles: startRoles})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, u := range agentUpdates {
		if _, err := d.db.ExecContext(ctx,
			`UPDATE agents SET roles=?, start_roles=? WHERE id=?`,
			marshalJSONArray(u.roles), marshalJSONArray(u.startRoles), u.id); err != nil {
			return err
		}
	}

	// Agent templates: roles.
	rows, err = d.db.QueryContext(ctx, `SELECT id, roles FROM agent_templates`)
	if err != nil {
		return err
	}
	type tplUpdate struct {
		id    string
		roles []string
	}
	var tplUpdates []tplUpdate
	for rows.Next() {
		var id, rolesJSON string
		if err := rows.Scan(&id, &rolesJSON); err != nil {
			rows.Close()
			return err
		}
		if roles, ch := replaceInSlice(unmarshalJSONStringSlice(rolesJSON), oldName, newName); ch {
			tplUpdates = append(tplUpdates, tplUpdate{id: id, roles: roles})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, u := range tplUpdates {
		if _, err := d.db.ExecContext(ctx,
			`UPDATE agent_templates SET roles=? WHERE id=?`,
			marshalJSONArray(u.roles), u.id); err != nil {
			return err
		}
	}

	// Tasks: role + review_role columns (plain strings).
	if _, err := d.db.ExecContext(ctx,
		`UPDATE tasks SET role=? WHERE role=?`, newName, oldName); err != nil {
		return err
	}
	if _, err := d.db.ExecContext(ctx,
		`UPDATE tasks SET review_role=? WHERE review_role=?`, newName, oldName); err != nil {
		return err
	}
	return nil
}
