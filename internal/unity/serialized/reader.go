package serialized

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// reader is a cursor over a serialised file whose endianness is decided by
// its header: the header itself is big-endian, the rest almost always little.
type reader struct {
	b   []byte
	pos int
	be  bool
	err error
}

func (r *reader) need(n int) bool {
	if r.err != nil {
		return false
	}
	if n < 0 || r.pos+n > len(r.b) {
		r.err = fmt.Errorf("%w at %d", io.ErrUnexpectedEOF, r.pos)
		return false
	}
	return true
}

func (r *reader) bytes(n int) []byte {
	if !r.need(n) {
		return nil
	}
	out := r.b[r.pos : r.pos+n]
	r.pos += n
	return out
}

func (r *reader) skip(n int) { r.bytes(n) }

func (r *reader) u8() uint8 {
	if !r.need(1) {
		return 0
	}
	v := r.b[r.pos]
	r.pos++
	return v
}

func (r *reader) bool() bool { return r.u8() != 0 }

func (r *reader) u16() uint16 {
	if !r.need(2) {
		return 0
	}
	var v uint16
	if r.be {
		v = binary.BigEndian.Uint16(r.b[r.pos:])
	} else {
		v = binary.LittleEndian.Uint16(r.b[r.pos:])
	}
	r.pos += 2
	return v
}

func (r *reader) i16() int16 { return int16(r.u16()) }

func (r *reader) u32() uint32 {
	if !r.need(4) {
		return 0
	}
	var v uint32
	if r.be {
		v = binary.BigEndian.Uint32(r.b[r.pos:])
	} else {
		v = binary.LittleEndian.Uint32(r.b[r.pos:])
	}
	r.pos += 4
	return v
}

func (r *reader) i32() int32 { return int32(r.u32()) }

func (r *reader) u64() uint64 {
	if !r.need(8) {
		return 0
	}
	var v uint64
	if r.be {
		v = binary.BigEndian.Uint64(r.b[r.pos:])
	} else {
		v = binary.LittleEndian.Uint64(r.b[r.pos:])
	}
	r.pos += 8
	return v
}

func (r *reader) i64() int64 { return int64(r.u64()) }

func (r *reader) f32() float32 { return math.Float32frombits(r.u32()) }
func (r *reader) f64() float64 { return math.Float64frombits(r.u64()) }

func (r *reader) cstring() string {
	if r.err != nil {
		return ""
	}
	i := bytes.IndexByte(r.b[r.pos:], 0)
	if i < 0 {
		r.err = fmt.Errorf("unterminated string at %d", r.pos)
		return ""
	}
	s := string(r.b[r.pos : r.pos+i])
	r.pos += i + 1
	return s
}

// alignedString is an int32 length, the bytes, then alignment to four.
func (r *reader) alignedString() string {
	n := int(r.i32())
	s := string(r.bytes(n))
	r.align(4)
	return s
}

func (r *reader) align(n int) {
	if rem := r.pos % n; rem != 0 {
		r.skip(n - rem)
	}
}
