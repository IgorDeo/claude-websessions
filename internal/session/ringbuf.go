package session

import "sync"

// RingBuf is a thread-safe circular byte buffer.
type RingBuf struct {
	mu      sync.Mutex
	buf     []byte
	size    int
	w       int   // next write position
	written int64 // total bytes written (used to determine if buffer is full)
}

func NewRingBuf(size int) *RingBuf {
	return &RingBuf{
		buf:  make([]byte, size),
		size: size,
	}
}

func (r *RingBuf) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	n := len(p)

	// If data is larger than buffer, only keep the tail
	if n >= r.size {
		copy(r.buf, p[n-r.size:])
		r.w = 0
		r.written += int64(n)
		return n, nil
	}

	// Write data, wrapping around if needed
	firstChunk := r.size - r.w
	if firstChunk >= n {
		copy(r.buf[r.w:], p)
	} else {
		copy(r.buf[r.w:], p[:firstChunk])
		copy(r.buf, p[firstChunk:])
	}

	r.w = (r.w + n) % r.size
	r.written += int64(n)

	return n, nil
}

func (r *RingBuf) isFull() bool {
	return r.written >= int64(r.size)
}

// Bytes returns the current buffer contents in order.
func (r *RingBuf) Bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isFull() {
		out := make([]byte, r.w)
		copy(out, r.buf[:r.w])
		return out
	}

	out := make([]byte, r.size)
	// Read from write position (oldest) to end, then start to write position
	n := copy(out, r.buf[r.w:])
	copy(out[n:], r.buf[:r.w])
	return out
}
