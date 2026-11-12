package run

import (
	"crypto/sha256"
	"encoding/binary"
)

// SnapshotHash pins an approval to the snapshot it was raised in: SHA-256 over the agent version, the SDK version and the blob.
func SnapshotHash(s *Snapshot) []byte {
	// SECURITY: hash the bytes being saved or loaded, never a re-read; fixed-width and length-prefixed fields keep two snapshots apart.
	h := sha256.New()
	_, _ = h.Write(s.AgentVersionID[:])
	var n [8]byte
	binary.BigEndian.PutUint64(n[:], uint64(len(s.SDKVersion)))
	_, _ = h.Write(n[:])
	_, _ = h.Write([]byte(s.SDKVersion))
	binary.BigEndian.PutUint64(n[:], uint64(len(s.Blob)))
	_, _ = h.Write(n[:])
	_, _ = h.Write(s.Blob)
	return h.Sum(nil)
}
