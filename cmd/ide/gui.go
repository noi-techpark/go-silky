// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gdamore/tcell/v2"
	"github.com/noi-techpark/go-apigorowler"
	"github.com/rivo/tview"
	"github.com/sergi/go-diff/diffmatchpatch"
	"gopkg.in/yaml.v3"
)

var debounceTimer *time.Timer
var debounceMutex sync.Mutex

func escapeBrackets(input string) string {
	return strings.NewReplacer(
		"[", "[\u200B",
		"]", "\u200B]",
	).Replace(input)
}

func getEventTypeName(t apigorowler.ProfileEventType) string {
	names := map[apigorowler.ProfileEventType]string{
		apigorowler.EVENT_ROOT_START:            "Root Start",
		apigorowler.EVENT_REQUEST_STEP_START:    "Request Step Start",
		apigorowler.EVENT_REQUEST_STEP_END:      "Request Step End",
		apigorowler.EVENT_CONTEXT_SELECTION:     "Context Selection",
		apigorowler.EVENT_REQUEST_PAGE_START:    "Request Page Start",
		apigorowler.EVENT_REQUEST_PAGE_END:      "Request Page End",
		apigorowler.EVENT_PAGINATION_EVAL:       "Pagination Evaluation",
		apigorowler.EVENT_URL_COMPOSITION:       "URL Composition",
		apigorowler.EVENT_REQUEST_DETAILS:       "Request Details",
		apigorowler.EVENT_REQUEST_RESPONSE:      "Request Response",
		apigorowler.EVENT_RESPONSE_TRANSFORM:    "Response Transform",
		apigorowler.EVENT_CONTEXT_MERGE:         "Context Merge",
		apigorowler.EVENT_FOREACH_STEP_START:    "ForEach Step Start",
		apigorowler.EVENT_FOREACH_STEP_END:      "ForEach Step End",
		apigorowler.EVENT_FORVALUES_STEP_START:  "ForValues Step Start",
		apigorowler.EVENT_FORVALUES_STEP_END:    "ForValues Step End",
		apigorowler.EVENT_PARALLELISM_SETUP:     "Parallelism Setup",
		apigorowler.EVENT_ITEM_SELECTION:        "Item Selection",
		apigorowler.EVENT_AUTH_START:            "Auth Start",
		apigorowler.EVENT_AUTH_CACHED:           "Auth Cached",
		apigorowler.EVENT_AUTH_LOGIN_START:      "Auth Login Start",
		apigorowler.EVENT_AUTH_LOGIN_END:        "Auth Login End",
		apigorowler.EVENT_AUTH_TOKEN_EXTRACT:    "Token Extract",
		apigorowler.EVENT_AUTH_TOKEN_INJECT:     "Token Inject",
		apigorowler.EVENT_AUTH_END:              "Auth End",
		apigorowler.EVENT_RESULT:                "Result",
		apigorowler.EVENT_STREAM_RESULT:         "Stream Result",
		apigorowler.EVENT_ERROR:                 "Error",
	}
	if name, ok := names[t]; ok {
		return name
	}
	return fmt.Sprintf("Unknown (%d)", t)
}

func isContainerEvent(t apigorowler.ProfileEventType) bool {
	return t == apigorowler.EVENT_ROOT_START ||
		t == apigorowler.EVENT_REQUEST_STEP_START ||
		t == apigorowler.EVENT_FOREACH_STEP_START ||
		t == apigorowler.EVENT_FORVALUES_STEP_START ||
		t == apigorowler.EVENT_REQUEST_PAGE_START ||
		t == apigorowler.EVENT_ITEM_SELECTION ||
		t == apigorowler.EVENT_AUTH_START ||
		t == apigorowler.EVENT_AUTH_LOGIN_START
}

func isStartEvent(t apigorowler.ProfileEventType) bool {
	return t == apigorowler.EVENT_ROOT_START ||
		t == apigorowler.EVENT_REQUEST_STEP_START ||
		t == apigorowler.EVENT_FOREACH_STEP_START ||
		t == apigorowler.EVENT_FORVALUES_STEP_START ||
		t == apigorowler.EVENT_REQUEST_PAGE_START ||
		t == apigorowler.EVENT_AUTH_START ||
		t == apigorowler.EVENT_AUTH_LOGIN_START
}

func isEndEvent(t apigorowler.ProfileEventType) bool {
	return t == apigorowler.EVENT_REQUEST_STEP_END ||
		t == apigorowler.EVENT_FOREACH_STEP_END ||
		t == apigorowler.EVENT_FORVALUES_STEP_END ||
		t == apigorowler.EVENT_REQUEST_PAGE_END ||
		t == apigorowler.EVENT_AUTH_END ||
		t == apigorowler.EVENT_AUTH_LOGIN_END
}

// hasContextMapData returns true if the event contains context/template map data
func hasContextMapData(data apigorowler.StepProfilerData) bool {
	switch data.Type {
	case apigorowler.EVENT_URL_COMPOSITION:
		_, ok := data.Data["goTemplateContext"]
		return ok
	case apigorowler.EVENT_CONTEXT_SELECTION, apigorowler.EVENT_CONTEXT_MERGE:
		_, ok := data.Data["fullContextMap"]
		return ok
	}
	return false
}

func getHelpText() string {
	return `[yellow::b]ApiGorowler IDE - Keyboard Shortcuts[-:-:-]

[green::b]Navigation (Vim-style)[-:-:-]
  [yellow]j / ↓[-]         Move to next step in tree
  [yellow]k / ↑[-]         Move to previous step in tree
  [yellow]h / ←[-]         Collapse node OR go to parent
  [yellow]l / →[-]         Expand node
  [yellow]n[-]             Next sibling (same level)
  [yellow]N[-]             Previous sibling (same level)
  [yellow]p[-]             Jump to parent step
  [yellow]g[-]             Jump to first step
  [yellow]G[-]             Jump to last step
  [yellow]Enter/Space[-]   Toggle expand/collapse

[green::b]Tree Operations[-:-:-]
  [yellow]e[-]             Expand all steps
  [yellow]c[-]             Collapse all steps
  [yellow]E[-]             Expand current subtree
  [yellow]C[-]             Collapse current subtree

[green::b]Diff View (Transform/Merge)[-:-:-]
  [yellow]1[-]             Show "Before" data
  [yellow]2[-]             Show "After" data
  [yellow]3[-]             Show "Diff" (word-based)

[green::b]Context Map View[-:-:-]
  [yellow]m[-]             Open context/template map viewer (when available)
  [yellow]/[-]             Search in context map (when viewer open)
  [yellow]n[-]             Next search match
  [yellow]N[-]             Previous search match
  [yellow]Esc[-]           Close context map viewer

[green::b]View Controls[-:-:-]
  [yellow]Tab[-]           Switch focus between panels
  [yellow]?[-]             Toggle this help panel
  [yellow]d[-]             Dump steps to /out folder
  [yellow]r[-]             Refresh/restart crawler

[green::b]Execution[-:-:-]
  [yellow]s[-]             Stop current execution
  [yellow]Ctrl+C[-]        Stop execution & exit

[green::b]Scrolling (when focused on Log/Details)[-:-:-]
  [yellow]↑/↓[-]           Scroll line by line
  [yellow]PgUp/PgDn[-]     Scroll page by page
  [yellow]Home/End[-]      Jump to top/bottom

[yellow]Press [::b]?[-:-:-] or [::b]Esc[-:-:-] to close this help`
}

