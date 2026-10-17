package iprange

import (
	"math/big"
	"net/netip"
	"testing"
)

func TestPrefixRanges(t *testing.T) {
	for _, tc := range []struct{ prefix, end, size string }{
		{"0.0.0.0/0", "255.255.255.255", "4294967296"},
		{"10.0.0.9/24", "10.0.0.255", "256"},
		{"::/0", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "340282366920938463463374607431768211456"},
		{"2001:db8::/120", "2001:db8::ff", "256"},
		{"ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff/128", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "1"},
	} {
		prefix := netip.MustParsePrefix(tc.prefix)
		r := FromPrefix(prefix)
		if r.End.String() != tc.end || r.Size().String() != tc.size ||
			r.Cover() != prefix.Masked() {
			t.Fatalf("%s: %+v (%s)", tc.prefix, r, r.Size())
		}
	}
	for _, tc := range [][2]string{{"1.0.0.2", "1.0.0.1"}, {"1.0.0.1", "::1"}, {"fe80::1%eth0", "fe80::2%eth0"}} {
		if _, err := New(netip.MustParseAddr(tc[0]), netip.MustParseAddr(tc[1])); err == nil {
			t.Fatalf("accepted invalid range: %v", tc)
		}
	}
	r, err := New(netip.MustParseAddr("10.0.0.7"), netip.MustParseAddr("10.0.0.130"))
	if err != nil || r.Cover().String() != "10.0.0.0/24" || r.Size().String() != "124" {
		t.Fatalf("range: %+v, %v", r, err)
	}
	canonical, ok := ForPrefix(netip.MustParsePrefix("10.0.0.0/24"), false)
	if !ok || canonical.Start.String() != "::a00:0" || canonical.Size().String() != "256" {
		t.Fatalf("canonical range: %+v", canonical)
	}
	v4, ok := ForPrefix(netip.MustParsePrefix("::/0"), true)
	if !ok || v4.Cover().String() != "0.0.0.0/0" {
		t.Fatalf("IPv4 range: %+v", v4)
	}
	if _, ok := ForPrefix(netip.MustParsePrefix("2001:db8::/32"), true); ok {
		t.Fatal("unrelated IPv6 range became IPv4")
	}
}

func TestRangePrefixes(t *testing.T) {
	for _, endpoints := range [][2]string{
		{"10.0.0.1", "10.0.3.254"},
		{"1.255.255.255", "2.0.0.0"},
		{"0.0.0.0", "255.255.255.255"},
		{"::", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"},
		{"::1", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:fffe"},
		{"2001:db8::7", "2001:db8::130"},
		{"ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"},
	} {
		r, err := New(netip.MustParseAddr(endpoints[0]), netip.MustParseAddr(endpoints[1]))
		if err != nil {
			t.Fatal(err)
		}
		prefixes := r.Prefixes()
		if len(prefixes) > 254 {
			t.Fatalf("unbounded decomposition: %d", len(prefixes))
		}
		next := r.Start
		total := new(big.Int)
		for _, prefix := range prefixes {
			span := FromPrefix(prefix)
			if span.Start != next || span.End.Compare(r.End) > 0 || prefix != prefix.Masked() {
				t.Fatalf("incorrect cover %s in %+v", prefix, r)
			}
			total.Add(total, span.Size())
			next = span.End.Next()
		}
		if total.Cmp(r.Size()) != 0 {
			t.Fatalf("incomplete cover: %v", prefixes)
		}
	}
	if _, err := New(
		netip.MustParseAddr("::ffff:10.0.0.1%zone"),
		netip.MustParseAddr("10.0.0.2"),
	); err == nil {
		t.Fatal("accepted zoned mapped address")
	}
}
