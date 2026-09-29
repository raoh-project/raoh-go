package raoh

import (
	"strconv"
	"strings"
)

// Path is where in the input a value was found. The zero value is the root.
type Path struct {
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
	return Path{&pathNode{parent: p.node, key: name}}
}

// Index returns the path of the element at i below p.
func (p Path) Index(i int) Path {
	return Path{&pathNode{parent: p.node, index: i, isIndex: true}}
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

// String returns p as a JSON Pointer (RFC 6901). The root is "".
func (p Path) String() string {
	var b strings.Builder
	for _, s := range p.Segments() {
		b.WriteByte('/')
		b.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(s))
	}
	return b.String()
}

// under returns p read as relative to prefix.
func (p Path) under(prefix Path) Path {
	if prefix.IsRoot() {
		return p
	}
	if p.IsRoot() {
		return prefix
	}
	nodes := []*pathNode{}
	for n := p.node; n != nil; n = n.parent {
		nodes = append(nodes, n)
	}
	out := prefix
	for i := len(nodes) - 1; i >= 0; i-- {
		n := nodes[i]
		if n.isIndex {
			out = out.Index(n.index)
		} else {
			out = out.Key(n.key)
		}
	}
	return out
}