// Generate a tview-color-tagged diff from before/after strings
func getColoredDiff(before, after string) string {
	// If there's no previous content, treat all as new
	if before == "" {
		return escapeBrackets(after)
	}

	dmp := diffmatchpatch.New()
	diffs := dmp.DiffMain(before, after, false)

	var result strings.Builder
	for _, d := range diffs {
		switch d.Type {
		case diffmatchpatch.DiffInsert:
			// Green background for additions
			result.WriteString(`[black:green]` + escapeBrackets(d.Text) + `[-:-:-]`)
		case diffmatchpatch.DiffDelete:
			// Red background for deletions
			result.WriteString(`[white:red]` + escapeBrackets(d.Text) + `[-:-:-]`)
		case diffmatchpatch.DiffEqual:
			// Default formatting for unchanged
			result.WriteString(escapeBrackets(d.Text))
		}
	}
	return result.String()
}

type ConsoleLogger struct {
	LogFunc func(msg string)
}

func (cl ConsoleLogger) Info(msg string, args ...any) {
	escaped := escapeBrackets(fmt.Sprintf(msg, args...))
	cl.LogFunc("[INFO] " + escaped)
}

func (cl ConsoleLogger) Debug(msg string, args ...any) {
	escaped := escapeBrackets(fmt.Sprintf(msg, args...))
	cl.LogFunc("[#bdc9c4] " + escaped)
}

func (cl ConsoleLogger) Warning(msg string, args ...any) {
	escaped := escapeBrackets(fmt.Sprintf(msg, args...))
	cl.LogFunc("[orange] " + escaped)
}

func (cl ConsoleLogger) Error(msg string, args ...any) {
	escaped := escapeBrackets(fmt.Sprintf(msg, args...))
	cl.LogFunc("[red] " + escaped)
}

type ConsoleApp struct {
	app            *tview.Application
	watcher        *fsnotify.Watcher
	selectedStep   int
	mutex          sync.Mutex
	execLog        *tview.TextView
	stepDetails    *tview.TextView
	steps          *tview.TreeView
	statusBar      *tview.TextView
	helpPanel      *tview.TextView
	pages          *tview.Pages
	mainLayout     *tview.Flex
	configFilePath string
	profilerData   []apigorowler.StepProfilerData
	stopFn         context.CancelFunc
	// ID-based hierarchy tracking
	nodeMap        map[string]*tview.TreeNode
	// Diff view state
	currentDiffView string // "before", "after", or "diff"
	currentEventData apigorowler.StepProfilerData
}

func recoverAndLog(logger ConsoleLogger) {
	if r := recover(); r != nil {
		stack := debug.Stack()
		logger.Error("Recovered from panic: %v\nStack Trace:\n%s", r, string(stack))
	}
}

func NewConsoleApp() *ConsoleApp {
	return &ConsoleApp{
		app:          tview.NewApplication(),
		selectedStep: 0,
		profilerData: make([]apigorowler.StepProfilerData, 0),
		nodeMap:      make(map[string]*tview.TreeNode),
	}
}

func (c *ConsoleApp) Run() {
	var inputField *tview.InputField

	inputField = tview.NewInputField().
		SetLabel("Enter path to configuration file: ").
		SetFieldWidth(40).
		SetDoneFunc(func(key tcell.Key) {
			if key == tcell.KeyEnter {
				c.validateAndGotoIDE(inputField)
			}
		})

	form := tview.NewForm().
		AddFormItem(inputField)

	form.SetBorder(true).SetTitle("Configuration Input").SetTitleAlign(tview.AlignLeft)

	c.app.SetRoot(form, true)

	if err := c.app.Run(); err != nil {
		log.Fatal(err)
	}
}

func (c *ConsoleApp) validateAndGotoIDE(inputField *tview.InputField) {
	path := inputField.GetText()
	if _, err := os.Stat(path); err != nil {
		inputField.SetLabel("Invalid path. Enter path to configuration file: ")
		return
	}
	c.configFilePath = path
	c.gotoIDE(path)
	go func() {
		c.onConfigFileChanged()
	}()
}

