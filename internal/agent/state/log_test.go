package state

import (
	"encoding/binary"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

func TestAgentLogQueueIsIdempotentAndAcknowledged(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "agent")
	store, err := Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	event := testLogEvent(time.Now().UTC(), "request failed")
	if err := store.EnqueueAgentLog(event); err != nil {
		t.Fatal(err)
	}
	if err := store.EnqueueAgentLog(event); err != nil {
		t.Fatal(err)
	}
	batch, err := store.PendingAgentLogs(64, 8*1024*1024)
	if err != nil || len(batch) != 1 || batch[0].ID != event.ID {
		t.Fatalf("pending logs = %#v, %v", batch, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.AcknowledgeAgentLogs([]string{event.ID}); err != nil {
		t.Fatal(err)
	}
	batch, err = store.PendingAgentLogs(64, 8*1024*1024)
	if err != nil || len(batch) != 0 {
		t.Fatalf("acknowledged logs remain queued: %#v, %v", batch, err)
	}
}

func TestAgentLogQueueEvictsOldestAndKeepsRevisionSafeGaps(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agent"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	start := time.Now().UTC().Add(-time.Hour)
	seedLogQueue(t, store, MaxAgentLogEvents, start)
	newest := testLogEvent(start.Add(MaxAgentLogEvents*time.Second), "newest")
	if err := store.EnqueueAgentLog(newest); err != nil {
		t.Fatal(err)
	}
	firstBatch, err := store.PendingAgentLogs(64, 8*1024*1024)
	if err != nil || len(firstBatch) != 64 || firstBatch[0].DroppedCount == nil || *firstBatch[0].DroppedCount != 1 {
		t.Fatalf("first bounded batch = %#v, %v", firstBatch, err)
	}
	firstGapID := firstBatch[0].ID
	if err := store.RecordAgentLogGap(2, start.Add(time.Minute), start.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := store.AcknowledgeAgentLogs([]string{firstGapID}); err != nil {
		t.Fatal(err)
	}
	nextBatch, err := store.PendingAgentLogs(1, 8*1024*1024)
	if err != nil || len(nextBatch) != 1 || nextBatch[0].ID == firstGapID ||
		nextBatch[0].DroppedCount == nil || *nextBatch[0].DroppedCount != 2 {
		t.Fatalf("new gap was removed by stale acknowledgement: %#v, %v", nextBatch, err)
	}
}

func TestAgentLogQueueLimitsCountAndBytes(t *testing.T) {
	if !overAgentLogLimit(MaxAgentLogEvents+1, 0) ||
		!overAgentLogLimit(0, MaxAgentLogQueueBytes+1) ||
		overAgentLogLimit(MaxAgentLogEvents, MaxAgentLogQueueBytes) {
		t.Fatal("Agent log queue limits are not independently enforced")
	}
}

func TestAgentLogValidationRejectsCredentialsInRequestTarget(t *testing.T) {
	event := testLogEvent(time.Now().UTC(), "failed")
	target := "https://user:password@example.com/check?key=secret"
	event.RequestTarget = &target
	store, err := Open(filepath.Join(t.TempDir(), "agent"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.EnqueueAgentLog(event); err != ErrInvalidAgentLog {
		t.Fatalf("credential-bearing request target error = %v", err)
	}
}

func TestAgentLogBucketsPreserveV011StateSchema(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "agent"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if SchemaVersion() != 9 {
		t.Fatalf("Agent state schema = %d, want v0.1.1-compatible schema 9", SchemaVersion())
	}
	if err := store.EnqueueAgentLog(testLogEvent(time.Now().UTC(), "new bucket")); err != nil {
		t.Fatal(err)
	}
	if err := store.database.View(func(tx *bolt.Tx) error {
		encoded := tx.Bucket(metaBucket).Get(schemaVersionKey)
		if len(encoded) != 8 || binary.BigEndian.Uint64(encoded) != 9 {
			t.Fatalf("stored Agent state schema = %x", encoded)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func seedLogQueue(t *testing.T, store *Store, count int, start time.Time) {
	t.Helper()
	err := store.database.Update(func(tx *bolt.Tx) error {
		queue := tx.Bucket(agentLogsBucket)
		index := tx.Bucket(agentLogIndexBucket)
		metadata := tx.Bucket(agentLogMetadataBucket)
		var logicalBytes uint64
		for item := 0; item < count; item++ {
			event := testLogEvent(start.Add(time.Duration(item)*time.Second), strings.Repeat("x", 8))
			encoded, err := json.Marshal(event)
			if err != nil {
				return err
			}
			sequence, err := queue.NextSequence()
			if err != nil {
				return err
			}
			key := make([]byte, 8)
			binary.BigEndian.PutUint64(key, sequence)
			if err := queue.Put(key, encoded); err != nil {
				return err
			}
			if err := index.Put([]byte(event.ID), key); err != nil {
				return err
			}
			logicalBytes += uint64(len(key) + len(encoded))
		}
		if err := putMetadataUint64(metadata, logCountKey, uint64(count)); err != nil {
			return err
		}
		return putMetadataUint64(metadata, logBytesKey, logicalBytes)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func testLogEvent(at time.Time, message string) LogEvent {
	return LogEvent{
		ID: uuid.NewString(), OccurredAt: at, Level: "info",
		Component: "test", EventType: "test-event", Message: message,
	}
}
