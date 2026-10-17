package database

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestZIPDirectoryLimits(t *testing.T) {
	data, err := os.ReadFile("../../testdata/test-data/GeoLite2-ASN-Test.mmdb")
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		entries   int
		declared  uint16
		wantError string
	}{
		{name: "boundary", entries: 1024, declared: 1024},
		{name: "too many", entries: 1025, declared: 1025, wantError: "more than 1024"},
		{name: "hidden entries", entries: 1025, declared: 1, wantError: "more than 1024"},
		{name: "oversized allocation", entries: 1, declared: 65534, wantError: "more than 1024"},
		{name: "count mismatch", entries: 2, declared: 1, wantError: zip.ErrFormat.Error()},
	} {
		t.Run(test.name, func(t *testing.T) {
			var archive bytes.Buffer
			writer := zip.NewWriter(&archive)
			for i := 0; i < test.entries; i++ {
				name := "README"
				if i == 0 {
					name = "database.mmdb"
				}
				file, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
				if err != nil {
					t.Fatal(err)
				}
				if i == 0 {
					if _, err := file.Write(data); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			body := archive.Bytes()
			binary.LittleEndian.PutUint16(body[len(body)-12:], test.declared)
			file := zipTestFile(t, body)
			var extracted bytes.Buffer
			err := extractMMDB(t.Context(), file, &extracted)
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) ||
					extracted.Len() != 0 {
					t.Fatalf(
						"archive was not rejected before extraction: %v, %d bytes",
						err,
						extracted.Len(),
					)
				}
			} else if err != nil || !bytes.Equal(extracted.Bytes(), data) {
				t.Fatalf("valid archive failed: %v", err)
			}
		})
	}
}

func TestZIPDirectoryValidation(t *testing.T) {
	body := archiveData(t, "zip", map[string][]byte{"db.mmdb": []byte("database")})
	le := binary.LittleEndian
	end := body[len(body)-22:]
	var end64 [76]byte
	copy(end64[:4], "PK\x06\x06")
	le.PutUint64(end64[4:], 44)
	le.PutUint64(end64[24:], 1)
	le.PutUint64(end64[32:], 1)
	le.PutUint64(end64[40:], uint64(le.Uint32(end[12:])))
	le.PutUint64(end64[48:], uint64(le.Uint32(end[16:])))
	copy(end64[56:60], "PK\x06\x07")
	le.PutUint64(end64[64:], uint64(len(body)-22))
	le.PutUint32(end64[72:], 1)
	zip64 := append(bytes.Clone(body[:len(body)-22]), end64[:]...)
	zip64 = append(zip64, end...)
	le.PutUint16(zip64[len(zip64)-12:], 0xffff)
	commented := append(bytes.Clone(body), []byte("PK\x05\x06")...)
	le.PutUint16(commented[len(body)-2:], 4)
	for _, test := range []struct {
		name      string
		body      []byte
		mutate    func([]byte)
		wantError bool
	}{
		{name: "zip64", body: zip64},
		{name: "comment with signature", body: commented},
		{name: "zip64 oversized allocation", body: zip64, wantError: true, mutate: func(b []byte) {
			le.PutUint64(b[len(b)-98+32:], 1<<32)
		}},
		{name: "zip64 invalid offset", body: zip64, wantError: true, mutate: func(b []byte) {
			le.PutUint64(b[len(b)-34:], ^uint64(0))
		}},
		{name: "truncated footer", body: body[:len(body)-1], wantError: true},
		{name: "directory offset", body: body, wantError: true, mutate: func(b []byte) {
			le.PutUint32(b[len(b)-6:], 0)
		}},
		{name: "entry length", body: body, wantError: true, mutate: func(b []byte) {
			offset := le.Uint32(b[len(b)-6:])
			le.PutUint16(b[int(offset)+28:], 0xffff)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := bytes.Clone(test.body)
			if test.mutate != nil {
				test.mutate(body)
			}
			file := zipTestFile(t, body)
			var extracted bytes.Buffer
			err := extractMMDB(t.Context(), file, &extracted)
			if test.wantError {
				if err == nil || extracted.Len() != 0 {
					t.Fatalf("malformed archive was not rejected: %v", err)
				}
			} else if err != nil || extracted.String() != "database" {
				t.Fatalf("valid ZIP64 failed: %q, %v", extracted.String(), err)
			}
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := checkZIPDirectory(
		ctx,
		zipTestFile(t, body),
		int64(len(body)),
	); !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf("directory check ignored cancellation: %v", err)
	}
}

func zipTestFile(t *testing.T, body []byte) *os.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "*.zip")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	if _, err := file.Write(body); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	return file
}
