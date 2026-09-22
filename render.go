package diskchecker

// render.go is the fyne front end for the scanner (scanner.go) and the node tree (tree.go).
//
// The whole UI is one call:
//
//	diskchecker.Run("/home/me")
//
// To put it inside an app you already have:
//
//	w := diskchecker.NewWindow(myApp, "/home/me")
//	w.Show()
//
// To embed only the view in your own window:
//
//	browser := diskchecker.NewBrowser(w, "/home/me")
//	w.SetContent(browser.Content())

import (
	"errors"
	"fmt"
	"image/color"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	fyneTheme "fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const (
	appID         = "com.diskchecker.app"
	windowTitle   = "Disk Checker"
	windowWidth   = 1000
	windowHeight  = 650
	treeSplitSize = 0.4
)

// Run scans rootPath and shows the disk checker window, blocking until it is closed.
func Run(rootPath string) {
	a := app.NewWithID(appID)
	NewWindow(a, rootPath).ShowAndRun()
}

// NewWindow creates a disk checker window in an existing app and starts scanning rootPath.
// Call Show or ShowAndRun on the result.
func NewWindow(a fyne.App, rootPath string) fyne.Window {
	w := a.NewWindow(windowTitle)
	w.Resize(fyne.NewSize(windowWidth, windowHeight))
	w.SetContent(NewBrowser(w, rootPath).Content())
	return w
}

// Browser is the disk checker view: a folder tree, the selected item's details,
// file actions and a chart of the largest items.
type Browser struct {
	window   fyne.Window
	rootPath string
	scanner  *Scanner
	scanID   int // ignores results from scans that were superseded by a newer one
	variant  fyne.ThemeVariant
	selected *node
	listed   []*node // children of the selected directory, largest first

	totals map[*node]int64             // recursive size of every node
	ids    map[widget.TreeNodeID]*node // tree ids handed out to widget.Tree

	content   fyne.CanvasObject
	pathEntry *widget.Entry
	status    *widget.Label
	tree      *widget.Tree
	crumbs    *fyne.Container
	largest   *widget.List

	nameValue  *widget.Label
	pathValue  *widget.Label
	typeValue  *widget.Label
	sizeValue  *widget.Label
	itemsValue *widget.Label
	errValue   *widget.Label

	openButton   *widget.Button
	folderButton *widget.Button
	renameButton *widget.Button
	deleteButton *widget.Button
}

// NewBrowser builds the view for window w and starts scanning rootPath in the background.
// Pass an empty rootPath to start with an empty view and let the user pick a folder.
func NewBrowser(w fyne.Window, rootPath string) *Browser {
	b := &Browser{window: w}
	b.variant = fyne.CurrentApp().Settings().ThemeVariant()
	fyne.CurrentApp().Settings().SetTheme(&appTheme{variant: b.variant})
	b.build()
	if rootPath != "" {
		b.Scan(rootPath)
	}
	return b
}

// Content returns the view to place in a window.
func (b *Browser) Content() fyne.CanvasObject {
	return b.content
}

// Scan (re)scans rootPath and shows the result once it is ready. Safe to call repeatedly.
func (b *Browser) Scan(rootPath string) {
	absPath, err := filepath.Abs(rootPath)
	if err != nil {
		b.setStatus("Invalid path: " + err.Error())
		return
	}
	b.scanID++
	scanID := b.scanID
	b.rootPath = absPath
	b.pathEntry.SetText(absPath)
	b.setStatus("Scanning " + absPath + " …")

	go func() {
		scanner, err := NewScanner(absPath)
		fyne.Do(func() {
			if scanID != b.scanID {
				return
			}
			if err != nil {
				b.setStatus("Scan failed: " + err.Error())
				return
			}
			b.load(scanner)
		})
	}()
}