func (c *ConsoleApp) gotoIDE(path string) {
	var err error
	c.watcher, err = fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}

	err = c.watcher.Add(path)
	if err != nil {
		log.Fatal(err)
	}

	// Create UI components
	c.execLog = tview.NewTextView()
	c.execLog.SetDynamicColors(true)
	c.execLog.SetScrollable(true)
	c.execLog.SetBorder(true)
	c.execLog.SetTitle(" Execution Log ")

	c.stepDetails = tview.NewTextView()
	c.stepDetails.SetDynamicColors(true)
	c.stepDetails.SetScrollable(true)
	c.stepDetails.SetBorder(true)
	c.stepDetails.SetTitle(" Step Details ")
	c.stepDetails.SetWrap(true)

	root := tview.NewTreeNode("Steps").SetSelectable(false)
	c.steps = tview.NewTreeView().SetRoot(root)
	c.steps.SetBorder(true)
	c.steps.SetTitle(" Execution Steps ")

	c.statusBar = tview.NewTextView()
	c.statusBar.SetDynamicColors(true)
	c.updateStatusBar()

	c.helpPanel = tview.NewTextView()
	c.helpPanel.SetDynamicColors(true)
	c.helpPanel.SetBorder(true)
	c.helpPanel.SetTitle(" Help ")
	c.helpPanel.SetText(getHelpText())

	c.app.EnableMouse(true)

	// Log scrolling
	c.execLog.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		x, y := c.execLog.GetScrollOffset()
		switch event.Key() {
		case tcell.KeyUp:
			c.execLog.ScrollTo(x, y-1)
			return nil
		case tcell.KeyDown:
			c.execLog.ScrollTo(x, y+1)
			return nil
		case tcell.KeyPgUp:
			c.execLog.ScrollTo(x, y-10)
			return nil
		case tcell.KeyPgDn:
			c.execLog.ScrollTo(x, y+10)
			return nil
		case tcell.KeyHome:
			c.execLog.ScrollToBeginning()
			return nil
		case tcell.KeyEnd:
			c.execLog.ScrollToEnd()
			return nil
		}
		return event
	})

	// Step details scrolling - let tview handle most scrolling, but ensure proper behavior
	c.stepDetails.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		_, _, _, height := c.stepDetails.GetInnerRect()
		row, _ := c.stepDetails.GetScrollOffset()

		switch event.Key() {
		case tcell.KeyUp:
			if row > 0 {
				c.stepDetails.ScrollTo(0, row-1)
			}
			return nil
		case tcell.KeyDown:
			c.stepDetails.ScrollTo(0, row+1)
			return nil
		case tcell.KeyPgUp:
			newRow := row - height
			if newRow < 0 {
				newRow = 0
			}
			c.stepDetails.ScrollTo(0, newRow)
			return nil
		case tcell.KeyPgDn:
			c.stepDetails.ScrollTo(0, row+height)
			return nil
		case tcell.KeyHome:
			c.stepDetails.ScrollToBeginning()
			return nil
		case tcell.KeyEnd:
			c.stepDetails.ScrollToEnd()
			return nil
		}
		return event
	})

	// Tree navigation and operations
	c.steps.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		node := c.steps.GetCurrentNode()
		if node == nil {
			return event
		}

		switch event.Key() {
		case tcell.KeyRune:
			switch event.Rune() {
			case 'k': // Up
				return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
			case 'j': // Down
				return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
			case 'h': // Left/collapse
				if node.IsExpanded() {
					node.SetExpanded(false)
				} else {
					// Go to parent
					c.goToParentNode()
				}
				return nil
			case 'l': // Right/expand
				if len(node.GetChildren()) > 0 && !node.IsExpanded() {
					node.SetExpanded(true)
				}
				return nil
			case 'g': // Go to first
				c.goToFirstNode()
				return nil
			case 'G': // Go to last
				c.goToLastNode()
				return nil
			case 'p': // Go to parent
				c.goToParentNode()
				return nil
			case 'e': // Expand all
				c.expandAll()
				return nil
			case 'c': // Collapse all
				c.collapseAll()
				return nil
			case 'E': // Expand subtree
				c.expandSubtree(node)
				return nil
			case 'C': // Collapse subtree
				c.collapseSubtree(node)
				return nil
			case 'n': // Next sibling
				c.goToNextSibling()
				return nil
			case 'N': // Previous sibling
				c.goToPrevSibling()
				return nil
			}
		case tcell.KeyLeft:
			if node.IsExpanded() {
				node.SetExpanded(false)
			} else {
				c.goToParentNode()
			}
			return nil
		case tcell.KeyRight:
			if len(node.GetChildren()) > 0 && !node.IsExpanded() {
				node.SetExpanded(true)
			}
			return nil
		}
		return event
	})

	// Global hotkeys with focus tracking
	focusOrder := []tview.Primitive{c.steps, c.stepDetails, c.execLog}

	c.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		// Get currently focused primitive
		focused := c.app.GetFocus()

		switch event.Key() {
		case tcell.KeyTAB:
			// Find current focus index
			currentIndex := 0
			for i, prim := range focusOrder {
				if prim == focused {
					currentIndex = i
					break
				}
			}
			// Move to next
			nextIndex := (currentIndex + 1) % len(focusOrder)
			c.app.SetFocus(focusOrder[nextIndex])
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case '?':
				c.toggleHelp()
				return nil
			case 's':
				c.stopExec()
				return nil
			case 'd':
				c.dumpStepsToLog()
				return nil
			case 'r':
				go c.onConfigFileChanged()
				return nil
			case '1':
				c.currentDiffView = "before"
				c.updateStepDetails(c.steps.GetCurrentNode())
				return nil
			case '2':
				c.currentDiffView = "after"
				c.updateStepDetails(c.steps.GetCurrentNode())
				return nil
			case '3':
				c.currentDiffView = "diff"
				c.updateStepDetails(c.steps.GetCurrentNode())
				return nil
			case 'm':
				if hasContextMapData(c.currentEventData) {
					c.showContextMapForEvent(c.currentEventData)
				}
				return nil
			}
		case tcell.KeyEscape:
			// Close context map if open (let it handle its own close)
			if c.pages.HasPage("contextmap") {
				c.pages.RemovePage("contextmap")
				c.app.SetFocus(c.steps)
				return nil
			}
			// Close help if open
			if c.pages.HasPage("help") {
				c.pages.HidePage("help")
			}
			return nil
		}

		// Don't intercept navigation keys when step details or log have focus
		if focused == c.stepDetails || focused == c.execLog {
			// Let the focused primitive handle navigation keys
			return event
		}

		return event
	})

	c.steps.SetSelectedFunc(func(node *tview.TreeNode) {
		if len(node.GetChildren()) > 0 {
			node.SetExpanded(!node.IsExpanded())
		}
	})

	c.steps.SetChangedFunc(c.updateStepDetails)

	// Create layout
	mainFlex := tview.NewFlex().
		AddItem(c.steps, 0, 1, true).
		AddItem(c.stepDetails, 0, 2, false)

	c.mainLayout = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(c.execLog, 7, 0, false).
		AddItem(mainFlex, 0, 1, true).
		AddItem(c.statusBar, 1, 0, false)

	// Create pages for help overlay
	c.pages = tview.NewPages()
	c.pages.AddPage("main", c.mainLayout, true, true)

	c.app.SetRoot(c.pages, true).SetFocus(c.steps)

	// File watcher
	go func() {
		for {
			select {
			case event := <-c.watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					debounceMutex.Lock()
					if debounceTimer != nil {
						debounceTimer.Stop()
					}
					debounceTimer = time.AfterFunc(300*time.Millisecond, func() {
						c.onConfigFileChanged()
					})
					debounceMutex.Unlock()
				}
			case err := <-c.watcher.Errors:
				log.Println("Watcher error:", err)
			}
		}
	}()
}

// Helper to find parent node in tree structure
func (c *ConsoleApp) findParentNode(targetNode *tview.TreeNode) *tview.TreeNode {
	var findParent func(*tview.TreeNode, *tview.TreeNode) *tview.TreeNode
	findParent = func(current *tview.TreeNode, target *tview.TreeNode) *tview.TreeNode {
		for _, child := range current.GetChildren() {
			if child == target {
				return current
			}
			if found := findParent(child, target); found != nil {
				return found
			}
		}
		return nil
	}
	return findParent(c.steps.GetRoot(), targetNode)
}

// Navigation helpers
func (c *ConsoleApp) goToParentNode() {
	node := c.steps.GetCurrentNode()
	if node == nil {
		return
	}

	// Find parent by traversing tree structure
	parentNode := c.findParentNode(node)
	if parentNode != nil && parentNode != c.steps.GetRoot() {
		c.steps.SetCurrentNode(parentNode)
	}
}

func (c *ConsoleApp) goToNextSibling() {
	node := c.steps.GetCurrentNode()
	if node == nil {
		return
	}

	// Find parent by traversing tree structure
	parentNode := c.findParentNode(node)
	if parentNode == nil {
		return
	}

	// Find current node in parent's children
	children := parentNode.GetChildren()
	for i, child := range children {
		if child == node && i < len(children)-1 {
			// Move to next sibling
			c.steps.SetCurrentNode(children[i+1])
			return
		}
	}
}

func (c *ConsoleApp) goToPrevSibling() {
	node := c.steps.GetCurrentNode()
	if node == nil {
		return
	}

	// Find parent by traversing tree structure
	parentNode := c.findParentNode(node)
	if parentNode == nil {
		return
	}

	// Find current node in parent's children
	children := parentNode.GetChildren()
	for i, child := range children {
		if child == node && i > 0 {
			// Move to previous sibling
			c.steps.SetCurrentNode(children[i-1])
			return
		}
	}
}

