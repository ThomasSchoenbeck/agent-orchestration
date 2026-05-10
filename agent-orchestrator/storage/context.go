package storage

import (
	"context"
	"fmt"
	"log"
	"strings"

	"agent-orchestrator/db"
	"agent-orchestrator/llm"
)

// DefaultMaxProjectContextItems is the default cap on project context entries.
const DefaultMaxProjectContextItems = 200

// ContextStore wraps the database context operations and adds semantic search
// (when an embedder is available) and project memory management.
type ContextStore struct {
	db       *db.Database
	embedder llm.LLMProvider // optional; nil means keyword-only search
	maxItems int
}

// NewContextStore creates a ContextStore.
// embedder may be nil — in that case all queries fall back to keyword search.
// maxItems controls project context pruning (use DefaultMaxProjectContextItems if 0).
func NewContextStore(database *db.Database, embedder llm.LLMProvider, maxItems int) *ContextStore {
	if maxItems <= 0 {
		maxItems = DefaultMaxProjectContextItems
	}
	return &ContextStore{
		db:       database,
		embedder: embedder,
		maxItems: maxItems,
	}
}

// Save persists a context entry.
// If an embedder is configured the content is embedded before saving.
func (cs *ContextStore) Save(ctx context.Context, entry *db.ContextEntry) error {
	if cs.embedder != nil {
		emb, err := cs.embedder.Embed(ctx, llm.EmbedRequest{Input: []string{entry.Content}})
		if err != nil {
			// Non-fatal: log and continue without embedding.
			log.Printf("[storage] embedding failed for entry (type=%s): %v", entry.Type, err)
		} else if len(emb.Embeddings) > 0 {
			entry.Embedding = emb.Embeddings[0]
		}
	}

	if err := cs.db.CreateContextEntry(ctx, entry); err != nil {
		return fmt.Errorf("save context entry: %w", err)
	}

	// Prune if project context is set and exceeds the limit.
	if entry.ProjectID != "" {
		if err := cs.db.PruneProjectContext(ctx, entry.ProjectID, cs.maxItems); err != nil {
			log.Printf("[storage] prune project context for %s: %v", entry.ProjectID, err)
		}
	}
	return nil
}

// Query returns the most relevant context entries for a project.
// If an embedder is configured, semantic similarity is used.
// Otherwise, plain keyword search is performed (fallback).
//
// topK controls the maximum number of results.
func (cs *ContextStore) Query(ctx context.Context, projectID, query string, topK int) ([]*db.ContextEntry, error) {
	if topK <= 0 {
		topK = 10
	}

	// Semantic path: embed the query, rank by cosine similarity.
	if cs.embedder != nil && query != "" {
		emb, err := cs.embedder.Embed(ctx, llm.EmbedRequest{Input: []string{query}})
		if err == nil && len(emb.Embeddings) > 0 {
			return cs.semanticQuery(ctx, projectID, emb.Embeddings[0], topK)
		}
		// Fall through to keyword search on embedding failure.
		log.Printf("[storage] semantic query failed, falling back to keyword search: %v", err)
	}

	// Keyword fallback.
	return cs.db.QueryContext(ctx, projectID, query, topK)
}

// semanticQuery fetches all embedded entries for the project, ranks them
// by cosine similarity to the query vector, and returns the top-K.
func (cs *ContextStore) semanticQuery(ctx context.Context, projectID string, queryVec []float32, topK int) ([]*db.ContextEntry, error) {
	entries, err := cs.db.GetContextEntriesWithEmbeddings(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("fetch embedded context entries: %w", err)
	}
	if len(entries) == 0 {
		// Fall back to keyword search when no embeddings exist yet.
		return cs.db.QueryContext(ctx, projectID, "", topK)
	}

	ranked := make([]RankedEntry, len(entries))
	for i, e := range entries {
		ranked[i] = RankedEntry{
			ID:        e.ID,
			ProjectID: e.ProjectID,
			TaskID:    e.TaskID,
			Type:      e.Type,
			Content:   e.Content,
			Embedding: e.Embedding,
		}
	}

	top := RankBySimilarity(queryVec, ranked, topK)

	// Convert back to *db.ContextEntry by looking up from the original slice.
	entryMap := make(map[string]*db.ContextEntry, len(entries))
	for _, e := range entries {
		entryMap[e.ID] = e
	}

	result := make([]*db.ContextEntry, 0, len(top))
	for _, r := range top {
		if e, ok := entryMap[r.ID]; ok {
			result = append(result, e)
		}
	}
	return result, nil
}

// SaveProjectMemory is a convenience wrapper for storing a project-level
// memory item (architecture decision, design note, etc.).
func (cs *ContextStore) SaveProjectMemory(ctx context.Context, projectID, entryType, content string, metadata map[string]interface{}) error {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	entry := &db.ContextEntry{
		ProjectID: projectID,
		Type:      entryType, // e.g. "architecture", "design_note", "test_result", "diff"
		Content:   content,
		Metadata:  metadata,
	}
	return cs.Save(ctx, entry)
}

// GetProjectContext returns recent project-level context entries for assembly
// into an agent system prompt. Returns at most maxItems entries.
func (cs *ContextStore) GetProjectContext(ctx context.Context, projectID string, maxItems int) ([]*db.ContextEntry, error) {
	if maxItems <= 0 {
		maxItems = 20
	}
	return cs.db.QueryContext(ctx, projectID, "", maxItems)
}

// BuildProjectSystemPrompt assembles a compact system prompt section from
// project memory entries (e.g. to inject into an agent's LLM call).
func (cs *ContextStore) BuildProjectSystemPrompt(ctx context.Context, projectID string, maxItems int) (string, error) {
	entries, err := cs.GetProjectContext(ctx, projectID, maxItems)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("## Project Context\n\n")
	for _, e := range entries {
		sb.WriteString(fmt.Sprintf("### %s\n%s\n\n", e.Type, e.Content))
	}
	return sb.String(), nil
}
