// Package norm implements Unicode normalization, the forms NFC, NFD, NFKC and NFKD of UAX #15,
// as Java's java.text.Normalizer gives them.
//
// The standard library has no normalization, and this module depends on it alone, so the data the
// forms are driven by is generated from Java into tables.go (see internal/gennorm), and the
// algorithm is here: decompose, put combining marks in canonical order, and compose.
//
// Normalizing costs time linear in the length of the input, whatever it holds. The input is read
// once and worked on a segment at a time, a starter and the combining marks that follow it. A run of
// marks is put in order by an insertion sort when it is short and by a counting sort when it is
// long, and a mark is composed with its starter by one look at the table.
//
// Text that is not valid UTF-8 has no normalization, and is not changed by it: each byte that is not
// part of a UTF-8 sequence is kept as it is, and it composes with nothing and keeps the characters
// on its two sides apart, as a starter would.
package norm

import "unicode/utf8"

// Form is a normalization form.
type Form uint8

const (
	// NFC is canonical decomposition followed by canonical composition.
	NFC Form = iota
	// NFD is canonical decomposition.
	NFD
	// NFKC is compatibility decomposition followed by canonical composition.
	NFKC
	// NFKD is compatibility decomposition.
	NFKD
)

const (
	sBase  = 0xAC00
	lBase  = 0x1100
	vBase  = 0x1161
	tBase  = 0x11A7
	lCount = 19
	vCount = 21
	tCount = 28
	sCount = lCount * vCount * tCount

	// insertionLimit is the longest run of combining marks put in order by insertion sort.
	insertionLimit = 32
)

type cccRange struct {
	lo, hi rune
	ccc    uint8
}

type decomposition struct {
	r rune
	s string
}

type composite struct{ a, b, c rune }

// String returns s in the form f.
func String(f Form, s string) string {
	if f > NFKD {
		panic("norm: invalid form")
	}
	if isASCII(s) {
		return s
	}
	n := normalizer{form: f, in: s}
	return n.run()
}

// isASCII reports whether s is ASCII, which all four forms leave as it is.
func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

type item struct {
	r   rune
	ccc uint8
}

type normalizer struct {
	form     Form
	in       string
	pos      int    // the length of the prefix of in the output is equal to, until it departs
	out      []byte // the output, once it has departed from in
	diverged bool

	// seg is the segment being built: when hasStarter, seg[0] is its starter, and the rest are
	// combining marks; otherwise it is marks that come first in the text or after an invalid byte.
	seg        []item
	hasStarter bool
	aux        []item

	// work counts the steps taken, so that a test can hold them to a bound linear in the input.
	work int
}

func (n *normalizer) composing() bool { return n.form == NFC || n.form == NFKC }
func (n *normalizer) compat() bool    { return n.form == NFKC || n.form == NFKD }

func (n *normalizer) run() string {
	s := n.in
	for i := 0; i < len(s); {
		n.work++
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			n.flush()
			n.emitByte(s[i])
			i++
			continue
		}
		i += size
		n.add(r)
	}
	n.flush()
	if !n.diverged {
		if n.pos == len(n.in) {
			return n.in
		}
		n.diverge()
	}
	return string(n.out)
}

// add decomposes r and pushes what it decomposes to.
func (n *normalizer) add(r rune) {
	if r < minDecomp {
		n.push(r, cccOf(r))
		return
	}
	if r >= sBase && r < sBase+sCount {
		n.work++
		s := r - sBase
		n.push(lBase+s/(vCount*tCount), 0)
		n.push(vBase+s%(vCount*tCount)/tCount, 0)
		if t := s % tCount; t != 0 {
			n.push(tBase+t, 0)
		}
		return
	}
	d, ok := "", false
	if n.compat() {
		d, ok = lookup(compatDecomps, r)
	}
	if !ok {
		d, ok = lookup(canonicalDecomps, r)
	}
	if !ok {
		n.push(r, cccOf(r))
		return
	}
	for _, dr := range d {
		n.work++
		n.push(dr, cccOf(dr))
	}
}

func (n *normalizer) push(r rune, ccc uint8) {
	if ccc != 0 {
		n.seg = append(n.seg, item{r, ccc})
		return
	}
	n.finishMarks()
	if n.composing() && n.hasStarter && len(n.seg) == 1 {
		// Nothing but the starter is between it and r, so r can compose with it.
		n.work++
		if c, ok := compose(n.seg[0].r, r); ok {
			n.seg[0] = item{c, cccOf(c)}
			return
		}
	}
	n.emitSegment()
	n.seg = append(n.seg[:0], item{r, 0})
	n.hasStarter = true
}

