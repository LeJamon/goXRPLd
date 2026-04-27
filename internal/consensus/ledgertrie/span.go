package ledgertrie

import "github.com/LeJamon/goXRPLd/internal/consensus"

// span is the half-open interval [start, end) of a ledger's ancestry,
// always non-empty. Port of ledger_trie_detail::Span<Ledger>
// (LedgerTrie.h:77-198).
type span struct {
	start  uint32
	end    uint32
	ledger Ledger
}

// newSpanFromLedger returns [0, l.Seq()+1).
func newSpanFromLedger(l Ledger) span {
	return span{start: 0, end: l.Seq() + 1, ledger: l}
}

func (s span) clamp(val uint32) uint32 {
	if val < s.start {
		return s.start
	}
	if val > s.end {
		return s.end
	}
	return val
}

func (s span) sub(from, to uint32) (span, bool) {
	nf := s.clamp(from)
	nt := s.clamp(to)
	if nf < nt {
		return span{start: nf, end: nt, ledger: s.ledger}, true
	}
	return span{}, false
}

func (s span) from(spot uint32) (span, bool)   { return s.sub(spot, s.end) }
func (s span) before(spot uint32) (span, bool) { return s.sub(s.start, spot) }

func (s span) startID() consensus.LedgerID { return s.ledger.Ancestor(s.start) }

// diff is Span::diff (LedgerTrie.h:144-148).
func (s span) diff(o Ledger) uint32 { return s.clamp(Mismatch(s.ledger, o)) }

// tip is Span::tip (LedgerTrie.h:151-156).
func (s span) tip() SpanTip {
	tipSeq := s.end - 1
	return SpanTip{Seq: tipSeq, ID: s.ledger.Ancestor(tipSeq), ledger: s.ledger}
}

// mergeSpans combines two adjacent spans, taking the ledger from the
// higher-end span so the tip resolves correctly. Port of free `merge`
// (LedgerTrie.h:189-197).
func mergeSpans(a, b span) span {
	lo := a.start
	if b.start < lo {
		lo = b.start
	}
	if a.end < b.end {
		return span{start: lo, end: b.end, ledger: b.ledger}
	}
	return span{start: lo, end: a.end, ledger: a.ledger}
}

// node is a trie node. Port of ledger_trie_detail::Node<Ledger>
// (LedgerTrie.h:201-269).
type node struct {
	s             span
	tipSupport    uint32
	branchSupport uint32
	children      []*node
	parent        *node
}

func newEmptyNode(genesis Ledger) *node {
	return &node{s: span{start: 0, end: 1, ledger: genesis}}
}

func newNodeFromSpan(s span) *node { return &node{s: s} }

// eraseChild swap-pops child from n.children. Mirrors Node::erase
// (LedgerTrie.h:227-239).
func (n *node) eraseChild(child *node) {
	for i, c := range n.children {
		if c == child {
			last := len(n.children) - 1
			n.children[i] = n.children[last]
			n.children[last] = nil
			n.children = n.children[:last]
			return
		}
	}
}
