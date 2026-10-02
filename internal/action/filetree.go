package action

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/micro-editor/micro/v2/internal/buffer"
	"github.com/micro-editor/micro/v2/internal/config"
	"github.com/micro-editor/micro/v2/internal/screen"
)

type FileTreeEntry struct {
	Path     string
	Name     string
	Depth    int
	IsDir    bool
	Expanded bool
	IsParent bool
}

type FileTree struct {
	Root     string
	Entries  []FileTreeEntry
	expanded map[string]bool
}

func NewFileTree(root string) (*FileTree, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, os.ErrInvalid
	}
	tree := &FileTree{
		Root:     root,
		expanded: make(map[string]bool),
	}
	if err := tree.refresh(); err != nil {
		return nil, err
	}
	return tree, nil
}

func (tree *FileTree) refresh() error {
	previousEntries := tree.Entries
	tree.Entries = nil
	if err := tree.readDir(tree.Root, 0); err != nil {
		tree.Entries = previousEntries
		return err
	}
	parent := filepath.Dir(tree.Root)
	if parent != tree.Root {
		parentEntry := FileTreeEntry{Path: parent, Name: "..", IsDir: true, IsParent: true}
		tree.Entries = append([]FileTreeEntry{parentEntry}, tree.Entries...)
	}
	return nil
}

func (tree *FileTree) navigateToParent() (string, error) {
	previousRoot := tree.Root
	parent := filepath.Dir(previousRoot)
	if parent == previousRoot {
		return "", nil
	}
	previousExpanded, wasExpanded := tree.expanded[previousRoot]
	tree.Root = parent
	tree.expanded[previousRoot] = true
	if err := tree.refresh(); err != nil {
		tree.Root = previousRoot
		if wasExpanded {
			tree.expanded[previousRoot] = previousExpanded
		} else {
			delete(tree.expanded, previousRoot)
		}
		return "", err
	}
	return previousRoot, nil
}

func (tree *FileTree) clone() (*FileTree, error) {
	clone, err := NewFileTree(tree.Root)
	if err != nil {
		return nil, err
	}
	for path, expanded := range tree.expanded {
		clone.expanded[path] = expanded
	}
	if err := clone.refresh(); err != nil {
		return nil, err
	}
	return clone, nil
}

func (tree *FileTree) readDir(path string, depth int) error {
	items, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !item.IsDir() {
			continue
		}
		itemPath := filepath.Join(path, item.Name())
		tree.appendEntry(itemPath, item.Name(), depth, true)
		if tree.expanded[itemPath] {
			_ = tree.readDir(itemPath, depth+1)
		}
	}
	for _, item := range items {
		if item.IsDir() {
			continue
		}
		tree.appendEntry(filepath.Join(path, item.Name()), item.Name(), depth, false)
	}
	return nil
}

func (tree *FileTree) appendEntry(path, name string, depth int, isDir bool) {
	entry := FileTreeEntry{
		Path:     path,
		Name:     name,
		Depth:    depth,
		IsDir:    isDir,
		Expanded: tree.expanded[path],
	}
	tree.Entries = append(tree.Entries, entry)
}

func (tree *FileTree) buffer() *buffer.Buffer {
	var lines []string
	for _, entry := range tree.Entries {
		prefix := strings.Repeat("  ", entry.Depth)
		if entry.IsParent {
			lines = append(lines, prefix+"[..] ../")
		} else if entry.IsDir {
			marker := "[+] "
			if entry.Expanded {
				marker = "[-] "
			}
			lines = append(lines, prefix+marker+entry.Name)
		} else {
			lines = append(lines, prefix+"    "+entry.Name)
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "(empty)")
	}
	b := buffer.NewBufferFromString(strings.Join(lines, "\n"), "", buffer.BTFileTree)
	b.SetName("File tree: " + tree.Root)
	return b
}

func (tree *FileTree) entryAt(index int) (FileTreeEntry, bool) {
	if index < 0 || index >= len(tree.Entries) {
		return FileTreeEntry{}, false
	}
	return tree.Entries[index], true
}

func (tree *FileTree) indexOf(path string) int {
	for i, entry := range tree.Entries {
		if entry.Path == path {
			return i
		}
	}
	return -1
}

func (tree *FileTree) refreshPane(pane *BufPane, selectedPath string) {
	if err := tree.refresh(); err != nil {
		InfoBar.Error(err)
		return
	}
	tree.renderPane(pane, selectedPath)
}

func (tree *FileTree) renderPane(pane *BufPane, selectedPath string) {
	pane.OpenBuffer(tree.buffer())
	if index := tree.indexOf(selectedPath); index >= 0 {
		pane.Cursor.X = 0
		pane.Cursor.Y = index
	}
	pane.Relocate()
}

func (h *BufPane) navigateFileTreeParent() {
	selectedPath, err := h.fileTree.navigateToParent()
	if err != nil {
		InfoBar.Error(err)
	} else if selectedPath != "" {
		h.fileTree.renderPane(h, selectedPath)
	}
}