// finishMarks puts the marks of the segment in canonical order and, when composing, composes them
// with its starter. A mark that does not compose stays; it keeps in the way of any later mark of
// the same or a lower class, and of a following starter.
func (n *normalizer) finishMarks() {
	marks := n.seg
	if n.hasStarter {
		marks = marks[min(1, len(marks)):]
	}
	if len(marks) == 0 {
		return
	}
	n.sortMarks(marks)
	if !n.composing() || !n.hasStarter {
		return
	}
	starter, kept, last := n.seg[0].r, 0, uint8(0)
	for _, m := range marks {
		n.work++
		if last < m.ccc {
			if c, ok := compose(starter, m.r); ok {
				starter = c
				continue
			}
		}
		marks[kept] = m
		kept++
		last = m.ccc
	}
	n.seg[0].r = starter
	n.seg = n.seg[:1+kept]
}

// sortMarks orders marks by combining class, keeping the order of marks of one class.
func (n *normalizer) sortMarks(marks []item) {
	sorted := true
	for i := 1; i < len(marks); i++ {
		n.work++
		if marks[i-1].ccc > marks[i].ccc {
			sorted = false
			break
		}
	}
	if sorted {
		return
	}
	if len(marks) <= insertionLimit {
		for i := 1; i < len(marks); i++ {
			m := marks[i]
			j := i
			for ; j > 0 && marks[j-1].ccc > m.ccc; j-- {
				n.work++
				marks[j] = marks[j-1]
			}
			marks[j] = m
		}
		return
	}
	var count [257]int
	for _, m := range marks {
		count[int(m.ccc)+1]++
	}
	for c := 1; c < len(count); c++ {
		count[c] += count[c-1]
	}
	if cap(n.aux) < len(marks) {
		n.aux = make([]item, len(marks))
	}
	aux := n.aux[:len(marks)]
	for _, m := range marks {
		n.work++
		aux[count[m.ccc]] = m
		count[m.ccc]++
	}
	copy(marks, aux)
}

// flush finishes the segment and writes it out.
func (n *normalizer) flush() {
	n.finishMarks()
	n.emitSegment()
	n.seg = n.seg[:0]
	n.hasStarter = false
}

func (n *normalizer) emitSegment() {
	for _, it := range n.seg {
		n.emit(it.r)
	}
}

// emit writes r to the output. While the output is equal to the input it is only counted, so that
// text a form leaves as it is comes back as the same string without a copy.
func (n *normalizer) emit(r rune) {
	if !n.diverged {
		if n.pos < len(n.in) {
			if cr, size := utf8.DecodeRuneInString(n.in[n.pos:]); cr == r && !(cr == utf8.RuneError && size == 1) {
				n.pos += size
				return
			}
		}
		n.diverge()
	}
	n.out = utf8.AppendRune(n.out, r)
}

// emitByte writes a byte that is not part of a UTF-8 sequence.
func (n *normalizer) emitByte(b byte) {
	if !n.diverged {
		if n.pos < len(n.in) && n.in[n.pos] == b {
			n.pos++
			return
		}
		n.diverge()
	}
	n.out = append(n.out, b)
}

func (n *normalizer) diverge() {
	n.diverged = true
	n.out = make([]byte, 0, len(n.in)+len(n.in)/8+8)
	n.out = append(n.out, n.in[:n.pos]...)
}

// cccOf is the canonical combining class of r.
func cccOf(r rune) uint8 {
	if r < minCCC {
		return 0
	}
	lo, hi := 0, len(cccRanges)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		switch rg := cccRanges[mid]; {
		case r < rg.lo:
			hi = mid
		case r > rg.hi:
			lo = mid + 1
		default:
			return rg.ccc
		}
	}
	return 0
}

func lookup(table []decomposition, r rune) (string, bool) {
	lo, hi := 0, len(table)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		switch d := table[mid]; {
		case r < d.r:
			hi = mid
		case r > d.r:
			lo = mid + 1
		default:
			return d.s, true
		}
	}
	return "", false
}

// compose is the primary composite of a and b, if there is one.
func compose(a, b rune) (rune, bool) {
	switch {
	case a >= lBase && a < lBase+lCount && b >= vBase && b < vBase+vCount:
		return sBase + ((a-lBase)*vCount+(b-vBase))*tCount, true
	case a >= sBase && a < sBase+sCount && (a-sBase)%tCount == 0 && b > tBase && b < tBase+tCount:
		return a + (b - tBase), true
	case b < minSecond:
		return 0, false
	}
	lo, hi := 0, len(composites)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		c := composites[mid]
		switch {
		case a < c.a || a == c.a && b < c.b:
			hi = mid
		case a > c.a || b > c.b:
			lo = mid + 1
		default:
			return c.c, true
		}
	}
	return 0, false
}
