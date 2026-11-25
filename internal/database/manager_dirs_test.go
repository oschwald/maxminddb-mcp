package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// copyTestDB copies the City test database to dir/name.
func copyTestDB(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(testDBPath)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	//nolint:gosec // Callers pass directories from t.TempDir.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// resolvedTempDir returns a temporary directory with symlinks resolved, such as
// /var to /private/var on macOS.
func resolvedTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func waitForCondition(t *testing.T, description string, ready func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ready() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", description)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestLoadDirectorySymlinkedRoot(t *testing.T) {
	target := resolvedTempDir(t)
	if err := os.Mkdir(filepath.Join(target, "sub"), 0o750); err != nil {
		t.Fatal(err)
	}
	copyTestDB(t, target, "root.mmdb")
	copyTestDB(t, filepath.Join(target, "sub"), "nested.mmdb")
	link := filepath.Join(t.TempDir(), "databases")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}

	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if err := manager.LoadDirectory(link); err != nil {
		t.Fatal(err)
	}
	if err := manager.WatchDirectory(link); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"root.mmdb":   filepath.Join(target, "root.mmdb"),
		"nested.mmdb": filepath.Join(target, "sub", "nested.mmdb"),
	} {
		info, ok := manager.GetDatabase(name)
		if !ok {
			t.Fatalf("%s was not loaded through the symlinked root", name)
		}
		if info.Path != want {
			t.Errorf("%s path = %s, want %s", name, info.Path, want)
		}
	}

	// The updater and installer build paths from the configured directory.
	if err := manager.LoadDatabase(filepath.Join(link, "root.mmdb")); err != nil {
		t.Fatalf("reload through the configured path failed: %v", err)
	}
	manager.RemoveDatabaseByPath(filepath.Join(link, "root.mmdb"))
	if _, ok := manager.GetDatabase("root.mmdb"); ok {
		t.Fatal("removal through the configured path left the database loaded")
	}
}

func TestLoadDirectorySkipsUnreadableSubdirectory(t *testing.T) {
	dir := resolvedTempDir(t)
	copyTestDB(t, dir, "readable.mmdb")
	locked := filepath.Join(dir, "lost+found")
	if err := os.Mkdir(locked, 0o750); err != nil {
		t.Fatal(err)
	}
	copyTestDB(t, locked, "hidden.mmdb")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // Restore access so t.TempDir can clean up.
	t.Cleanup(func() { _ = os.Chmod(locked, 0o750) })
	if _, err := os.ReadDir(locked); err == nil {
		t.Skip("directory permissions are not enforced for this user")
	}

	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if err := manager.LoadDirectory(dir); err != nil {
		t.Fatalf("unreadable subdirectory failed the load: %v", err)
	}
	if _, ok := manager.GetDatabase("readable.mmdb"); !ok {
		t.Fatal("database beside the unreadable subdirectory was not loaded")
	}
	if err := manager.LoadDirectory(locked); err == nil {
		t.Fatal("unreadable root did not fail the load")
	}
}

func TestLoadDirectoryMissingRoot(t *testing.T) {
	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	missing := filepath.Join(t.TempDir(), "missing")
	err = manager.LoadDirectory(missing)
	if err == nil || !strings.Contains(err.Error(), "directory does not exist") {
		t.Fatalf("missing root error = %v", err)
	}
}

func TestRemoveDatabaseByPathDirectory(t *testing.T) {
	dir := resolvedTempDir(t)
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	copyTestDB(t, dir, "kept.mmdb")
	copyTestDB(t, sub, "nested.mmdb")
	// A sibling whose name shares the prefix must not be removed.
	if err := os.Mkdir(sub+"-other", 0o750); err != nil {
		t.Fatal(err)
	}
	copyTestDB(t, sub+"-other", "other.mmdb")

	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if err := manager.LoadDirectory(dir); err != nil {
		t.Fatal(err)
	}
	manager.RemoveDatabaseByPath(sub)
	if _, ok := manager.GetDatabase("nested.mmdb"); ok {
		t.Fatal("database below the removed directory is still loaded")
	}
	for _, name := range []string{"kept.mmdb", "other.mmdb"} {
		if _, ok := manager.GetDatabase(name); !ok {
			t.Fatalf("%s was removed with an unrelated directory", name)
		}
	}
}

