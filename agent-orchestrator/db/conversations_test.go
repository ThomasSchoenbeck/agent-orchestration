package db_test

import (
	"context"
	"testing"

	"agent-orchestrator/db"
)

func TestConversationCRUD(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	c := &db.Conversation{Title: "Chat 1", ProviderID: "p1"}
	if err := d.CreateConversation(ctx, c); err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}
	if c.ID == "" {
		t.Fatal("CreateConversation did not assign an id")
	}

	got, err := d.GetConversation(ctx, c.ID)
	if err != nil {
		t.Fatalf("GetConversation: %v", err)
	}
	if got.Title != "Chat 1" || got.ProviderID != "p1" {
		t.Errorf("GetConversation = %+v, want title=Chat 1 provider=p1", got)
	}

	got.Title = "Chat 1 renamed"
	if err := d.UpdateConversation(ctx, got); err != nil {
		t.Fatalf("UpdateConversation: %v", err)
	}
	if re, _ := d.GetConversation(ctx, c.ID); re.Title != "Chat 1 renamed" {
		t.Errorf("update not persisted: %q", re.Title)
	}

	list, err := d.ListConversations(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListConversations: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("ListConversations = %d, want 1", len(list))
	}

	if err := d.DeleteConversation(ctx, c.ID); err != nil {
		t.Fatalf("DeleteConversation: %v", err)
	}
	if _, err := d.GetConversation(ctx, c.ID); err == nil {
		t.Error("GetConversation after delete should error")
	}
}

func TestConversationMessagesAndChatLog(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	c := &db.Conversation{Title: "C"}
	if err := d.CreateConversation(ctx, c); err != nil {
		t.Fatalf("CreateConversation: %v", err)
	}

	if err := d.AddMessage(ctx, &db.Message{ConversationID: c.ID, Role: "user", Content: "hello"}); err != nil {
		t.Fatalf("AddMessage user: %v", err)
	}
	if err := d.AddMessage(ctx, &db.Message{ConversationID: c.ID, Role: "assistant", Content: "hi there", InputTokens: 3, OutputTokens: 2}); err != nil {
		t.Fatalf("AddMessage assistant: %v", err)
	}

	msgs, err := d.ListMessages(ctx, c.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("ListMessages = %d, want 2", len(msgs))
	}

	conv, withMsgs, err := d.GetConversationWithMessages(ctx, c.ID, 10)
	if err != nil {
		t.Fatalf("GetConversationWithMessages: %v", err)
	}
	if conv.ID != c.ID || len(withMsgs) != 2 {
		t.Errorf("GetConversationWithMessages: conv=%s msgs=%d", conv.ID, len(withMsgs))
	}

	chatLog, err := d.ListChatLog(ctx, 10)
	if err != nil {
		t.Fatalf("ListChatLog: %v", err)
	}
	if len(chatLog) == 0 {
		t.Error("ListChatLog should include the messages just added")
	}
}
