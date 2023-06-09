package io

import (
	"crypto/sha256"
	"hash"
	"io"
)

// Reader describes a reader with a record of the number of bytes
// and a sha256 hash of what's read.
type Reader interface {
	// Read reads the given bytes and returns the number of bytes read.
	Read([]byte) (int, error)

	// SHA256Hash returns the SHA256 hash of what's read.
	SHA256Hash() hash.Hash

	// N is a record of the total number of bytes read so far.
	N() int64
}

// NewReader creates a new Reader
func NewReader(r io.Reader) Reader {
	hash := sha256.New()
	base := io.TeeReader(r, hash)
	return &ReaderWithContext{base: base, sha256Hash: hash}
}

// ReaderWithContext provides an implementation of Reader.
type ReaderWithContext struct {
	base       io.Reader
	sha256Hash hash.Hash
	n          int64
}

// Read reads the given bytes
func (r *ReaderWithContext) Read(p []byte) (int, error) {
	n, err := r.base.Read(p)
	r.n += int64(n)
	return n, err
}

// N returns the total number of bytes read.
func (r *ReaderWithContext) N() int64 {
	return r.n
}

// SHA256Hash returns the SHA256 hash of the bytes read.
func (r *ReaderWithContext) SHA256Hash() hash.Hash {
	return r.sha256Hash
}
