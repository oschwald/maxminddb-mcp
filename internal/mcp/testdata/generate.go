// Run from the repository's testdata module:
// go run ../internal/mcp/testdata/generate.go ../internal/mcp/testdata
package main

import (
	"fmt"
	"log"
	"maps"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/maxmind/mmdbwriter/v2"
	"github.com/maxmind/mmdbwriter/v2/mmdbtype"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatal("usage: generate output-directory")
	}
	for _, version := range []string{"old", "new", "dense", "groups"} {
		if err := generate(os.Args[1], version); err != nil {
			log.Fatal(err)
		}
	}
}

func generate(directory, version string) error {
	tree, err := mmdbwriter.New(mmdbwriter.Options{
		DatabaseType: "MCP test records", BuildEpoch: 1700000000,
		IPVersion: 6, IncludeReservedNetworks: true,
		Languages: []string{"en"}, Description: map[string]string{"en": "Query tool fixtures"},
	})
	if err != nil {
		return err
	}
	records := map[string]mmdbtype.DataType{
		"10.0.0.0/24":    mmdbtype.Map{"country": mmdbtype.String("US"), "asn": mmdbtype.Uint32(1)},
		"10.0.1.0/24":    mmdbtype.String(version),
		"10.0.2.0/24":    mmdbtype.Slice{mmdbtype.Uint32(7), mmdbtype.Map{"name": mmdbtype.String("array")}},
		"2001:db8::/120": mmdbtype.Map{"country": mmdbtype.String("US"), "asn": mmdbtype.Uint32(1)},
		"ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff/128": mmdbtype.Bool(true),
	}
	if version == "groups" {
		records = map[string]mmdbtype.DataType{"2001:db8:eeee::/128": mmdbtype.String(strings.Repeat("x", 1024))}
		address := netip.MustParseAddr("2001:db8:ffff::")
		for i := range 101 {
			records[netip.PrefixFrom(address, 128).String()] = mmdbtype.Uint32(i)
			address = address.Next()
		}
	} else if version == "dense" {
		records = map[string]mmdbtype.DataType{}
		address := netip.MustParseAddr("2001:db8:ffff::")
		for i := range 10001 {
			records[netip.PrefixFrom(address, 128).String()] = mmdbtype.Bool(i%2 == 0)
			address = address.Next()
		}
	} else if version == "old" {
		records["10.0.3.0/24"] = mmdbtype.Map{"country": mmdbtype.String("CA")}
	} else {
		delete(records, "10.0.0.0/24")
		// Different integer encodings prevent the writer from merging equal JSON answers.
		records["10.0.0.0/25"] = mmdbtype.Map{"country": mmdbtype.String("US"), "asn": mmdbtype.Uint16(1)}
		records["10.0.0.128/25"] = mmdbtype.Map{"country": mmdbtype.String("US"), "asn": mmdbtype.Uint32(1)}
		records["10.0.4.0/24"] = mmdbtype.Map{"country": mmdbtype.String("GB")}
	}
	for _, prefix := range slices.Sorted(maps.Keys(records)) {
		value := records[prefix]
		if err := tree.Insert(netip.MustParsePrefix(prefix), value); err != nil {
			return err
		}
	}
	file, err := os.Create(filepath.Join(directory, version+".mmdb"))
	if err != nil {
		return err
	}
	_, writeErr := tree.WriteTo(file)
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("write fixture: %w", writeErr)
	}
	return closeErr
}
