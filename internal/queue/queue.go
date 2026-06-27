// package queue is the relay-side durable offline store: one bbolt bucket per
// user, holding pokes sent while they were away. it survives a relay restart,
// drains in send order on the target's next hello, and drops pokes older than a
// ttl so a poke sent to someone on holiday does not ambush them on return.
package queue

import (
	"encoding/binary"
	"encoding/json"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/undont/poke/internal/protocol"
)

// Queue is a persistent per-user poke store.
type Queue struct {
	db *bolt.DB
}

// Open creates or reopens the queue database at the given directory.
func Open(dir string) (*Queue, error) {
	path := filepath.Join(dir, "queue.db")
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	return &Queue{db: db}, nil
}

// Close flushes and closes the database.
func (q *Queue) Close() error { return q.db.Close() }

// Enqueue appends a poke to the target user's bucket, preserving send order via
// a monotonic sequence key. it stores the deliverable poked frame, which
// already names the sender.
func (q *Queue) Enqueue(user string, p protocol.Poked) error {
	return q.db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(user))
		if err != nil {
			return err
		}
		seq, err := b.NextSequence()
		if err != nil {
			return err
		}
		val, err := json.Marshal(p)
		if err != nil {
			return err
		}
		return b.Put(seqKey(seq), val)
	})
}

// Drain returns the target user's queued pokes in send order and clears the
// bucket. pokes older than ttl are dropped rather than returned. a ttl of zero
// keeps everything.
func (q *Queue) Drain(user string, ttl time.Duration, now time.Time) ([]protocol.Poked, error) {
	var out []protocol.Poked
	err := q.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(user))
		if b == nil {
			return nil
		}
		cutoff := int64(0)
		if ttl > 0 {
			cutoff = now.Add(-ttl).Unix()
		}
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var p protocol.Poked
			if err := json.Unmarshal(v, &p); err != nil {
				continue // skip a corrupt record rather than wedge the drain
			}
			if p.TS >= cutoff {
				out = append(out, p)
			}
		}
		return tx.DeleteBucket([]byte(user))
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Len reports how many pokes are queued for a user.
func (q *Queue) Len(user string) (int, error) {
	n := 0
	err := q.db.View(func(tx *bolt.Tx) error {
		if b := tx.Bucket([]byte(user)); b != nil {
			n = b.Stats().KeyN
		}
		return nil
	})
	return n, err
}

func seqKey(seq uint64) []byte {
	var k [8]byte
	binary.BigEndian.PutUint64(k[:], seq)
	return k[:]
}
