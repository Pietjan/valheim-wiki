package serialized

// decode reads the value node i describes and returns it with the index of
// the next sibling. Maps for structs, slices for arrays, strings, numbers and
// bools for the leaves; alignment follows the node's meta flag.
func decode(r *reader, nodes []Node, i int) (any, int) {
	n := nodes[i]
	align := n.MetaFlag&0x4000 != 0
	next := i + 1
	// The direct children are the nodes at level+1 until the level drops back.
	children := func() []int {
		var out []int
		for j := i + 1; j < len(nodes) && nodes[j].Level > n.Level; j++ {
			if nodes[j].Level == n.Level+1 {
				out = append(out, j)
			}
		}
		return out
	}
	end := func() int {
		j := i + 1
		for j < len(nodes) && nodes[j].Level > n.Level {
			j++
		}
		return j
	}

	var v any
	switch n.Type {
	case "string":
		// A string is an Array of char with its own size; the alignment sits
		// on the string node.
		// Unity aligns after every string whatever the flags say.
		v = r.alignedString()
		next = end()
	case "Array":
		// size, then data repeated.
		kids := children()
		if len(kids) != 2 {
			v = []any{}
			next = end()
			break
		}
		count := int(r.i32())
		data := kids[1]
		dataEnd := end()
		if nodes[data].Type == "UInt8" || nodes[data].Type == "char" {
			// A byte array reads as one slice, which is what TypelessData is.
			v = append([]byte(nil), r.bytes(count)...)
		} else {
			items := make([]any, 0, max(count, 0))
			for k := 0; k < count && r.err == nil; k++ {
				item, _ := decode(r, nodes, data)
				items = append(items, item)
			}
			v = items
		}
		next = dataEnd
	case "TypelessData":
		count := int(r.i32())
		v = append([]byte(nil), r.bytes(count)...)
		next = end()
	case "bool":
		v = r.bool()
	case "SInt8":
		v = int8(r.u8())
	case "UInt8", "char":
		v = r.u8()
	case "SInt16", "short":
		v = r.i16()
	case "UInt16", "unsigned short":
		v = r.u16()
	case "SInt32", "int", "Type*":
		v = r.i32()
	case "UInt32", "unsigned int":
		v = r.u32()
	case "SInt64", "long long":
		v = r.i64()
	case "UInt64", "unsigned long long", "FileSize":
		v = r.u64()
	case "float":
		v = r.f32()
	case "double":
		v = r.f64()
	default:
		kids := children()
		if len(kids) == 0 {
			// A leaf of a type this does not know: skip its bytes.
			r.skip(int(n.Size))
			v = nil
			break
		}
		// vector / map / staticvector wrap a single Array child.
		if len(kids) == 1 && nodes[kids[0]].Type == "Array" {
			v, _ = decode(r, nodes, kids[0])
			next = end()
			break
		}
		m := make(map[string]any, len(kids))
		for _, k := range kids {
			var child any
			child, _ = decode(r, nodes, k)
			m[nodes[k].Name] = child
		}
		v = m
		next = end()
	}
	if align {
		r.align(4)
	}
	return v, next
}