func TestWatchSubdirectoryMovedOut(t *testing.T) {
	dir := resolvedTempDir(t)
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	copyTestDB(t, dir, "kept.mmdb")
	copyTestDB(t, sub, "nested.mmdb")

	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if err := manager.LoadDirectory(dir); err != nil {
		t.Fatal(err)
	}
	if err := manager.WatchDirectory(dir); err != nil {
		t.Fatal(err)
	}
	manager.StartWatching()
	if _, ok := manager.GetDatabase("nested.mmdb"); !ok {
		t.Fatal("nested database was not loaded")
	}

	if err := os.Rename(sub, filepath.Join(t.TempDir(), "moved")); err != nil {
		t.Fatal(err)
	}
	waitForCondition(t, "removal of the moved subdirectory", func() bool {
		_, ok := manager.GetDatabase("nested.mmdb")
		return !ok
	})
	if _, ok := manager.GetDatabase("kept.mmdb"); !ok {
		t.Fatal("moving a subdirectory removed the database beside it")
	}
}

func TestRemoveConfiguredRootKeepsDatabases(t *testing.T) {
	dir := resolvedTempDir(t)
	copyTestDB(t, dir, "kept.mmdb")

	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if err := manager.LoadDirectory(dir); err != nil {
		t.Fatal(err)
	}
	manager.RemoveDatabaseByPath(dir)
	if _, ok := manager.GetDatabase("kept.mmdb"); !ok {
		t.Fatal("an event for the configured directory unloaded its databases")
	}
}

func TestLoadDirectorySymlinkedDatabase(t *testing.T) {
	targetDir := resolvedTempDir(t)
	target := copyTestDB(t, targetDir, "target.mmdb")
	dir := resolvedTempDir(t)
	link := filepath.Join(dir, "linked.mmdb")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}

	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if err := manager.LoadDirectory(dir); err != nil {
		t.Fatal(err)
	}
	info, ok := manager.GetDatabase("linked.mmdb")
	if !ok {
		t.Fatal("symlinked database was not loaded")
	}
	if info.Size != targetInfo.Size() {
		t.Errorf("size = %d, want the target's size %d", info.Size, targetInfo.Size())
	}
	if info.Path != link {
		t.Errorf("path = %s, want %s", info.Path, link)
	}

	// A file event for the unchanged link must not reload the database.
	if err := manager.LoadDatabase(link); err != nil {
		t.Fatal(err)
	}
	reloaded, ok := manager.GetDatabase("linked.mmdb")
	if !ok {
		t.Fatal("symlinked database was unloaded")
	}
	if reloaded.Generation != info.Generation {
		t.Error("unchanged symlinked database was reloaded")
	}
}

func TestLoadDirectorySkipsBrokenSymlinksAndDirectories(t *testing.T) {
	dir := resolvedTempDir(t)
	copyTestDB(t, dir, "good.mmdb")
	if err := os.Symlink(
		filepath.Join(dir, "missing.mmdb"),
		filepath.Join(dir, "broken.mmdb"),
	); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "dir.mmdb")); err != nil {
		t.Fatal(err)
	}

	manager, err := New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if err := manager.LoadDirectory(dir); err != nil {
		t.Fatalf("broken symlink failed the load: %v", err)
	}
	if _, ok := manager.GetDatabase("good.mmdb"); !ok {
		t.Fatal("database beside the broken symlink was not loaded")
	}
	for _, name := range []string{"broken.mmdb", "dir.mmdb"} {
		if _, ok := manager.GetDatabase(name); ok {
			t.Errorf("%s was loaded", name)
		}
	}
}