func (h *BufPane) addFileTreeToTab(tab *Tab, selectedPath string) error {
	for _, pane := range tab.Panes {
		if treePane, ok := pane.(*BufPane); ok && treePane.fileTree != nil {
			if index := treePane.fileTree.indexOf(selectedPath); index >= 0 {
				treePane.Cursor.X = 0
				treePane.Cursor.Y = index
			}
			treePane.Relocate()
			return nil
		}
	}
	var editorPane *BufPane
	for _, pane := range tab.Panes {
		if bufPane, ok := pane.(*BufPane); ok && bufPane.fileTree == nil {
			editorPane = bufPane
			break
		}
	}
	if editorPane == nil {
		return nil
	}
	tree, err := h.fileTree.clone()
	if err != nil {
		return err
	}
	treePane := editorPane.VSplitIndex(tree.buffer(), false)
	treePane.fileTree = tree
	resizeFileTreePane(treePane)
	if index := tree.indexOf(selectedPath); index >= 0 {
		treePane.Cursor.X = 0
		treePane.Cursor.Y = index
	}
	treePane.Relocate()
	tab.SetActive(tab.GetPane(editorPane.ID()))
	return nil
}

func resizeFileTreePane(treePane *BufPane) {
	width := treePane.tab.W / 4
	if width < 20 {
		width = treePane.tab.W / 3
	}
	if width > 0 {
		treePane.ResizePane(width)
	}
}

func (h *BufPane) FileTreeCmd(args []string) {
	for i, pane := range h.tab.Panes {
		treePane, ok := pane.(*BufPane)
		if !ok || treePane.fileTree == nil {
			continue
		}
		if h.tab.GetNode(treePane.ID()).Unsplit() {
			treePane.Close()
			h.tab.RemovePane(i)
			h.tab.Resize()
			active := i
			if active >= len(h.tab.Panes) {
				active = len(h.tab.Panes) - 1
			}
			h.tab.SetActive(active)
		}
		return
	}
	root, err := os.Getwd()
	if err != nil {
		InfoBar.Error(err)
		return
	}
	tree, err := NewFileTree(root)
	if err != nil {
		InfoBar.Error(err)
		return
	}
	treePane := h.VSplitIndex(tree.buffer(), false)
	treePane.fileTree = tree
	resizeFileTreePane(treePane)
}

func (h *BufPane) FileTreeOpen() bool {
	if h.fileTree == nil {
		return h.InsertNewline()
	}
	entry, ok := h.fileTree.entryAt(h.Cursor.Y)
	if !ok {
		return true
	}
	if entry.IsParent {
		h.navigateFileTreeParent()
		return true
	}
	if entry.IsDir {
		h.fileTree.expanded[entry.Path] = !entry.Expanded
		h.fileTree.refreshPane(h, entry.Path)
		return true
	}
	b, err := buffer.NewBufferFromFile(entry.Path, buffer.BTDefault)
	if err != nil {
		InfoBar.Error(err)
		return true
	}
	for tabIndex, tab := range Tabs.List {
		for _, pane := range tab.Panes {
			if editorPane, ok := pane.(*BufPane); ok && editorPane.fileTree == nil && editorPane.Buf.SharedBuffer == b.SharedBuffer {
				if err := h.addFileTreeToTab(tab, entry.Path); err != nil {
					InfoBar.Error(err)
				}
				Tabs.SetActive(tabIndex)
				tab.SetActive(tab.GetPane(editorPane.ID()))
				return true
			}
		}
	}
	width, height := screen.Screen.Size()
	iOffset := config.GetInfoBarOffset()
	tab := NewTabFromBuffer(0, 0, width, height-1-iOffset, b)
	if err := h.addFileTreeToTab(tab, entry.Path); err != nil {
		InfoBar.Error(err)
	}
	Tabs.AddTab(tab)
	Tabs.SetActive(len(Tabs.List) - 1)
	return true
}

func (h *BufPane) FileTreeLeft() bool {
	if h.fileTree == nil {
		return h.CursorLeft()
	}
	entry, ok := h.fileTree.entryAt(h.Cursor.Y)
	if !ok {
		return true
	}
	if entry.IsParent {
		h.navigateFileTreeParent()
		return true
	}
	if entry.IsDir && entry.Expanded {
		h.fileTree.expanded[entry.Path] = false
		h.fileTree.refreshPane(h, entry.Path)
		return true
	}
	if entry.Depth == 0 {
		h.navigateFileTreeParent()
		return true
	}
	parent := filepath.Dir(entry.Path)
	if parent != h.fileTree.Root {
		h.fileTree.expanded[parent] = false
		h.fileTree.refreshPane(h, parent)
	}
	return true
}

func (h *BufPane) FileTreeRight() bool {
	if h.fileTree == nil {
		return h.CursorRight()
	}
	entry, ok := h.fileTree.entryAt(h.Cursor.Y)
	if !ok || entry.IsParent || !entry.IsDir {
		return true
	}
	if !entry.Expanded {
		h.fileTree.expanded[entry.Path] = true
		h.fileTree.refreshPane(h, entry.Path)
		return true
	}
	if next, ok := h.fileTree.entryAt(h.Cursor.Y + 1); ok && next.Depth > entry.Depth {
		h.Cursor.Y++
		h.Cursor.X = 0
		h.Relocate()
	}
	return true
}

func (h *BufPane) FileTreeRefresh() bool {
	if h.fileTree == nil {
		return false
	}
	selectedPath := ""
	if entry, ok := h.fileTree.entryAt(h.Cursor.Y); ok {
		selectedPath = entry.Path
	}
	h.fileTree.refreshPane(h, selectedPath)
	return true
}
