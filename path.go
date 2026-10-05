package raoh

import (
	"strconv"
	"strings"
)

// Path is where in the input a value was found. The zero value is the root.
//
// A path is a sequence of segments, each written as text: a member name, or
// an index in decimal. [Path.Index] takes an int for convenience; the segment
// it makes is the same as [Path.Key] with the decimal digits.
//
// A Path is not comparable with ==, which would compare where the paths are
// stored and not what they say. Use [Path.Equal].
type Path struct {
	_    [0]func()
	node *pathNode
}

type pathNode struct {
	parent  *pathNode
	key     string
	index   int
	isIndex bool
}

// Key returns the path of the member name below p.
func (p Path) Key(name string) Path {
	return Path{node: &pathNode{parent: p.node, key: name}}
}

// Index returns the path of the element at i below p.
func (p Path) Index(i int) Path {
	return Path{node: &pathNode{parent: p.node, index: i, isIndex: true}}
}

// PathOf returns the path of the members named by segments, from the root
// down. With none it is the root. Each segment is taken as text, so
// PathOf("items", "0") equals the path of [Path.Index](0) below "items".
func PathOf(segments ...string) Path {
	var p Path
	for _, s := range segments {
		p = p.Key(s)
	}
	return p
}

// Equal reports whether p and other are the same sequence of segments, as
// [Path.Segments] and [Path.String] read them: the member "0" and the element
// at index 0 are the same segment.
func (p Path) Equal(other Path) bool {
	a, b := p.node, other.node
	for a != nil && b != nil {
		if a == b {
			return true
		}
		if !sameSegment(a, b) {
			return false
		}
		a, b = a.parent, b.parent
	}
	return a == b
}

func sameSegment(a, b *pathNode) bool {
	switch {
	case a.isIndex && b.isIndex:
		return a.index == b.index
	case !a.isIndex && !b.isIndex:
		return a.key == b.key
	case a.isIndex:
		return strconv.Itoa(a.index) == b.key
	default:
		return a.key == strconv.Itoa(b.index)
	}
}

// IsRoot reports whether p is the root of the input.
func (p Path) IsRoot() bool { return p.node == nil }

// Segments returns the keys and indexes of p from the root down, indexes
// written in decimal.
func (p Path) Segments() []string {
	var segs []string
	for n := p.node; n != nil; n = n.parent {
		if n.isIndex {
			segs = append(segs, strconv.Itoa(n.index))
		} else {
			segs = append(segs, n.key)
		}
	}
	for i, j := 0, len(segs)-1; i < j; i, j = i+1, j-1 {
		segs[i], segs[j] = segs[j], segs[i]
	}
	return segs
}

// pointerEscape writes a segment of a JSON Pointer: ~ as ~0 and / as ~1.
var pointerEscape = strings.NewReplacer("~", "~0", "/", "~1")

// String returns p as a JSON Pointer (RFC 6901). The root is "".
func (p Path) String() string {
	var b strings.Builder
	for _, s := range p.Segments() {
		b.WriteByte('/')
		if strings.ContainsAny(s, "~/") {
			pointerEscape.WriteString(&b, s)
		} else {
			b.WriteString(s)
		}
	}
	return b.String()
}

// Append returns other below p: the path that reads other as relative to p.
func (p Path) Append(other Path) Path {
	if p.IsRoot() {
		return other
	}
	if other.IsRoot() {
		return p
	}
	var nodes []*pathNode
	for n := other.node; n != nil; n = n.parent {
		nodes = append(nodes, n)
	}
	out := p
	for i := len(nodes) - 1; i >= 0; i-- {
		if n := nodes[i]; n.isIndex {
			out = out.Index(n.index)
		} else {
			out = out.Key(n.key)
		}
	}
	return out
}