func (b *Browser) build() {
	b.pathEntry = widget.NewEntry()
	b.pathEntry.SetPlaceHolder("Folder to scan")
	b.pathEntry.OnSubmitted = b.Scan
	b.status = widget.NewLabel("Choose a folder to scan")
	b.crumbs = container.NewHBox()

	b.tree = widget.NewTree(b.treeChildren, b.treeIsBranch, newTreeRow, b.updateTreeRow)
	b.tree.OnSelected = func(id widget.TreeNodeID) {
		if n := b.ids[id]; n != nil {
			b.show(n)
		}
	}

	b.largest = widget.NewList(
		func() int { return len(b.listed) },
		newChartRow,
		func(i widget.ListItemID, row fyne.CanvasObject) { b.updateChartRow(b.listed[i], row) },
	)
	b.largest.OnSelected = func(i widget.ListItemID) {
		b.largest.UnselectAll()
		b.selectNode(b.listed[i])
	}

	toolbar := container.NewHBox(
		widget.NewButtonWithIcon("Browse", fyneTheme.FolderOpenIcon(), b.browse),
		widget.NewButtonWithIcon("Scan", fyneTheme.ViewRefreshIcon(), func() { b.Scan(b.pathEntry.Text) }),
		widget.NewButtonWithIcon("", fyneTheme.ColorPaletteIcon(), b.toggleTheme),
	)
	top := container.NewVBox(
		container.NewBorder(nil, nil, nil, toolbar, b.pathEntry),
		container.NewHScroll(b.crumbs),
	)

	split := container.NewHSplit(b.tree, b.buildDetails())
	split.Offset = treeSplitSize
	b.content = container.NewBorder(top, b.status, nil, nil, split)
	b.show(nil)
}

func (b *Browser) buildDetails() fyne.CanvasObject {
	newValue := func() *widget.Label {
		label := widget.NewLabel("")
		label.Wrapping = fyne.TextWrapBreak
		return label
	}
	b.nameValue, b.pathValue, b.typeValue = newValue(), newValue(), newValue()
	b.sizeValue, b.itemsValue, b.errValue = newValue(), newValue(), newValue()

	form := widget.NewForm(
		widget.NewFormItem("Name", b.nameValue),
		widget.NewFormItem("Path", b.pathValue),
		widget.NewFormItem("Type", b.typeValue),
		widget.NewFormItem("Size", b.sizeValue),
		widget.NewFormItem("Items", b.itemsValue),
		widget.NewFormItem("Error", b.errValue),
	)

	b.openButton = widget.NewButtonWithIcon("Open", fyneTheme.FileApplicationIcon(), func() { b.open(b.selected) })
	b.folderButton = widget.NewButtonWithIcon("Show in folder", fyneTheme.FolderIcon(), func() { b.open(b.selected.parent) })
	b.renameButton = widget.NewButtonWithIcon("Rename", fyneTheme.DocumentCreateIcon(), func() { b.rename(b.selected) })
	b.deleteButton = widget.NewButtonWithIcon("Delete", fyneTheme.DeleteIcon(), func() { b.delete(b.selected) })
	b.deleteButton.Importance = widget.DangerImportance
	actions := container.NewGridWrap(fyne.NewSize(150, 36), b.openButton, b.folderButton, b.renameButton, b.deleteButton)

	header := container.NewVBox(
		form,
		actions,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Largest items", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
	)
	return container.NewBorder(header, nil, nil, nil, b.largest)
}

// load swaps in a finished scan and selects its root.
func (b *Browser) load(scanner *Scanner) {
	b.scanner = scanner
	b.totals = map[*node]int64{}
	b.ids = map[widget.TreeNodeID]*node{}
	root := scanner.GetRoot()
	b.computeTotal(root)

	b.tree.Refresh()
	rootID := b.idOf(root)
	b.tree.OpenBranch(rootID)
	b.tree.Select(rootID)
	b.setStatus(fmt.Sprintf("Scanned %s: %s in %d items", b.rootPath, humanSize(b.totals[root]), countItems(root)))
}

// computeTotal stores the recursive size of n and sorts every directory largest first.
func (b *Browser) computeTotal(n *node) int64 {
	total := n.size
	if n.isDir {
		total = 0
		for _, child := range n.children {
			total += b.computeTotal(child)
		}
		sort.SliceStable(n.children, func(i, j int) bool {
			return b.totals[n.children[i]] > b.totals[n.children[j]]
		})
	}
	b.totals[n] = total
	return total
}

// show fills the details panel, breadcrumbs and chart for n (nil clears them).
func (b *Browser) show(n *node) {
	b.selected = n
	b.listed = nil
	b.updateBreadcrumbs(n)

	hasNode := n != nil
	setEnabled(b.openButton, hasNode)
	setEnabled(b.folderButton, hasNode && !n.IsRoot())
	setEnabled(b.renameButton, hasNode && !n.IsRoot())
	setEnabled(b.deleteButton, hasNode && !n.IsRoot())

	if !hasNode {
		for _, label := range []*widget.Label{b.nameValue, b.pathValue, b.typeValue, b.sizeValue, b.itemsValue, b.errValue} {
			label.SetText("-")
		}
		b.largest.Refresh()
		return
	}

	b.nameValue.SetText(n.name)
	b.pathValue.SetText(b.pathOf(n))
	b.sizeValue.SetText(fmt.Sprintf("%s (%d bytes)", humanSize(b.totals[n]), b.totals[n]))
	b.errValue.SetText("-")
	if n.err != nil {
		b.errValue.SetText(n.err.Error())
	}
	if n.isDir {
		b.typeValue.SetText("Directory")
		b.itemsValue.SetText(fmt.Sprintf("%d direct, %d total", len(n.children), countItems(n)))
		b.listed = n.children
	} else {
		b.typeValue.SetText("File")
		b.itemsValue.SetText("-")
	}
	b.largest.Refresh()
	b.largest.ScrollToTop()
}

