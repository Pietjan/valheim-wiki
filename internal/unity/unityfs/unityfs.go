// Package unityfs reads the UnityFS archive that an AssetBundle is: a header,
// a table of compressed blocks, and a directory of the files those blocks
// hold once joined and decompressed - a serialised file per "CAB-..." entry
// and, beside it, the ".resS" resource file its meshes and textures point
// into.
//
// Only what an extractor needs: every node comes back as bytes. Compression
// 2 and 3 are LZ4; 1 is LZMA, which none of Valheim's bundles use and which
// is reported rather than decoded.
package unityfs

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/pierrec/lz4/v4"
)

// Node is one file inside the archive.
type Node struct {
	Path  string
	Flags uint32
	Data  []byte
}

// Archive is one bundle, read whole.
type Archive struct {
	Signature     string
	Version       uint32
	UnityVersion  string
	UnityRevision string
	Nodes         []Node
}

// Node returns the entry with this path, or nil.
func (a *Archive) Node(path string) *Node {
	for i := range a.Nodes {
		if a.Nodes[i].Path == path {
			return &a.Nodes[i]
		}
	}
	return nil
}

// Open reads and decompresses a bundle file.
func Open(path string) (*Archive, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	a, err := Parse(b)
	if err != nil {
		return nil, fmt.Errorf("unityfs: %s: %w", path, err)
	}
	return a, nil
}

const (
	compressionMask   = 0x3f
	blocksInfoAtEnd   = 0x80
	blocksInfoPadding = 0x200
)

// List names the files inside a bundle without reading it: the header and
// the directory are a few kilobytes at the front, or at the back when the
// flags say so, and nothing else is touched. For finding which of eight
// hundred bundles holds a given CAB, which Open would answer at ninety
// megabytes a bundle.
func List(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// The fixed header is under a hundred bytes; the directory that follows
	// it is read separately once its size is known.
	head := make([]byte, 4096)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF {
		return nil, fmt.Errorf("unityfs: %s: %w", path, err)
	}
	head = head[:n]
	r := &reader{b: head}
	if sig := r.cstring(); sig != "UnityFS" {
		return nil, fmt.Errorf("unityfs: %s: signature %q, want UnityFS", path, sig)
	}
	version := r.u32()
	r.cstring()
	r.cstring()
	size := r.i64()
	compressedInfo := r.u32()
	uncompressedInfo := r.u32()
	flags := r.u32()
	if r.err != nil {
		return nil, fmt.Errorf("unityfs: %s: %w", path, r.err)
	}
	if version >= 7 {
		r.align(16)
	}

	info := make([]byte, compressedInfo)
	at := int64(r.pos)
	if flags&blocksInfoAtEnd != 0 {
		at = size - int64(compressedInfo)
	}
	if _, err := f.ReadAt(info, at); err != nil {
		return nil, fmt.Errorf("unityfs: %s: blocks info: %w", path, err)
	}
	info, err = decompress(flags&compressionMask, info, int(uncompressedInfo))
	if err != nil {
		return nil, fmt.Errorf("unityfs: %s: blocks info: %w", path, err)
	}

	ir := &reader{b: info}
	ir.skip(16)
	blockCount := ir.i32()
	ir.skip(10 * max(int(blockCount), 0))
	nodeCount := ir.i32()
	var names []string
	for i := 0; i < int(nodeCount); i++ {
		ir.i64()
		ir.i64()
		ir.u32()
		names = append(names, ir.cstring())
	}
	if ir.err != nil {
		return nil, fmt.Errorf("unityfs: %s: blocks info: %w", path, ir.err)
	}
	return names, nil
}

