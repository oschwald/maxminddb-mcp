package database

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var databaseNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}\.mmdb$`)

var downloadClient = &http.Client{
	Transport: func() *http.Transport {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.ResponseHeaderTimeout = 30 * time.Second
		return transport
	}(),
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many redirects")
		}
		if err := validateDownloadURL(req.URL); err != nil {
			return err
		}
		if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
			return errors.New("HTTPS downloads cannot redirect to HTTP")
		}
		// A signed URL can contain credentials in its path or query.
		req.Header.Del("Referer")
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
		return nil
	},
}

func validateDownloadURL(u *url.URL) error {
	if (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil ||
		u.Fragment != "" {
		return errors.New("use an HTTP or HTTPS URL without a username, password, or fragment")
	}
	return nil
}

// InstallURL imports one database without saving the URL or using MaxMind credentials.
func (m *Manager) InstallURL(
	ctx context.Context,
	dir, name, source string,
	overwrite bool,
	report func(string),
) (result UpdateResult) {
	result.Database = name
	if !databaseNamePattern.MatchString(name) {
		result.Error = "database must be a filename ending in .mmdb, using only letters, numbers, dots, underscores, and hyphens"
		return result
	}
	u, err := url.Parse(source)
	if err != nil {
		result.Error = "invalid download URL"
		return result
	}
	if err := validateDownloadURL(u); err != nil {
		result.Error = err.Error()
		return result
	}
	if dir == "" {
		result.Error = "configure a database directory before installing a database"
		return result
	}
	select {
	case <-ctx.Done():
		result.Error = ctx.Err().Error()
		return result
	case m.installing <- struct{}{}:
	}
	defer func() { <-m.installing }()
	if err := ctx.Err(); err != nil {
		result.Error = err.Error()
		return result
	}
	path, err := filepath.Abs(filepath.Join(dir, name))
	if err != nil {
		result.Error = "resolve database directory: " + err.Error()
		return result
	}
	unlock, err := lockDatabase(ctx, path)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer unlock()
	if existing, ok := m.GetDatabase(name); ok && existing.Path != path {
		result.Error = "a database with that name is already loaded from another directory"
		return result
	}
	if _, err := os.Lstat(path); err == nil && !overwrite {
		result.Error = "database already exists: choose another filename or set overwrite to true"
		return result
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		result.Error = "check destination: " + err.Error()
		return result
	}
	if report != nil {
		report("Downloading " + name)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		result.Error = "invalid download request"
		return result
	}
	response, err := downloadClient.Do(req)
	if err != nil {
		// Do not expose signed URLs through transport or redirect errors.
		result.Error = "download request failed: check the URL and network connection"
		if ctx.Err() != nil {
			result.Error = ctx.Err().Error()
		}
		return result
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("download returned HTTP %d", response.StatusCode)
		return result
	}
	if response.ContentLength > maxDatabaseBytes {
		result.Error = "database download exceeds 1 GiB"
		return result
	}
	archive, err := os.CreateTemp(dir, "download-*.tmp")
	if err != nil {
		result.Error = "create download file: " + err.Error()
		return result
	}
	defer func() { _ = archive.Close(); _ = os.Remove(archive.Name()) }()
	writer := io.Writer(archive)
	if report != nil {
		writer = io.MultiWriter(writer, &downloadProgress{report: report, edition: name})
	}
	if _, err := copyDatabase(ctx, writer, response.Body); err != nil {
		result.Error = "download did not complete within the 1 GiB limit"
		if ctx.Err() != nil {
			result.Error = ctx.Err().Error()
		}
		return result
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		result.Error = "read downloaded file: " + err.Error()
		return result
	}
	file, err := os.CreateTemp(dir, "database-*.tmp")
	if err != nil {
		result.Error = "create database file: " + err.Error()
		return result
	}
	defer func() { _ = os.Remove(file.Name()) }()
	if report != nil {
		report("Extracting and validating " + name)
	}
	extractErr := extractMMDB(ctx, archive, file)
	closeErr := file.Close()
	if err := errors.Join(extractErr, closeErr); err != nil {
		result.Error = "extract database: " + err.Error()
		return result
	}
	info, err := os.Stat(file.Name())
	if err != nil {
		result.Error = "read extracted database: " + err.Error()
		return result
	}
	modified, _ := http.ParseTime(response.Header.Get("Last-Modified"))
	if modified.IsZero() {
		modified = time.Now().UTC()
	}
	result = m.replaceDatabase(ctx, file.Name(), path, "", modified, info.Size())
	result.Database = name
	return result
}

func extractMMDB(ctx context.Context, source *os.File, target io.Writer) error {
	reader := bufio.NewReader(contextReader{ctx: ctx, reader: source})
	magic, _ := reader.Peek(4)
	if string(magic) == "PK\x03\x04" || string(magic) == "PK\x05\x06" {
		return extractZIP(ctx, source, target)
	}
	if len(magic) >= 2 && magic[0] == 0x1f && magic[1] == 0x8b {
		gz, err := gzip.NewReader(reader)
		if err != nil {
			return err
		}
		defer func() { _ = gz.Close() }()
		reader = bufio.NewReader(gz)
	}
	header, _ := reader.Peek(512)
	if len(header) == 512 {
		_, err := tar.NewReader(bytes.NewReader(header)).Next()
		if err == nil || string(header[257:262]) == "ustar" {
			return extractTAR(ctx, reader, target)
		}
	}
	_, err := copyDatabase(ctx, target, reader)
	return err
}

func extractTAR(ctx context.Context, source io.Reader, target io.Writer) error {
	limited := &io.LimitedReader{
		R: contextReader{ctx: ctx, reader: source},
		N: maxDatabaseBytes + 1,
	}
	archive := tar.NewReader(limited)
	found := false
	for entries := 0; ; entries++ {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		if entries >= 1024 {
			return errors.New("archive contains more than 1024 entries")
		}
		if !strings.HasSuffix(strings.ToLower(header.Name), ".mmdb") {
			continue
		}
		if found || header.Typeflag != tar.TypeReg {
			return errors.New("archive must contain exactly one regular .mmdb file")
		}
		found = true
		if _, err := copyDatabase(ctx, target, archive); err != nil {
			return err
		}
	}
	// Consume the gzip trailer so checksum failures cannot pass as successful extraction.
	if _, err := io.Copy(io.Discard, limited); err != nil {
		return err
	}
	if limited.N == 0 {
		return errors.New("expanded archive exceeds 1 GiB")
	}
	if !found {
		return errors.New("archive does not contain an .mmdb file")
	}
	return nil
}

func extractZIP(ctx context.Context, source *os.File, target io.Writer) error {
	info, err := source.Stat()
	if err != nil {
		return err
	}
	archive, err := zip.NewReader(source, info.Size())
	if err != nil {
		return err
	}
	if len(archive.File) > 1024 {
		return errors.New("archive contains more than 1024 entries")
	}
	var database *zip.File
	for _, file := range archive.File {
		if strings.HasSuffix(strings.ToLower(file.Name), ".mmdb") {
			if database != nil || !file.Mode().IsRegular() {
				return errors.New("archive must contain exactly one regular .mmdb file")
			}
			database = file
		}
	}
	if database == nil {
		return errors.New("archive does not contain an .mmdb file")
	}
	reader, err := database.Open()
	if err != nil {
		return err
	}
	defer func() { _ = reader.Close() }()
	_, err = copyDatabase(ctx, target, reader)
	return err
}