// selectNode expands the tree down to n and selects it.
func (b *Browser) selectNode(n *node) {
	if n == nil {
		return
	}
	for p := n.parent; p != nil; p = p.parent {
		b.tree.OpenBranch(b.idOf(p))
	}
	id := b.idOf(n)
	b.tree.Select(id)
	b.tree.ScrollTo(id)
}

func (b *Browser) updateBreadcrumbs(n *node) {
	var path []*node
	for current := n; current != nil; current = current.parent {
		path = append([]*node{current}, path...)
	}

	b.crumbs.RemoveAll()
	for i, crumb := range path {
		if i > 0 {
			b.crumbs.Add(widget.NewLabel("›"))
		}
		button := widget.NewButton(crumb.name, func() { b.selectNode(crumb) })
		button.Importance = widget.LowImportance
		b.crumbs.Add(button)
	}
}

// Tree callbacks. Tree ids are node pointers; "" is widget.Tree's invisible root.

func (b *Browser) idOf(n *node) widget.TreeNodeID {
	id := fmt.Sprintf("%p", n)
	b.ids[id] = n
	return id
}

func (b *Browser) treeChildren(id widget.TreeNodeID) []widget.TreeNodeID {
	if b.scanner == nil {
		return nil
	}
	if id == "" {
		return []widget.TreeNodeID{b.idOf(b.scanner.GetRoot())}
	}
	n := b.ids[id]
	if n == nil {
		return nil
	}
	ids := make([]widget.TreeNodeID, len(n.children))
	for i, child := range n.children {
		ids[i] = b.idOf(child)
	}
	return ids
}

func (b *Browser) treeIsBranch(id widget.TreeNodeID) bool {
	if id == "" {
		return true
	}
	n := b.ids[id]
	return n != nil && n.isDir
}

// newTreeRow lays out icon | name | size. NewBorder orders Objects as: name, icon, size.
func newTreeRow(bool) fyne.CanvasObject {
	name := widget.NewLabel("")
	name.Truncation = fyne.TextTruncateEllipsis
	return container.NewBorder(nil, nil, widget.NewIcon(nil), widget.NewLabel(""), name)
}

func (b *Browser) updateTreeRow(id widget.TreeNodeID, _ bool, row fyne.CanvasObject) {
	n := b.ids[id]
	if n == nil {
		return
	}
	objects := row.(*fyne.Container).Objects
	objects[0].(*widget.Label).SetText(n.name)
	objects[1].(*widget.Icon).SetResource(iconFor(n))
	objects[2].(*widget.Label).SetText(humanSize(b.totals[n]))
}

// newChartRow draws a size bar behind name | size. Objects: bar, then (name, size).
func newChartRow() fyne.CanvasObject {
	bar := widget.NewProgressBar()
	bar.TextFormatter = func() string { return "" }
	name := widget.NewLabel("")
	name.Truncation = fyne.TextTruncateEllipsis
	return container.NewStack(bar, container.NewBorder(nil, nil, nil, widget.NewLabel(""), name))
}

func (b *Browser) updateChartRow(n *node, row fyne.CanvasObject) {
	objects := row.(*fyne.Container).Objects
	labels := objects[1].(*fyne.Container).Objects

	share := 0.0
	if parentTotal := b.totals[n.parent]; parentTotal > 0 {
		share = float64(b.totals[n]) / float64(parentTotal)
	}
	objects[0].(*widget.ProgressBar).SetValue(share)
	labels[0].(*widget.Label).SetText(n.name)
	labels[1].(*widget.Label).SetText(fmt.Sprintf("%s  %.1f%%", humanSize(b.totals[n]), share*100))
}

// Actions

func (b *Browser) browse() {
	dialog.ShowFolderOpen(func(folder fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, b.window)
			return
		}
		if folder != nil {
			b.Scan(folder.Path())
		}
	}, b.window)
}

