// Package iprange provides inclusive address ranges for MMDB queries.
package iprange

import (
	"fmt"
	"math/big"
	"net/netip"
)

type Range struct{ Start, End netip.Addr }

func New(start, end netip.Addr) (Range, error) {
	start, end = start.Unmap(), end.Unmap()
	if !start.IsValid() || !end.IsValid() || start.Zone() != "" || end.Zone() != "" ||
		start.BitLen() != end.BitLen() ||
		start.Compare(end) > 0 {
		return Range{}, fmt.Errorf(
			"range endpoints must be ordered addresses of the same family without zones",
		)
	}
	return Range{start, end}, nil
}

func FromPrefix(prefix netip.Prefix) Range {
	start := prefix.Masked().Addr()
	end := start.As16()
	for bit := 128 - start.BitLen() + prefix.Bits(); bit < 128; bit++ {
		end[bit/8] |= 1 << (7 - bit%8)
	}
	last := netip.AddrFrom16(end)
	if start.Is4() {
		last = last.Unmap()
	}
	return Range{start, last}
}

// ForPrefix maps the canonical IPv4 subtree at ::/96 into the requested family.
func ForPrefix(prefix netip.Prefix, ipv4 bool) (Range, bool) {
	r := FromPrefix(prefix)
	if prefix.Addr().Is4() == ipv4 {
		return r, true
	}
	if !ipv4 {
		convert := func(address netip.Addr) netip.Addr {
			var b [16]byte
			a := address.As4()
			copy(b[12:], a[:])
			return netip.AddrFrom16(b)
		}
		return Range{convert(r.Start), convert(r.End)}, true
	}
	r, ok := r.Intersect(FromPrefix(netip.MustParsePrefix("::/96")))
	if !ok {
		return Range{}, false
	}
	convert := func(address netip.Addr) netip.Addr { b := address.As16(); return netip.AddrFrom4([4]byte(b[12:])) }
	return Range{convert(r.Start), convert(r.End)}, true
}

func (r Range) Intersect(other Range) (Range, bool) {
	if r.Start.BitLen() != other.Start.BitLen() {
		return Range{}, false
	}
	if r.Start.Compare(other.Start) < 0 {
		r.Start = other.Start
	}
	if r.End.Compare(other.End) > 0 {
		r.End = other.End
	}
	return r, r.Start.Compare(r.End) <= 0
}

func (r Range) Cover() netip.Prefix {
	for bits := r.Start.BitLen(); bits >= 0; bits-- {
		prefix := netip.PrefixFrom(r.Start, bits).Masked()
		if prefix.Contains(r.End) {
			return prefix
		}
	}
	panic("valid ranges always have a covering prefix")
}

func (r Range) Size() *big.Int {
	start, end := r.Start.As16(), r.End.As16()
	count := new(big.Int).Sub(new(big.Int).SetBytes(end[:]), new(big.Int).SetBytes(start[:]))
	return count.Add(count, big.NewInt(1))
}
