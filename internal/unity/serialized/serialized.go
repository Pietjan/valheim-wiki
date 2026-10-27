// Package serialized reads a Unity SerializedFile: the header, the types it
// holds with their type trees, the object table, and the external files it
// references. Objects decode through their type tree into plain Go values,
// the way UnityPy's read_typetree hands back dicts, so a caller walks a
// prefab by field name and never by class layout.
//
// Written for the files an AssetBundle holds - version 22 and up, type trees
// present - and reporting anything older rather than guessing at it.
package serialized

import (
	"fmt"
)

// External is a file this one refers to, by the index its PPtrs use.
type External struct {
	GUID [16]byte
	Type int32
	Path string
}

// Type is one class layout the file carries, with its tree.
type Type struct {
	ClassID     int32
	Stripped    bool
	ScriptIndex int16
	ScriptID    [16]byte
	OldHash     [16]byte
	Nodes       []Node
	// Dependencies are the type indices this type refers to, since 21.
	Dependencies []int32
	// For a reference type since 20.
	ClassName, Namespace, Assembly string
}

// Node is one field in a type tree, at its depth.
type Node struct {
	Version  uint16
	Level    uint8
	Flags    uint8
	Type     string
	Name     string
	Size     int32
	Index    int32
	MetaFlag int32
	RefHash  uint64
	// TypeOff and NameOff are the raw string offsets, kept for diagnosis.
	TypeOff, NameOff uint32
}

// Object is one entry of the object table.
type Object struct {
	PathID    int64
	Offset    int64
	Size      uint32
	TypeIndex int32
	ClassID   int32
	file      *File
}

// File is one serialised file.
type File struct {
	Version      uint32
	UnityVersion string
	Platform     int32
	BigEndian    bool
	Types        []Type
	RefTypes     []Type
	// HasTypeTrees is false for a file stripped of them.
	HasTypeTrees bool
	Objects      map[int64]*Object
	Order        []int64 // path ids in table order
	Externals    []External
	UserInfo     string
	data         []byte
	dataOffset   int64
}

// Parse reads a serialised file from its bytes.
func Parse(b []byte) (*File, error) {
	f := &File{Objects: map[int64]*Object{}, data: b}
	r := &reader{b: b, be: true}
	metadataSize := r.u32()
	fileSize := int64(r.u32())
	f.Version = r.u32()
	dataOffset := int64(r.u32())
	if f.Version < 9 {
		return nil, fmt.Errorf("serialized: version %d not supported", f.Version)
	}
	endian := r.u8()
	r.skip(3)
	if f.Version >= 22 {
		metadataSize = r.u32()
		fileSize = r.i64()
		dataOffset = r.i64()
		r.skip(8)
	}
	_ = metadataSize
	if r.err != nil {
		return nil, r.err
	}
	if fileSize > int64(len(b)) || dataOffset > int64(len(b)) {
		return nil, fmt.Errorf("serialized: header claims %d bytes, data at %d, have %d", fileSize, dataOffset, len(b))
	}
	f.BigEndian = endian != 0
	f.dataOffset = dataOffset
	r.be = f.BigEndian

	f.UnityVersion = r.cstring()
	f.Platform = r.i32()
	hasTree := true
	if f.Version >= 13 {
		hasTree = r.bool()
	}
	// A player build's own files (resources.assets) carry no type trees:
	// their objects decode only through a tree borrowed from a file that
	// does, see Object.TreeWith.
	f.HasTypeTrees = hasTree
	typeCount := int(r.i32())
	for i := 0; i < typeCount && r.err == nil; i++ {
		t, err := readType(r, f.Version, false, hasTree)
		if err != nil {
			return nil, fmt.Errorf("serialized: type %d: %w", i, err)
		}
		f.Types = append(f.Types, t)
	}
	objectCount := int(r.i32())
	for i := 0; i < objectCount && r.err == nil; i++ {
		r.align(4)
		o := &Object{file: f}
		o.PathID = r.i64()
		if f.Version >= 22 {
			o.Offset = r.i64()
		} else {
			o.Offset = int64(r.u32())
		}
		o.Size = r.u32()
		o.TypeIndex = r.i32()
		if int(o.TypeIndex) < len(f.Types) && o.TypeIndex >= 0 {
			o.ClassID = f.Types[o.TypeIndex].ClassID
		}
		f.Objects[o.PathID] = o
		f.Order = append(f.Order, o.PathID)
	}
	if f.Version >= 11 {
		n := int(r.i32())
		for i := 0; i < n && r.err == nil; i++ {
			r.i32()
			r.align(4)
			r.i64()
		}
	}
	n := int(r.i32())
	for i := 0; i < n && r.err == nil; i++ {
		var e External
		r.cstring()
		copy(e.GUID[:], r.bytes(16))
		e.Type = r.i32()
		e.Path = r.cstring()
		f.Externals = append(f.Externals, e)
	}
	if f.Version >= 20 {
		n := int(r.i32())
		for i := 0; i < n && r.err == nil; i++ {
			t, err := readType(r, f.Version, true, hasTree)
			if err != nil {
				return nil, fmt.Errorf("serialized: ref type %d: %w", i, err)
			}
			f.RefTypes = append(f.RefTypes, t)
		}
	}
	f.UserInfo = r.cstring()
	if r.err != nil {
		return nil, fmt.Errorf("serialized: metadata: %w", r.err)
	}
	return f, nil
}

