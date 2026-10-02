package action

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTreeBindingsAreBufferDefaults(t *testing.T) {
	bufferBindings := DefaultBindings("buffer")
	if bufferBindings["Left"] != "FileTreeLeft" || bufferBindings["Right"] != "FileTreeRight" {
		t.Fatalf("tree navigation bindings missing from buffer defaults: %#v", bufferBindings)
	}
	if bufferBindings["F5"] != "FileTreeRefresh" {
		t.Fatalf("tree refresh binding missing from buffer defaults: %#v", bufferBindings)
	}

	commandBindings := DefaultBindings("command")
	if commandBindings["Left"] != "CursorLeft" || commandBindings["Right"] != "CursorRight" {
		t.Fatalf("command pane cursor bindings changed: %#v", commandBindings)
	}
	if _, ok := commandBindings["F5"]; ok {
		t.Fatal("tree refresh binding should not be assigned to the command pane")
	}
}

func TestFileTreeExpansion(t *testing.T) {
	root := t.TempDir()
	nestedDir := filepath.Join(root, "folder", "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{
		filepath.Join(root, "root.txt"),
		filepath.Join(root, "folder", "nested", "child.txt"),
	} {
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	tree, err := NewFileTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Entries) != 3 || !tree.Entries[0].IsParent || tree.Entries[1].Name != "folder" || tree.Entries[2].Name != "root.txt" {
		t.Fatalf("unexpected root entries: %#v", tree.Entries)
	}

	tree.expanded[filepath.Join(root, "folder")] = true
	tree.expanded[filepath.Join(root, "folder", "nested")] = true
	if err := tree.refresh(); err != nil {
		t.Fatal(err)
	}
	if len(tree.Entries) != 5 || tree.Entries[2].Name != "nested" || tree.Entries[3].Name != "child.txt" {
		t.Fatalf("unexpected expanded entries: %#v", tree.Entries)
	}
	if tree.Entries[3].Path != filepath.Join(root, "folder", "nested", "child.txt") {
		t.Fatalf("unexpected child path: %q", tree.Entries[3].Path)
	}

	clone, err := tree.clone()
	if err != nil {
		t.Fatal(err)
	}
	if len(clone.Entries) != len(tree.Entries) || !clone.expanded[filepath.Join(root, "folder")] {
		t.Fatalf("clone did not preserve expanded tree: %#v", clone.Entries)
	}
	clone.expanded[filepath.Join(root, "folder")] = false
	if !tree.expanded[filepath.Join(root, "folder")] {
		t.Fatal("clone shares expansion state with the original tree")
	}
}

func TestFileTreeNavigateToParent(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "child")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	tree, err := NewFileTree(root)
	if err != nil {
		t.Fatal(err)
	}

	selectedPath, err := tree.navigateToParent()
	if err != nil {
		t.Fatal(err)
	}
	if selectedPath != root || tree.Root != parent {
		t.Fatalf("parent navigation selected %q and set root to %q", selectedPath, tree.Root)
	}
	if len(tree.Entries) != 3 || !tree.Entries[0].IsParent || tree.Entries[1].Path != root || !tree.Entries[1].Expanded {
		t.Fatalf("former root was not expanded in parent tree: %#v", tree.Entries)
	}
	if tree.Entries[2].Path != filepath.Join(root, "file.txt") {
		t.Fatalf("former root contents were not shown: %#v", tree.Entries)
	}

	filesystemRoot := string(os.PathSeparator)
	if volume := filepath.VolumeName(root); volume != "" {
		filesystemRoot = volume + string(os.PathSeparator)
	}
	tree.Root = filesystemRoot
	if selectedPath, err := tree.navigateToParent(); err != nil || selectedPath != "" {
		t.Fatalf("filesystem root navigation returned %q, %v", selectedPath, err)
	}
}
