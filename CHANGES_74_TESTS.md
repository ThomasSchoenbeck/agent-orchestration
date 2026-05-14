# #74 Tests: Updated for Role Resolution Feature

## Problem
The new role resolution feature in #74 loads both agents and roles in parallel, doubling the number of API calls. The original tests expected 1 fetch call per load, but now there are 2 (agents + roles).

## Test Failures Fixed

### 1. "Refresh button triggers reload"
**Before:** Expected 2 fetch calls (1 initial + 1 on refresh)
**After:** Expects 4 fetch calls (2 on initial load + 2 on refresh)
- Initial load calls: `/api/agents` and `/api/roles`
- Refresh click calls: `/api/agents` and `/api/roles` again

### 2. "sets up polling interval on mount"
**Before:** Expected 1 call initially, then 2 total after poll
**After:** Expects 2 calls initially, then 4 total after poll
- Initial load: 2 calls (agents + roles)
- After 10s poll trigger: 2 more calls = 4 total

## Changes Made

### 1. Updated stubFetch() to distinguish endpoints
```javascript
function stubFetch(agents = AGENTS, roles = ROLES) {
  vi.stubGlobal('fetch', vi.fn((url) => {
    let data = agents
    if (url === '/api/roles') {
      data = roles
    }
    return Promise.resolve({
      ok: true, status: 200,
      json: () => Promise.resolve(data),
    })
  }))
}
```
- Returns agent data for `/api/agents` calls
- Returns role data for `/api/roles` calls
- Enables proper testing of both endpoints

### 2. Added ROLES test data
```javascript
const ROLES = [
  { id: 'r1', name: 'worker', label: 'Worker', enabled: true },
  { id: 'r2', name: 'reviewer', label: 'Reviewer', enabled: true },
  { id: 'r3', name: 'orchestrator', label: 'Orchestrator', enabled: true },
]
```

### 3. Updated API call expectations
- "calls GET /api/agents and /api/roles on mount" — verifies both endpoints are called
- Refresh button test expects 4 total calls (2+2)
- Polling interval test expects 4 total calls after one poll interval

### 4. Added new feature tests
- **"shows resolved role definitions"** — verifies role labels are displayed
- **"shows warning for undefined roles"** — verifies "no definition" badge appears for unknown roles

## Test Coverage
✅ API calls happen for both agents and roles
✅ Refresh button reloads both endpoints
✅ Polling interval reloads both endpoints
✅ Resolved role definitions are displayed
✅ Warning badges show for undefined roles
✅ Empty state handling preserved
✅ Response shape handling preserved

## All Tests Now Passing
- Initial API expectations: 2 calls (agents + roles)
- Refresh button: 4 total calls
- Polling interval: 4 total calls
- Feature tests: Role resolution and warning display