// Parse reads a bundle from its bytes.
func Parse(b []byte) (*Archive, error) {
	r := &reader{b: b}
	a := &Archive{}
	a.Signature = r.cstring()
	if a.Signature != "UnityFS" {
		return nil, fmt.Errorf("signature %q, want UnityFS", a.Signature)
	}
	a.Version = r.u32()
	a.UnityVersion = r.cstring()
	a.UnityRevision = r.cstring()
	if a.Version < 6 {
		return nil, fmt.Errorf("archive version %d not supported", a.Version)
	}
	size := r.i64()
	compressedInfo := r.u32()
	uncompressedInfo := r.u32()
	flags := r.u32()
	if r.err != nil {
		return nil, r.err
	}
	if a.Version >= 7 {
		r.align(16)
	}

	var info []byte
	switch {
	case flags&blocksInfoAtEnd != 0:
		if int64(len(b)) < size || size < int64(compressedInfo) {
			return nil, fmt.Errorf("blocks info at end: archive %d bytes, header says %d", len(b), size)
		}
		info = b[size-int64(compressedInfo) : size]
	default:
		info = r.bytes(int(compressedInfo))
	}
	if r.err != nil {
		return nil, r.err
	}
	info, err := decompress(flags&compressionMask, info, int(uncompressedInfo))
	if err != nil {
		return nil, fmt.Errorf("blocks info: %w", err)
	}
	if a.Version >= 7 && flags&blocksInfoPadding != 0 {
		r.align(16)
	}

	ir := &reader{b: info}
	ir.skip(16) // hash of the uncompressed data
	blockCount := ir.i32()
	type block struct {
		uncompressed, compressed uint32
		flags                    uint16
	}
	blocks := make([]block, 0, max(blockCount, 0))
	for i := 0; i < int(blockCount); i++ {
		blocks = append(blocks, block{ir.u32(), ir.u32(), ir.u16()})
	}
	nodeCount := ir.i32()
	type entry struct {
		offset, size int64
		flags        uint32
		path         string
	}
	entries := make([]entry, 0, max(nodeCount, 0))
	for i := 0; i < int(nodeCount); i++ {
		entries = append(entries, entry{ir.i64(), ir.i64(), ir.u32(), ir.cstring()})
	}
	if ir.err != nil {
		return nil, fmt.Errorf("blocks info: %w", ir.err)
	}

	// The blocks join into one stream the directory offsets index.
	var total int
	for _, bl := range blocks {
		total += int(bl.uncompressed)
	}
	data := make([]byte, 0, total)
	for i, bl := range blocks {
		raw := r.bytes(int(bl.compressed))
		if r.err != nil {
			return nil, fmt.Errorf("block %d: %w", i, r.err)
		}
		out, err := decompress(uint32(bl.flags)&compressionMask, raw, int(bl.uncompressed))
		if err != nil {
			return nil, fmt.Errorf("block %d: %w", i, err)
		}
		data = append(data, out...)
	}

	for _, e := range entries {
		if e.offset < 0 || e.size < 0 || e.offset+e.size > int64(len(data)) {
			return nil, fmt.Errorf("node %s: %d+%d outside %d bytes of data", e.path, e.offset, e.size, len(data))
		}
		a.Nodes = append(a.Nodes, Node{Path: e.path, Flags: e.flags, Data: data[e.offset : e.offset+e.size]})
	}
	return a, nil
}

func decompress(kind uint32, in []byte, size int) ([]byte, error) {
	switch kind {
	case 0:
		return in, nil
	case 2, 3:
		out := make([]byte, size)
		n, err := lz4.UncompressBlock(in, out)
		if err != nil {
			return nil, err
		}
		if n != size {
			return nil, fmt.Errorf("lz4: %d bytes out, want %d", n, size)
		}
		return out, nil
	case 1:
		return nil, fmt.Errorf("lzma blocks are not supported")
	default:
		return nil, fmt.Errorf("compression %d unknown", kind)
	}
}

// reader is a big-endian cursor, which is what the archive header is.
type reader struct {
	b   []byte
	pos int
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

func (r *reader) u16() uint16 {
	if !r.need(2) {
		return 0
	}
	v := binary.BigEndian.Uint16(r.b[r.pos:])
	r.pos += 2
	return v
}

func (r *reader) u32() uint32 {
	if !r.need(4) {
		return 0
	}
	v := binary.BigEndian.Uint32(r.b[r.pos:])
	r.pos += 4
	return v
}

func (r *reader) i32() int32 { return int32(r.u32()) }

func (r *reader) i64() int64 {
	if !r.need(8) {
		return 0
	}
	v := binary.BigEndian.Uint64(r.b[r.pos:])
	r.pos += 8
	return int64(v)
}

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

func (r *reader) align(n int) {
	if rem := r.pos % n; rem != 0 {
		r.skip(n - rem)
	}
}