func (c *ConsoleApp) goToFirstNode() {
	root := c.steps.GetRoot()
	children := root.GetChildren()
	if len(children) > 0 {
		c.steps.SetCurrentNode(children[0])
	}
}

func (c *ConsoleApp) goToLastNode() {
	// Find last visible node by traversing tree
	var lastNode *tview.TreeNode
	var traverse func(*tview.TreeNode)
	traverse = func(node *tview.TreeNode) {
		lastNode = node
		if node.IsExpanded() {
			children := node.GetChildren()
			for _, child := range children {
				traverse(child)
			}
		}
	}

	root := c.steps.GetRoot()
	for _, child := range root.GetChildren() {
		traverse(child)
	}

	if lastNode != nil {
		c.steps.SetCurrentNode(lastNode)
	}
}

func (c *ConsoleApp) expandAll() {
	root := c.steps.GetRoot()
	var expand func(*tview.TreeNode)
	expand = func(node *tview.TreeNode) {
		node.SetExpanded(true)
		for _, child := range node.GetChildren() {
			expand(child)
		}
	}
	// Expand root's children, not root itself
	for _, child := range root.GetChildren() {
		expand(child)
	}
}

func (c *ConsoleApp) collapseAll() {
	root := c.steps.GetRoot()
	var collapse func(*tview.TreeNode)
	collapse = func(node *tview.TreeNode) {
		node.SetExpanded(false)
		for _, child := range node.GetChildren() {
			collapse(child)
		}
	}
	// Collapse root's children, not root itself (which would hide everything)
	for _, child := range root.GetChildren() {
		collapse(child)
	}
}

func (c *ConsoleApp) expandSubtree(node *tview.TreeNode) {
	var expand func(*tview.TreeNode)
	expand = func(n *tview.TreeNode) {
		n.SetExpanded(true)
		for _, child := range n.GetChildren() {
			expand(child)
		}
	}
	expand(node)
}

func (c *ConsoleApp) collapseSubtree(node *tview.TreeNode) {
	var collapse func(*tview.TreeNode)
	collapse = func(n *tview.TreeNode) {
		n.SetExpanded(false)
		for _, child := range n.GetChildren() {
			collapse(child)
		}
	}
	collapse(node)
}

func (c *ConsoleApp) toggleHelp() {
	if c.pages.HasPage("help") {
		c.pages.RemovePage("help")
	} else {
		// Create modal with help
		modal := tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(tview.NewFlex().
				AddItem(nil, 0, 1, false).
				AddItem(c.helpPanel, 80, 1, true).
				AddItem(nil, 0, 1, false), 0, 8, true).
			AddItem(nil, 0, 1, false)

		c.pages.AddPage("help", modal, true, true)
	}
}

func (c *ConsoleApp) updateStatusBar() {
	// Base status bar text
	statusText := " [yellow]?[-] Help  [yellow]n/N[-] Sibling  [yellow]1/2/3[-] Diff  [yellow]s[-] Stop  [yellow]d[-] Dump  [yellow]r[-] Restart  [yellow]e/c[-] Expand/Collapse"

	// Add context map shortcut if current event has context data
	if hasContextMapData(c.currentEventData) {
		statusText += "  [black:cyan] m [-:-:-] Context Map"
	}

	c.statusBar.SetText(statusText)
}

func (c *ConsoleApp) updateStepDetails(node *tview.TreeNode) {
	ref := node.GetReference()
	if ref == nil {
		c.stepDetails.SetText("")
		c.currentEventData = apigorowler.StepProfilerData{}
		c.updateStatusBar()
		return
	}

	data, ok := ref.(apigorowler.StepProfilerData)
	if !ok {
		c.stepDetails.SetText("")
		c.currentEventData = apigorowler.StepProfilerData{}
		c.updateStatusBar()
		return
	}

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Store current event data for diff toggling and status bar
	c.currentEventData = data

	// Update status bar to show context map shortcut if available
	c.updateStatusBar()

	c.stepDetails.ScrollToBeginning()

	// Build event-specific details
	var detailsText strings.Builder

	// Header
	detailsText.WriteString(fmt.Sprintf("[yellow::b]%s[-:-:-]\n", data.Name))
	detailsText.WriteString(fmt.Sprintf("[#666666]%s[-]\n\n", strings.Repeat("─", 60)))

	// Basic info
	detailsText.WriteString(fmt.Sprintf("[green::b]Type:[-:-:-] %s\n", getEventTypeName(data.Type)))
	detailsText.WriteString(fmt.Sprintf("[green::b]Time:[-:-:-] %s", data.Timestamp.Format("15:04:05.000")))
	if data.Duration > 0 {
		detailsText.WriteString(fmt.Sprintf("  [yellow::b]Duration:[-:-:-] %dms", data.Duration))
	}
	detailsText.WriteString("\n")

	if data.WorkerID != 0 || data.WorkerPool != "" {
		detailsText.WriteString(fmt.Sprintf("[green::b]Worker:[-:-:-] %s (Thread %d)\n", data.WorkerPool, data.WorkerID))
	}
	detailsText.WriteString("\n")

	// Event-specific details
	switch data.Type {
	case apigorowler.EVENT_ROOT_START:
		c.formatRootStart(&detailsText, data)
	case apigorowler.EVENT_REQUEST_STEP_START, apigorowler.EVENT_FOREACH_STEP_START, apigorowler.EVENT_FORVALUES_STEP_START:
		c.formatStepStart(&detailsText, data)
	case apigorowler.EVENT_CONTEXT_SELECTION:
		c.formatContextSelection(&detailsText, data)
	case apigorowler.EVENT_REQUEST_PAGE_START:
		c.formatRequestPage(&detailsText, data)
	case apigorowler.EVENT_PAGINATION_EVAL:
		c.formatPaginationEval(&detailsText, data)
	case apigorowler.EVENT_URL_COMPOSITION:
		c.formatUrlComposition(&detailsText, data)
	case apigorowler.EVENT_REQUEST_DETAILS:
		c.formatRequestDetails(&detailsText, data)
	case apigorowler.EVENT_REQUEST_RESPONSE:
		c.formatRequestResponse(&detailsText, data)
	case apigorowler.EVENT_RESPONSE_TRANSFORM:
		c.formatResponseTransform(&detailsText, data)
	case apigorowler.EVENT_CONTEXT_MERGE:
		c.formatContextMerge(&detailsText, data)
	case apigorowler.EVENT_PARALLELISM_SETUP:
		c.formatParallelismSetup(&detailsText, data)
	case apigorowler.EVENT_ITEM_SELECTION:
		c.formatItemSelection(&detailsText, data)
	case apigorowler.EVENT_AUTH_START, apigorowler.EVENT_AUTH_END:
		c.formatAuthStartEnd(&detailsText, data)
	case apigorowler.EVENT_AUTH_CACHED:
		c.formatAuthCached(&detailsText, data)
	case apigorowler.EVENT_AUTH_LOGIN_START, apigorowler.EVENT_AUTH_LOGIN_END:
		c.formatAuthLogin(&detailsText, data)
	case apigorowler.EVENT_AUTH_TOKEN_EXTRACT:
		c.formatAuthTokenExtract(&detailsText, data)
	case apigorowler.EVENT_AUTH_TOKEN_INJECT:
		c.formatAuthTokenInject(&detailsText, data)
	case apigorowler.EVENT_RESULT, apigorowler.EVENT_STREAM_RESULT:
		c.formatResult(&detailsText, data)
	case apigorowler.EVENT_ERROR:
		c.formatError(&detailsText, data)
	default:
		// Generic data display
		if len(data.Data) > 0 {
			dataJson, _ := json.MarshalIndent(data.Data, "", "  ")
			detailsText.WriteString(fmt.Sprintf("[green::b]Data:[-:-:-]\n%s\n", escapeBrackets(string(dataJson))))
		}
	}

	c.stepDetails.SetText(detailsText.String())
}