func (b *Browser) open(n *node) {
	if n == nil {
		return
	}
	fileURL := &url.URL{Scheme: "file", Path: b.pathOf(n)}
	if err := fyne.CurrentApp().OpenURL(fileURL); err != nil {
		dialog.ShowError(err, b.window)
	}
}

func (b *Browser) rename(n *node) {
	if n == nil || n.IsRoot() {
		return
	}
	entry := widget.NewEntry()
	entry.SetText(n.name)
	entry.Validator = func(name string) error {
		if name == "" || name == "." || name == ".." || strings.ContainsRune(name, os.PathSeparator) {
			return errors.New("invalid file name")
		}
		return nil
	}

	items := []*widget.FormItem{widget.NewFormItem("New name", entry)}
	dialog.ShowForm("Rename", "Rename", "Cancel", items, func(confirmed bool) {
		if !confirmed || entry.Text == n.name {
			return
		}
		oldPath := b.pathOf(n)
		newPath := filepath.Join(filepath.Dir(oldPath), entry.Text)
		if _, err := os.Lstat(newPath); err == nil {
			dialog.ShowError(fmt.Errorf("%s already exists", newPath), b.window)
			return
		}
		if err := os.Rename(oldPath, newPath); err != nil {
			dialog.ShowError(err, b.window)
			return
		}
		n.Rename(entry.Text)
		b.tree.RefreshItem(b.idOf(n))
		b.show(n)
	}, b.window)
}

func (b *Browser) delete(n *node) {
	if n == nil || n.IsRoot() {
		return
	}
	path := b.pathOf(n)
	message := fmt.Sprintf("Permanently delete %s (%s)?\nThis cannot be undone.", path, humanSize(b.totals[n]))
	dialog.ShowConfirm("Delete", message, func(confirmed bool) {
		if !confirmed {
			return
		}
		if err := os.RemoveAll(path); err != nil {
			dialog.ShowError(err, b.window)
			return
		}
		removed := b.totals[n]
		for p := n.parent; p != nil; p = p.parent {
			b.totals[p] -= removed
		}
		parent := n.RemoveNode()
		b.tree.Refresh()
		b.selectNode(parent)
		b.show(parent)
		b.setStatus("Deleted " + path)
	}, b.window)
}

func (b *Browser) toggleTheme() {
	if b.variant == fyneTheme.VariantDark {
		b.variant = fyneTheme.VariantLight
	} else {
		b.variant = fyneTheme.VariantDark
	}
	fyne.CurrentApp().Settings().SetTheme(&appTheme{variant: b.variant})
}

// Helpers

// pathOf returns n's real location on disk. node.GetAbsolutePath can't be used
// because the scanner only stores the root's base name, not its full path.
func (b *Browser) pathOf(n *node) string {
	var parts []string
	for current := n; current != nil && !current.IsRoot(); current = current.parent {
		parts = append([]string{current.name}, parts...)
	}
	return filepath.Join(append([]string{b.rootPath}, parts...)...)
}

func (b *Browser) setStatus(text string) {
	b.status.SetText(text)
}

func setEnabled(button *widget.Button, enabled bool) {
	if enabled {
		button.Enable()
	} else {
		button.Disable()
	}
}

func iconFor(n *node) fyne.Resource {
	switch {
	case n.err != nil:
		return fyneTheme.WarningIcon()
	case n.isDir:
		return fyneTheme.FolderIcon()
	default:
		return fyneTheme.FileIcon()
	}
}

func countItems(n *node) int {
	count := len(n.children)
	for _, child := range n.children {
		count += countItems(child)
	}
	return count
}

func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	value, exponent := float64(bytes)/unit, 0
	for value >= unit && exponent < 5 {
		value /= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", value, "KMGTPE"[exponent])
}

// appTheme is the default fyne theme pinned to one light/dark variant with a blue accent.
type appTheme struct {
	variant fyne.ThemeVariant
}

var accentColor = color.NRGBA{R: 0, G: 122, B: 204, A: 255}

func (t *appTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	if name == fyneTheme.ColorNamePrimary {
		return accentColor
	}
	return fyneTheme.DefaultTheme().Color(name, t.variant)
}

func (t *appTheme) Font(style fyne.TextStyle) fyne.Resource {
	return fyneTheme.DefaultTheme().Font(style)
}

func (t *appTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return fyneTheme.DefaultTheme().Icon(name)
}

func (t *appTheme) Size(name fyne.ThemeSizeName) float32 {
	return fyneTheme.DefaultTheme().Size(name)
}