func readType(r *reader, version uint32, ref, hasTree bool) (Type, error) {
	var t Type
	t.ClassID = r.i32()
	if version >= 16 {
		t.Stripped = r.bool()
	}
	if version >= 17 {
		t.ScriptIndex = r.i16()
	}
	if t.ClassID == 114 || (ref && t.ScriptIndex >= 0) {
		copy(t.ScriptID[:], r.bytes(16))
	}
	copy(t.OldHash[:], r.bytes(16))
	if version < 12 {
		return t, fmt.Errorf("type tree format before 12 not supported")
	}
	if !hasTree {
		// Nothing follows the hash without a tree, not even the
		// dependencies a tree's type lists after it.
		return t, r.err
	}
	nodeCount := int(r.i32())
	bufSize := int(r.i32())
	nodeSize := 24
	if version >= 19 {
		nodeSize = 32
	}
	nodes := r.bytes(nodeCount * nodeSize)
	buf := r.bytes(bufSize)
	if r.err != nil {
		return t, r.err
	}
	nr := &reader{b: nodes, be: r.be}
	for i := 0; i < nodeCount; i++ {
		var n Node
		n.Version = nr.u16()
		n.Level = nr.u8()
		n.Flags = nr.u8()
		typeOff := nr.u32()
		nameOff := nr.u32()
		n.Size = nr.i32()
		n.Index = nr.i32()
		n.MetaFlag = nr.i32()
		if version >= 19 {
			n.RefHash = nr.u64()
		}
		n.TypeOff, n.NameOff = typeOff, nameOff
		n.Type = treeString(buf, typeOff)
		n.Name = treeString(buf, nameOff)
		t.Nodes = append(t.Nodes, n)
	}
	if version >= 21 {
		if ref {
			t.ClassName = r.cstring()
			t.Namespace = r.cstring()
			t.Assembly = r.cstring()
		} else {
			n := int(r.i32())
			for i := 0; i < n; i++ {
				t.Dependencies = append(t.Dependencies, r.i32())
			}
		}
	}
	return t, r.err
}

// treeString resolves a type-tree string offset: the high bit says it is in
// Unity's common table rather than the file's own buffer.
func treeString(buf []byte, off uint32) string {
	if off&0x80000000 != 0 {
		return commonString(off & 0x7fffffff)
	}
	if int(off) >= len(buf) {
		return ""
	}
	end := int(off)
	for end < len(buf) && buf[end] != 0 {
		end++
	}
	return string(buf[off:end])
}

// Class names the extractor asks about, by class id.
var classNames = map[int32]string{
	1: "GameObject", 4: "Transform", 21: "Material", 23: "MeshRenderer", 33: "MeshFilter",
	43: "Mesh", 114: "MonoBehaviour", 115: "MonoScript", 137: "SkinnedMeshRenderer",
	205: "LODGroup", 224: "RectTransform", 65: "BoxCollider", 64: "MeshCollider",
	136: "CapsuleCollider", 135: "SphereCollider", 28: "Texture2D", 213: "Sprite",
	142: "AssetBundle", 48: "Shader", 90: "Avatar", 111: "Animation", 95: "Animator",
	54: "Rigidbody", 82: "AudioSource", 108: "Light", 198: "ParticleSystem", 199: "ParticleSystemRenderer",
}

// ClassName is the class an object is, by name where the id is known.
func (o *Object) ClassName() string {
	if n, ok := classNames[o.ClassID]; ok {
		return n
	}
	return fmt.Sprintf("Class%d", o.ClassID)
}

// Raw is the object's own bytes.
func (o *Object) Raw() []byte {
	start := o.file.dataOffset + o.Offset
	end := start + int64(o.Size)
	if start < 0 || end > int64(len(o.file.data)) {
		return nil
	}
	return o.file.data[start:end]
}

// TreeWith decodes the object through another file's tree for its class:
// the way into a file stripped of its own.
func (o *Object) TreeWith(t *Type) (map[string]any, error) {
	if len(t.Nodes) == 0 {
		return nil, fmt.Errorf("serialized: type %d has no tree", t.ClassID)
	}
	r := &reader{b: o.Raw(), be: o.file.BigEndian}
	v, _ := decode(r, t.Nodes, 0)
	if r.err != nil {
		return nil, fmt.Errorf("serialized: object %d (%s): %w", o.PathID, o.ClassName(), r.err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return map[string]any{"value": v}, nil
	}
	return m, nil
}

// Tree decodes the object through its type tree.
func (o *Object) Tree() (map[string]any, error) {
	if o.TypeIndex < 0 || int(o.TypeIndex) >= len(o.file.Types) {
		return nil, fmt.Errorf("serialized: object %d has no type", o.PathID)
	}
	t := &o.file.Types[o.TypeIndex]
	if len(t.Nodes) == 0 {
		return nil, fmt.Errorf("serialized: object %d: type %d has no tree", o.PathID, t.ClassID)
	}
	r := &reader{b: o.Raw(), be: o.file.BigEndian}
	v, _ := decode(r, t.Nodes, 0)
	if r.err != nil {
		return nil, fmt.Errorf("serialized: object %d (%s): %w", o.PathID, o.ClassName(), r.err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return map[string]any{"value": v}, nil
	}
	return m, nil
}