// Event-specific formatters
func (c *ConsoleApp) formatRootStart(sb *strings.Builder, data apigorowler.StepProfilerData) {
	sb.WriteString("[cyan::b]Initial Context[-:-:-]\n")
	if ctx, ok := data.Data["contextMap"]; ok {
		jsonStr, _ := json.MarshalIndent(ctx, "", "  ")
		sb.WriteString(fmt.Sprintf("%s\n", escapeBrackets(string(jsonStr))))
	}
}

func (c *ConsoleApp) formatStepStart(sb *strings.Builder, data apigorowler.StepProfilerData) {
	sb.WriteString("[cyan::b]Step Configuration[-:-:-]\n")
	if cfg, ok := data.Data["stepConfig"]; ok {
		jsonStr, _ := json.MarshalIndent(cfg, "", "  ")
		sb.WriteString(fmt.Sprintf("%s\n", escapeBrackets(string(jsonStr))))
	}
}

func (c *ConsoleApp) formatContextSelection(sb *strings.Builder, data apigorowler.StepProfilerData) {
	if path, ok := data.Data["contextPath"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Context Path:[-:-:-] %s\n\n", path))
	}
	if ctx, ok := data.Data["currentContextData"]; ok {
		sb.WriteString("[cyan::b]Current Context Data[-:-:-]\n")
		jsonStr, _ := json.MarshalIndent(ctx, "", "  ")
		sb.WriteString(fmt.Sprintf("%s\n\n", escapeBrackets(string(jsonStr))))
	}

	// Show hint for full context map modal
	if _, ok := data.Data["fullContextMap"]; ok {
		sb.WriteString("[#888888]Press [yellow::b]m[-:-:-][#888888] to open full context map viewer[-]\n")
	}
}

func (c *ConsoleApp) formatRequestPage(sb *strings.Builder, data apigorowler.StepProfilerData) {
	if pageNum, ok := data.Data["pageNumber"]; ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Page Number:[-:-:-] %v\n", pageNum))
	}
}

func (c *ConsoleApp) formatPaginationEval(sb *strings.Builder, data apigorowler.StepProfilerData) {
	sb.WriteString("[cyan::b]Pagination State[-:-:-]\n\n")

	if prevState, ok := data.Data["previousState"].(map[string]any); ok {
		sb.WriteString("[yellow]Before:[-]\n")
		jsonStr, _ := json.MarshalIndent(prevState, "", "  ")
		sb.WriteString(fmt.Sprintf("%s\n\n", escapeBrackets(string(jsonStr))))
	}

	if afterState, ok := data.Data["afterState"].(map[string]any); ok {
		sb.WriteString("[green]After:[-]\n")
		jsonStr, _ := json.MarshalIndent(afterState, "", "  ")
		sb.WriteString(fmt.Sprintf("%s\n", escapeBrackets(string(jsonStr))))
	}
}

func (c *ConsoleApp) formatUrlComposition(sb *strings.Builder, data apigorowler.StepProfilerData) {
	if template, ok := data.Data["urlTemplate"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]URL Template:[-:-:-]\n%s\n\n", template))
	}
	if resultUrl, ok := data.Data["resultUrl"].(string); ok {
		sb.WriteString(fmt.Sprintf("[green::b]Result URL:[-:-:-]\n%s\n\n", resultUrl))
	}

	// Show hint for template context modal
	if _, ok := data.Data["goTemplateContext"]; ok {
		sb.WriteString("[#888888]Press [yellow::b]m[-:-:-][#888888] to open template context viewer[-]\n")
	}
}

func (c *ConsoleApp) formatRequestDetails(sb *strings.Builder, data apigorowler.StepProfilerData) {
	if curl, ok := data.Data["curl"].(string); ok {
		sb.WriteString("[cyan::b]cURL Command:[-:-:-]\n")
		sb.WriteString(fmt.Sprintf("[#888888]%s[-]\n\n", escapeBrackets(curl)))
	}

	if method, ok := data.Data["method"].(string); ok {
		sb.WriteString(fmt.Sprintf("[green::b]Method:[-:-:-] %s\n", method))
	}
	if url, ok := data.Data["url"].(string); ok {
		sb.WriteString(fmt.Sprintf("[green::b]URL:[-:-:-] %s\n\n", url))
	}

	if headers, ok := data.Data["headers"].(map[string]any); ok && len(headers) > 0 {
		sb.WriteString("[cyan::b]Headers:[-:-:-]\n")
		jsonStr, _ := json.MarshalIndent(headers, "", "  ")
		sb.WriteString(fmt.Sprintf("%s\n", escapeBrackets(string(jsonStr))))
	}
}

func (c *ConsoleApp) formatRequestResponse(sb *strings.Builder, data apigorowler.StepProfilerData) {
	if status, ok := data.Data["statusCode"]; ok {
		var statusCode int
		switch v := status.(type) {
		case int:
			statusCode = v
		case float64:
			statusCode = int(v)
		case int64:
			statusCode = int(v)
		default:
			statusCode = 0
		}

		color := "green"
		if statusCode >= 400 {
			color = "red"
		} else if statusCode >= 300 {
			color = "yellow"
		}
		sb.WriteString(fmt.Sprintf("[%s::b]Status Code:[-:-:-] %d\n\n", color, statusCode))
	}

	if body, ok := data.Data["body"]; ok {
		sb.WriteString("[cyan::b]Response Body:[-:-:-]\n")
		jsonStr, _ := json.MarshalIndent(body, "", "  ")
		// Truncate very large responses
		bodyStr := string(jsonStr)
		if len(bodyStr) > 5000 {
			bodyStr = bodyStr[:5000] + "\n\n[yellow]... (truncated, too large to display)[-]"
		}
		sb.WriteString(fmt.Sprintf("%s\n", escapeBrackets(bodyStr)))
	}
}

