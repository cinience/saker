package agui

const defaultEventRingSize = 256

type eventEntry struct {
	seq  int
	data []byte
}

type eventRing struct {
	buf  []eventEntry
	head int
	full bool
	size int
}

func newEventRing(size int) *eventRing {
	if size <= 0 {
		size = defaultEventRingSize
	}
	return &eventRing{
		buf:  make([]eventEntry, size),
		size: size,
	}
}

func (r *eventRing) Push(seq int, data []byte) {
	r.buf[r.head] = eventEntry{seq: seq, data: data}
	r.head++
	if r.head >= r.size {
		r.head = 0
		r.full = true
	}
}

func (r *eventRing) Since(seq int) []eventEntry {
	var start, count int
	if r.full {
		start = r.head
		count = r.size
	} else {
		start = 0
		count = r.head
	}
	if count == 0 {
		return nil
	}

	var result []eventEntry
	for i := 0; i < count; i++ {
		idx := (start + i) % r.size
		e := r.buf[idx]
		if e.seq > seq {
			result = append(result, e)
		}
	}
	return result
}

func (r *eventRing) Len() int {
	if r.full {
		return r.size
	}
	return r.head
}

// OldestSeq returns the sequence number of the oldest entry in the ring,
// or 0 if the ring is empty.
func (r *eventRing) OldestSeq() int {
	if r.head == 0 && !r.full {
		return 0
	}
	if r.full {
		return r.buf[r.head].seq
	}
	return r.buf[0].seq
}

// NewestSeq returns the sequence number of the newest entry in the ring,
// or 0 if the ring is empty.
func (r *eventRing) NewestSeq() int {
	if r.head == 0 && !r.full {
		return 0
	}
	idx := r.head - 1
	if idx < 0 {
		idx = r.size - 1
	}
	return r.buf[idx].seq
}
