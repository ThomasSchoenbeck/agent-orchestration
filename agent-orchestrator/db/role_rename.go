package db

import (
	"context"
	"database/sql"
	"strings"
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

// ResolveRoleRefs maps each ref to a role-definition id where one exists
// (matching by name or id), leaving refs with no role definition unchanged.
// Used at write boundaries (agent registration, task creation) to store ids
// while callers may still submit human-readable names (Task 9, Phase 3).
func (d *Database) ResolveRoleRefs(ctx context.Context, refs []string) ([]string, error) {
	if len(refs) == 0 {
		return refs, nil
	}
	ph := strings.Repeat("?,", len(refs))
	ph = ph[:len(ph)-1]
	args := make([]interface{}, 0, len(refs)*2)
	for _, r := range refs {
		args = append(args, r)
	}
	for _, r := range refs {
		args = append(args, r)
	}
	rows, err := d.db.QueryContext(ctx,
		"SELECT id, name FROM agent_role_definitions WHERE id IN ("+ph+") OR name IN ("+ph+")", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	nameToID := map[string]string{}
	idSet := map[string]bool{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		nameToID[name] = id
		idSet[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]string, len(refs))
	for i, r := range refs {
		switch {
		case idSet[r]:
			out[i] = r // already an id
		case nameToID[r] != "":
			out[i] = nameToID[r] // name → id
		default:
			out[i] = r // no role definition; leave as-is
		}
	}
	return out, nil
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

	// Providers: provider-level roles array (Phase 5, T5.1 removed model-level roles).
	rows, err := d.db.QueryContext(ctx, `SELECT id, roles FROM providers`)
	if err != nil {
		return err
	}
	type provUpdate struct {
		id    string
		roles []string
	}
	var provUpdates []provUpdate
	for rows.Next() {
		var id, rolesJSON string
		if err := rows.Scan(&id, &rolesJSON); err != nil {
			rows.Close()
			return err
		}
		roles := unmarshalJSONStringSlice(rolesJSON)
		if newRoles, dirty := replaceInSlice(roles, oldName, newName); dirty {
			provUpdates = append(provUpdates, provUpdate{id: id, roles: newRoles})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, u := range provUpdates {
		if _, err := d.db.ExecContext(ctx,
			`UPDATE providers SET roles=? WHERE id=?`,
			marshalJSONArray(u.roles), u.id); err != nil {
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