func (c *ConsoleApp) formatResponseTransform(sb *strings.Builder, data apigorowler.StepProfilerData) {
	if rule, ok := data.Data["transformRule"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Transform Rule:[-:-:-]\n[yellow]%s[-]\n\n", rule))
	}

	before, hasBefore := data.Data["beforeResponse"]
	after, hasAfter := data.Data["afterResponse"]

	if !hasBefore || !hasAfter {
		return
	}

	// Show view toggle buttons
	view := c.currentDiffView
	if view == "" {
		view = "diff"
		c.currentDiffView = "diff"
	}

	btn1 := "[ 1:Before ]"
	btn2 := "[ 2:After ]"
	btn3 := "[ 3:Diff ]"

	if view == "before" {
		btn1 = "[black:green][ 1:Before ][-:-:-]"
	} else if view == "after" {
		btn2 = "[black:green][ 2:After ][-:-:-]"
	} else {
		btn3 = "[black:green][ 3:Diff ][-:-:-]"
	}

	sb.WriteString(fmt.Sprintf("%s %s %s\n\n", btn1, btn2, btn3))

	// Display based on current view
	switch view {
	case "before":
		sb.WriteString("[cyan::b]Before Transform:[-:-:-]\n")
		jsonStr, _ := json.MarshalIndent(before, "", "  ")
		sb.WriteString(fmt.Sprintf("%s\n", escapeBrackets(string(jsonStr))))

	case "after":
		sb.WriteString("[cyan::b]After Transform:[-:-:-]\n")
		jsonStr, _ := json.MarshalIndent(after, "", "  ")
		sb.WriteString(fmt.Sprintf("%s\n", escapeBrackets(string(jsonStr))))

	case "diff":
		sb.WriteString("[cyan::b]Diff (word-based):[-:-:-]\n")
		beforeStr, _ := json.MarshalIndent(before, "", "  ")
		afterStr, _ := json.MarshalIndent(after, "", "  ")
		diff := getColoredDiff(string(beforeStr), string(afterStr))
		sb.WriteString(diff)
		sb.WriteString("\n")
	}
}

func (c *ConsoleApp) formatContextMerge(sb *strings.Builder, data apigorowler.StepProfilerData) {
	if rule, ok := data.Data["mergeRule"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Merge Rule:[-:-:-]\n[yellow]%s[-]\n\n", rule))
	}

	if currentKey, ok := data.Data["currentContextKey"].(string); ok {
		sb.WriteString(fmt.Sprintf("[green::b]Current Context:[-:-:-] %s\n", currentKey))
	}
	if targetKey, ok := data.Data["targetContextKey"].(string); ok {
		sb.WriteString(fmt.Sprintf("[green::b]Target Context:[-:-:-] %s\n\n", targetKey))
	}

	before, hasBefore := data.Data["targetContextBefore"]
	after, hasAfter := data.Data["targetContextAfter"]

	if !hasBefore || !hasAfter {
		return
	}

	// Show view toggle buttons
	view := c.currentDiffView
	if view == "" {
		view = "diff"
		c.currentDiffView = "diff"
	}

	btn1 := "[ 1:Before ]"
	btn2 := "[ 2:After ]"
	btn3 := "[ 3:Diff ]"

	if view == "before" {
		btn1 = "[black:green][ 1:Before ][-:-:-]"
	} else if view == "after" {
		btn2 = "[black:green][ 2:After ][-:-:-]"
	} else {
		btn3 = "[black:green][ 3:Diff ][-:-:-]"
	}

	sb.WriteString(fmt.Sprintf("%s %s %s\n\n", btn1, btn2, btn3))

	// Display based on current view
	switch view {
	case "before":
		sb.WriteString("[cyan::b]Target Before Merge:[-:-:-]\n")
		jsonStr, _ := json.MarshalIndent(before, "", "  ")
		sb.WriteString(fmt.Sprintf("%s\n", escapeBrackets(string(jsonStr))))

	case "after":
		sb.WriteString("[cyan::b]Target After Merge:[-:-:-]\n")
		jsonStr, _ := json.MarshalIndent(after, "", "  ")
		sb.WriteString(fmt.Sprintf("%s\n", escapeBrackets(string(jsonStr))))

	case "diff":
		sb.WriteString("[cyan::b]Diff (word-based):[-:-:-]\n")
		beforeStr, _ := json.MarshalIndent(before, "", "  ")
		afterStr, _ := json.MarshalIndent(after, "", "  ")
		diff := getColoredDiff(string(beforeStr), string(afterStr))
		sb.WriteString(diff)
		sb.WriteString("\n")
	}

	// Show hint for full context map modal
	if _, ok := data.Data["fullContextMap"]; ok {
		sb.WriteString("\n[#888888]Press [yellow::b]m[-:-:-][#888888] to open full context map viewer[-]\n")
	}
}

func (c *ConsoleApp) formatParallelismSetup(sb *strings.Builder, data apigorowler.StepProfilerData) {
	if workers, ok := data.Data["maxConcurrency"]; ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Max Concurrency:[-:-:-] %v\n", workers))
	}
	if rateLimit, ok := data.Data["rateLimit"]; ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Rate Limit:[-:-:-] %v req/s\n", rateLimit))
	}
	if burst, ok := data.Data["burst"]; ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Burst:[-:-:-] %v\n", burst))
	}
	if poolId, ok := data.Data["workerPoolId"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Worker Pool ID:[-:-:-] %s\n", poolId))
	}
}

func (c *ConsoleApp) formatItemSelection(sb *strings.Builder, data apigorowler.StepProfilerData) {
	if idx, ok := data.Data["iterationIndex"]; ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Iteration Index:[-:-:-] %v\n\n", idx))
	}

	if item, ok := data.Data["itemValue"]; ok {
		sb.WriteString("[cyan::b]Item Value:[-:-:-]\n")
		jsonStr, _ := json.MarshalIndent(item, "", "  ")
		sb.WriteString(fmt.Sprintf("%s\n", escapeBrackets(string(jsonStr))))
	}
}

func (c *ConsoleApp) formatResult(sb *strings.Builder, data apigorowler.StepProfilerData) {
	var result interface{}
	if r, ok := data.Data["result"]; ok {
		result = r
	} else if e, ok := data.Data["entity"]; ok {
		result = e
		if idx, ok := data.Data["index"]; ok {
			sb.WriteString(fmt.Sprintf("[cyan::b]Stream Index:[-:-:-] %v\n\n", idx))
		}
	}

	if result != nil {
		sb.WriteString("[cyan::b]Result:[-:-:-]\n")
		jsonStr, _ := json.MarshalIndent(result, "", "  ")
		sb.WriteString(fmt.Sprintf("%s\n", escapeBrackets(string(jsonStr))))
	}
}

func (c *ConsoleApp) formatAuthStartEnd(sb *strings.Builder, data apigorowler.StepProfilerData) {
	if authType, ok := data.Data["authType"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Authentication Type:[-:-:-] %s\n\n", authType))
	}

	if err, ok := data.Data["error"].(string); ok {
		sb.WriteString(fmt.Sprintf("[red::b]Error:[-:-:-]\n%s\n\n", escapeBrackets(err)))
	}

	if len(data.Data) > 0 {
		sb.WriteString("[cyan::b]Auth Data:[-:-:-]\n")
		jsonStr, _ := json.MarshalIndent(data.Data, "", "  ")
		sb.WriteString(fmt.Sprintf("%s\n", escapeBrackets(string(jsonStr))))
	}
}

