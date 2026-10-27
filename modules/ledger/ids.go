package ledger

import (
	"crypto/rand"
	"fmt"
)

// AccountID and TransactionID are the ledger's own ids, random UUIDs. Other
// modules refer to accounts and transactions only by these.
type (
	AccountID     string
	TransactionID string
)

// newID returns a random (version 4) UUID.
func newID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // never fails: crypto/rand panics instead of returning an error
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
