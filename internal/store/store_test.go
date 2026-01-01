package store

import "testing"

func TestStore_SessionCRUD(t *testing.T) {
	s := New()
	now := int64(1000)

	sess, created, err := s.GetOrCreateSession("u1", "tag1", "m1", nil, nil, now)
	if err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}
	if !created {
		t.Fatalf("expected created")
	}
	if sess.Tag != "tag1" {
		t.Fatalf("expected tag1, got %q", sess.Tag)
	}

	list := s.ListSessions("u1")
	if len(list) != 1 {
		t.Fatalf("expected 1 session, got %d", len(list))
	}

	if !s.DeleteSession("u1", sess.ID, now+1) {
		t.Fatalf("expected delete true")
	}
	list = s.ListSessions("u1")
	if len(list) != 0 {
		t.Fatalf("expected 0 sessions, got %d", len(list))
	}
}

func TestStore_Messages(t *testing.T) {
	s := New()
	now := int64(1000)
	sess, _, err := s.GetOrCreateSession("u1", "tag1", "m1", nil, nil, now)
	if err != nil {
		t.Fatalf("GetOrCreateSession: %v", err)
	}

	msg1, err := s.AppendMessage("u1", sess.ID, "c1", now)
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	msg2, err := s.AppendMessage("u1", sess.ID, "c2", now)
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	if msg2.Seq <= msg1.Seq {
		t.Fatalf("expected seq to increase")
	}

	msgs, err := s.ListMessages("u1", sess.ID, msg1.Seq, 100)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected 1 msg after, got %d", len(msgs))
	}
}

func TestStore_AuthRequestAuthorize(t *testing.T) {
	s := New()
	now := int64(1000)
	s.UpsertAuthRequest("pk", true, now)
	_, ok := s.GetAuthRequest("pk")
	if !ok {
		t.Fatalf("expected auth request")
	}
	_, ok = s.AuthorizeAuthRequest("pk", "resp", "acct", "tok", now+1)
	if !ok {
		t.Fatalf("expected authorize ok")
	}
	req, _ := s.GetAuthRequest("pk")
	if req.Response != "resp" || req.Token != "tok" {
		t.Fatalf("unexpected request state")
	}
}

func TestStore_MachineOwnership(t *testing.T) {
	s := New()
	now := int64(1000)
	_, _, err := s.UpsertMachine("u1", "m1", "meta", nil, nil, now)
	if err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}
	_, _, err = s.UpsertMachine("u2", "m1", "meta", nil, nil, now)
	if err == nil {
		t.Fatalf("expected error")
	}
}