func (c *ConsoleApp) formatAuthCached(sb *strings.Builder, data apigorowler.StepProfilerData) {
	sb.WriteString("[green::b]Using cached credentials[-:-:-]\n\n")

	if age, ok := data.Data["age"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Cache Age:[-:-:-] %s\n", age))
	}

	if token, ok := data.Data["token"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Token (Masked):[-:-:-] %s\n", escapeBrackets(token)))
	}

	if cookieName, ok := data.Data["cookieName"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Cookie Name:[-:-:-] %s\n", escapeBrackets(cookieName)))
	}

	if cookieValue, ok := data.Data["cookieValue"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Cookie Value (Masked):[-:-:-] %s\n", escapeBrackets(cookieValue)))
	}
}

func (c *ConsoleApp) formatAuthLogin(sb *strings.Builder, data apigorowler.StepProfilerData) {
	if url, ok := data.Data["url"].(string); ok {
		if method, ok := data.Data["method"].(string); ok {
			sb.WriteString(fmt.Sprintf("[cyan::b]Login URL:[-:-:-] %s %s\n\n", method, escapeBrackets(url)))
		} else {
			sb.WriteString(fmt.Sprintf("[cyan::b]Login URL:[-:-:-] %s\n\n", escapeBrackets(url)))
		}
	}

	if status, ok := data.Data["statusCode"]; ok {
		var statusCode int
		switch v := status.(type) {
		case int:
			statusCode = v
		case float64:
			statusCode = int(v)
		case int64:
			statusCode = int(v)
		default:
			statusCode = 0
		}

		color := "green"
		if statusCode >= 400 {
			color = "red"
		} else if statusCode >= 300 {
			color = "yellow"
		}
		sb.WriteString(fmt.Sprintf("[%s::b]Status Code:[-:-:-] %d\n\n", color, statusCode))
	}

	if err, ok := data.Data["error"].(string); ok {
		sb.WriteString(fmt.Sprintf("[red::b]Error:[-:-:-]\n%s\n\n", escapeBrackets(err)))
	}

	if token, ok := data.Data["token"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Token (Masked):[-:-:-] %s\n", escapeBrackets(token)))
	}

	if len(data.Data) > 0 {
		sb.WriteString("\n[cyan::b]Full Login Data:[-:-:-]\n")
		jsonStr, _ := json.MarshalIndent(data.Data, "", "  ")
		sb.WriteString(fmt.Sprintf("%s\n", escapeBrackets(string(jsonStr))))
	}
}

func (c *ConsoleApp) formatAuthTokenExtract(sb *strings.Builder, data apigorowler.StepProfilerData) {
	if extractFrom, ok := data.Data["extractFrom"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Extract From:[-:-:-] %s\n", extractFrom))
	} else if selector, ok := data.Data["extractSelector"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Extract Selector:[-:-:-] %s\n", selector))
	}

	if cookieName, ok := data.Data["cookieName"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Cookie Name:[-:-:-] %s\n", escapeBrackets(cookieName)))
	}

	if cookieValue, ok := data.Data["cookieValue"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Cookie Value (Masked):[-:-:-] %s\n", escapeBrackets(cookieValue)))
	}

	if headerName, ok := data.Data["headerName"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Header Name:[-:-:-] %s\n", escapeBrackets(headerName)))
	}

	if jqSelector, ok := data.Data["jqSelector"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]JQ Selector:[-:-:-] %s\n", escapeBrackets(jqSelector)))
	}

	if token, ok := data.Data["token"].(string); ok {
		sb.WriteString(fmt.Sprintf("[green::b]Extracted Token (Masked):[-:-:-] %s\n", escapeBrackets(token)))
	}
}

func (c *ConsoleApp) formatAuthTokenInject(sb *strings.Builder, data apigorowler.StepProfilerData) {
	if location, ok := data.Data["location"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Injection Location:[-:-:-] %s\n", location))
	}

	if format, ok := data.Data["format"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Format:[-:-:-] %s\n", format))
	}

	if token, ok := data.Data["token"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Token (Masked):[-:-:-] %s\n", escapeBrackets(token)))
	}

	if headerKey, ok := data.Data["headerKey"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Header Key:[-:-:-] %s\n", escapeBrackets(headerKey)))
	}

	if queryKey, ok := data.Data["queryKey"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Query Parameter Key:[-:-:-] %s\n", escapeBrackets(queryKey)))
	}

	if cookieName, ok := data.Data["cookieName"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Cookie Name:[-:-:-] %s\n", escapeBrackets(cookieName)))
	}

	if cookieValue, ok := data.Data["cookieValue"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Cookie Value (Masked):[-:-:-] %s\n", escapeBrackets(cookieValue)))
	}
}

func (c *ConsoleApp) formatError(sb *strings.Builder, data apigorowler.StepProfilerData) {
	// Try multiple keys for error message
	var errorMsg string
	if err, ok := data.Data["error"].(string); ok {
		errorMsg = err
	} else if msg, ok := data.Data["message"].(string); ok {
		errorMsg = msg
	} else {
		errorMsg = "Unknown error"
	}

	sb.WriteString(fmt.Sprintf("[red::b]Error Message:[-:-:-]\n%s\n\n", escapeBrackets(errorMsg)))

	if errorType, ok := data.Data["errorType"].(string); ok {
		sb.WriteString(fmt.Sprintf("[yellow::b]Error Type:[-:-:-] %s\n", errorType))
	} else if errType, ok := data.Data["type"].(string); ok {
		sb.WriteString(fmt.Sprintf("[yellow::b]Error Type:[-:-:-] %s\n", errType))
	}

	if stepName, ok := data.Data["stepName"].(string); ok {
		sb.WriteString(fmt.Sprintf("[cyan::b]Step:[-:-:-] %s\n", escapeBrackets(stepName)))
	}

	if details, ok := data.Data["details"].(string); ok {
		sb.WriteString(fmt.Sprintf("[yellow::b]Details:[-:-:-]\n%s\n", escapeBrackets(details)))
	}

	if stackTrace, ok := data.Data["stackTrace"].(string); ok {
		sb.WriteString(fmt.Sprintf("\n[yellow::b]Stack Trace:[-:-:-]\n%s\n", escapeBrackets(stackTrace)))
	} else if stack, ok := data.Data["stack"].(string); ok {
		sb.WriteString(fmt.Sprintf("\n[yellow::b]Stack Trace:[-:-:-]\n%s\n", escapeBrackets(stack)))
	}

	if len(data.Data) > 0 {
		sb.WriteString("\n[cyan::b]Full Error Data:[-:-:-]\n")
		jsonStr, _ := json.MarshalIndent(data.Data, "", "  ")
		sb.WriteString(fmt.Sprintf("%s\n", escapeBrackets(string(jsonStr))))
	}
}

