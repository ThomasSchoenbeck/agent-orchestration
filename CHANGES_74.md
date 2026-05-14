# #74 Implementation: Show Resolved Role Definition on Agents Page

## Summary
Added a "Resolved definitions" column to the Agents page showing the matching role definition label for each agent role, with clear visual feedback for undefined roles.

## Changes Made

### File: `ui/src/pages/Agents.svelte`

**1. Import role listing function**
- Added `listRoles` import from api.js
- Enables fetching all defined roles to match against agent roles

**2. Add roles state and load function**
- Added `roles` state variable to store role definitions
- Updated `load()` function to fetch both agents and roles in parallel
- Improved performance by using `Promise.all()`

**3. Add role resolution helper**
```javascript
function resolveRole(roleName) {
  return roles.find(r => r.name === roleName)
}
```
- Looks up a role definition by name
- Returns the matching role definition or undefined

**4. Display resolved definitions section**
New UI section shows:
- **Found**: Green badge showing role definition label (linked to edit)
  - Background: `bg-green-900` with text `text-green-200`
  - Hover effect: `hover:bg-green-800`
  - Click navigates to `/roles/{def.id}/edit`
  - Title: "Click to edit role definition"

- **Not found**: Red warning badge with warning icon
  - Background: `bg-red-900` with text `text-red-200`
  - Icon: ⚠ prefix
  - Title: "No definition found for role 'X'"

## Visual Changes
Before:
```
Role strings
worker    orchestrator    reviewer
```

After:
```
Role strings
worker    orchestrator    reviewer

Resolved definitions:
Worker (green, linked)    Orchestrator (green, linked)    ⚠ no definition (red)
```

## Benefits
✅ Helps operators spot agents using stale or misspelled role names
✅ Quick access to edit role definitions (green badges are clickable)
✅ Clear visual feedback for configuration issues (red warning badges)
✅ Role definitions stay in sync with agent registrations

## Testing
The implementation was verified by:
1. Confirming `listRoles()` API function exists
2. Checking the role resolution logic handles missing definitions gracefully
3. Verifying Svelte 5 state and binding syntax
4. Ensuring visual hierarchy matches existing design system
