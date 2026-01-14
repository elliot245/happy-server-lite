package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore_MachinesPersistence_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "machines-state.json")

	s1 := NewWithOptions(Options{MachinesStateFile: stateFile})
	now := int64(1000)
	_, created, err := s1.UpsertMachine("u1", "m1", "meta", nil, nil, now)
	if err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}
	if !created {
		t.Fatalf("expected machine created")
	}

	info, err := os.Stat(stateFile)
	if err != nil {
		t.Fatalf("expected state file written: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected state file mode 0600, got %o", info.Mode().Perm())
	}

	s2 := NewWithOptions(Options{MachinesStateFile: stateFile})
	got := s2.ListMachines("u1")
	if len(got) != 1 {
		t.Fatalf("expected 1 machine, got %d", len(got))
	}
	if got[0].ID != "m1" || got[0].UserID != "u1" || got[0].Metadata != "meta" {
		t.Fatalf("unexpected machine loaded: %+v", got[0])
	}

	other := s2.ListMachines("u2")
	if len(other) != 0 {
		t.Fatalf("expected 0 machines for other user")
	}
}

func TestStore_MachinesPersistence_PersistsUpdates(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "machines-state.json")

	s1 := NewWithOptions(Options{MachinesStateFile: stateFile})
	now := int64(1000)
	createdMachine, created, err := s1.UpsertMachine("u1", "m1", "meta", nil, nil, now)
	if err != nil {
		t.Fatalf("UpsertMachine: %v", err)
	}
	if !created {
		t.Fatalf("expected machine created")
	}

	status, version, value := s1.UpdateMachineMetadata("u1", "m1", createdMachine.MetadataVersion, "meta2", now+1)
	if status != "success" {
		t.Fatalf("expected success, got %q", status)
	}
	if version != createdMachine.MetadataVersion+1 {
		t.Fatalf("unexpected version: %d", version)
	}
	if value != "meta2" {
		t.Fatalf("unexpected value: %q", value)
	}

	s2 := NewWithOptions(Options{MachinesStateFile: stateFile})
	got := s2.ListMachines("u1")
	if len(got) != 1 {
		t.Fatalf("expected 1 machine, got %d", len(got))
	}
	if got[0].Metadata != "meta2" {
		t.Fatalf("expected updated metadata, got %q", got[0].Metadata)
	}
	if got[0].MetadataVersion != createdMachine.MetadataVersion+1 {
		t.Fatalf("expected updated metadata version, got %d", got[0].MetadataVersion)
	}
}