func (c *ConsoleApp) appendLog(log string) {
	c.app.QueueUpdateDraw(func() {
		old := c.execLog.GetText(false)
		newLog := old
		if len(newLog) != 0 {
			newLog += "\n"
		}
		newLog += log

		c.execLog.SetText(newLog)
		c.execLog.ScrollToEnd()
	})
}

func (c *ConsoleApp) setupCrawlJob() {
	c.profilerData = make([]apigorowler.StepProfilerData, 0)
	c.nodeMap = make(map[string]*tview.TreeNode)
	if c.stopFn != nil {
		c.stopFn()
	}

	go func() {
		logger := ConsoleLogger{
			LogFunc: func(msg string) {
				c.appendLog(msg)
			},
		}
		defer recoverAndLog(logger)

		// accumulator for stream data
		streamedData := make([]interface{}, 0)

		craw, _, _ := apigorowler.NewApiCrawler(c.configFilePath)
		craw.SetLogger(logger)
		profiler := craw.EnableProfiler()
		defer close(profiler)

		// Track START events to update with duration later
		startEvents := make(map[string]int) // map event ID to profilerData index

		go func() {
			root := c.steps.GetRoot()

			for d := range profiler {
				c.profilerData = append(c.profilerData, d)
				dataIndex := len(c.profilerData) - 1

				// Create node label with duration for END events
				label := d.Name
				if d.Duration > 0 {
					label = fmt.Sprintf("%s (%dms)", d.Name, d.Duration)
				}

				// Determine if this is a container event
				isContainer := isContainerEvent(d.Type)

				// Create the tree node
				node := tview.NewTreeNode(label).
					SetReference(d).
					SetSelectable(true)

				// Store in nodeMap
				c.nodeMap[d.ID] = node

				// Store START events to update them with duration later
				if isStartEvent(d.Type) {
					startEvents[d.ID] = dataIndex
				}

				// Update START event with duration if this is an END event
				if isEndEvent(d.Type) {
					// Find matching START event (same ID)
					if startIdx, ok := startEvents[d.ID]; ok {
						startData := c.profilerData[startIdx]
						startData.Duration = d.Duration
						c.profilerData[startIdx] = startData

						// Update node label with duration
						if startNode, exists := c.nodeMap[d.ID]; exists {
							startNode.SetText(fmt.Sprintf("%s (%dms)", startData.Name, d.Duration))
						}
						delete(startEvents, d.ID)
					}
					continue // Don't add END events as separate nodes
				}

				// Find parent node
				var parentNode *tview.TreeNode
				if d.ParentID == "" {
					parentNode = root
				} else {
					parentNode = c.nodeMap[d.ParentID]
					if parentNode == nil {
						// Parent not found, add to root
						parentNode = root
					}
				}

				// Add node to parent
				c.app.QueueUpdateDraw(func() {
					parentNode.AddChild(node)
					if isContainer {
						parentNode.SetExpanded(true)
					}
					c.steps.SetCurrentNode(node)
					c.updateStepDetails(node)
				})
			}
		}()

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		c.stopFn = cancel

		// handle stream
		if craw.Config.Stream {
			stream := craw.GetDataStream()
			go func() {
				for d := range stream {
					streamedData = append(streamedData, d)
				}
			}()
		}

		err := craw.Run(ctx)

		if err != nil {
			c.appendLog("[red]" + escapeBrackets(err.Error()))
		} else {
			if craw.Config.Stream {
				close(craw.GetDataStream())
			}
			c.appendLog("[green]Crawler run completed successfully")
		}
	}()
}

func (c *ConsoleApp) onConfigFileChanged() {
	c.stepDetails.SetText("")
	c.steps.GetRoot().ClearChildren()
	c.nodeMap = make(map[string]*tview.TreeNode)

	data, err := os.ReadFile(c.configFilePath)
	if err != nil {
		log.Printf("Error reading config file: %v", err)
		c.app.QueueUpdateDraw(func() {
			c.execLog.SetText(fmt.Sprintf("[red]Error reading config file: %v", err))
		})
		return
	}

	var cfg apigorowler.Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		c.appendLog("[red]" + escapeBrackets(err.Error()))
		return
	}

	errors := apigorowler.ValidateConfig(cfg)
	if len(errors) != 0 {
		text := "[red]"
		for _, r := range errors {
			text += r.Error() + "\n"
		}

		c.appendLog(text)
		return
	}

	// If config valid, update UI or state here, also inside QueueUpdateDraw()
	c.appendLog("[green]Config validated successfully")
	c.setupCrawlJob()
}

func (c *ConsoleApp) dumpStepsToLog() {
	const dumpDir = "out"

	// Ensure output directory exists
	err := os.MkdirAll(dumpDir, 0755)
	if err != nil {
		c.appendLog(fmt.Sprintf("[red]Failed to create output directory: %v", err))
		return
	}

	// Clear existing files
	files, err := os.ReadDir(dumpDir)
	if err != nil {
		c.appendLog(fmt.Sprintf("[red]Failed to read output directory: %v", err))
		return
	}
	for _, file := range files {
		_ = os.Remove(filepath.Join(dumpDir, file.Name()))
	}

	// Lock profiler data
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// Helper to sanitize filenames
	sanitizeFilename := func(name string) string {
		name = strings.ReplaceAll(name, " ", "_")
		invalidChars := regexp.MustCompile(`[\\/:*?"<>|]`)
		return invalidChars.ReplaceAllString(name, "")
	}

	// Recursive traversal of tree
	var index int
	var traverse func(node *tview.TreeNode, depth int)

	traverse = func(node *tview.TreeNode, depth int) {
		// Only process nodes with data references
		if ref := node.GetReference(); ref != nil {
			if step, ok := ref.(apigorowler.StepProfilerData); ok {
				// Apply indentation
				prefix := strings.Repeat("__", depth)
				prefixedName := prefix + step.Name

				// Marshal step
				b, err := json.MarshalIndent(step, "", "  ")
				if err != nil {
					c.appendLog(fmt.Sprintf("[red]Failed to marshal step %d: %v", index, err))
				} else {
					escapedName := sanitizeFilename(prefixedName)
					filename := filepath.Join(dumpDir, fmt.Sprintf("%03d_%s.json", index, escapedName))
					err = os.WriteFile(filename, b, 0644)
					if err != nil {
						c.appendLog(fmt.Sprintf("[red]Failed to write step %d: %v", index, err))
					}
				}
				index++
			}
		}

		// Traverse children
		for _, child := range node.GetChildren() {
			traverse(child, depth+1)
		}
	}

	// Start traversal from root
	root := c.steps.GetRoot()
	traverse(root, 0)

	// Log result
	go func() {
		c.appendLog(fmt.Sprintf("[green]Dumped %d steps to folder '%s'", index, dumpDir))
	}()
}

func (c *ConsoleApp) stopExec() {
	if nil == c.stopFn {
		return
	}

	c.stopFn()

	go func() {
		c.appendLog("[orange]Execution stopped")
	}()
}

func main() {
	app := NewConsoleApp()
	app.Run()
}
