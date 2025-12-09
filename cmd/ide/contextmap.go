// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	silky "github.com/noi-techpark/go-silky"
	"github.com/rivo/tview"
)

// ContextMapViewer is a modal viewer for context/template maps with search functionality
type ContextMapViewer struct {
	app     *tview.Application
	pages   *tview.Pages
	onClose func()

	// UI components
	contentView *tview.TextView
	searchInput *tview.InputField
	statusBar   *tview.TextView
	modal       *tview.Flex

	// Content
	rawContent string
	lines      []string

	// Search state
	searchTerm    string
	searchMatches []int // line indices with matches
	currentMatch  int
}

// NewContextMapViewer creates a new context map viewer
func NewContextMapViewer(app *tview.Application, pages *tview.Pages, onClose func()) *ContextMapViewer {
	v := &ContextMapViewer{
		app:           app,
		pages:         pages,
		onClose:       onClose,
		searchMatches: make([]int, 0),
	}
	v.build()
	return v
}

func (v *ContextMapViewer) build() {
	// Content view
	v.contentView = tview.NewTextView()
	v.contentView.SetDynamicColors(true)
	v.contentView.SetScrollable(true)
	v.contentView.SetWrap(false)
	v.contentView.SetBorder(true)

	// Search input
	v.searchInput = tview.NewInputField()
	v.searchInput.SetLabel(" / ")
	v.searchInput.SetFieldWidth(0)
	v.searchInput.SetFieldBackgroundColor(tcell.ColorDarkSlateGray)

	// Status bar
	v.statusBar = tview.NewTextView()
	v.statusBar.SetDynamicColors(true)
	v.statusBar.SetText(" [yellow]/[-] Search  [yellow]n/N[-] Next/Prev  [yellow]j/k[-] Scroll  [yellow]g/G[-] Top/Bottom  [yellow]Esc[-] Close")

	// Layout
	modalContent := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(v.contentView, 0, 1, true).
		AddItem(v.searchInput, 1, 0, false).
		AddItem(v.statusBar, 1, 0, false)

	// Modal wrapper with padding
	v.modal = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 1, 0, false).
		AddItem(tview.NewFlex().
			AddItem(nil, 2, 0, false).
			AddItem(modalContent, 0, 1, true).
			AddItem(nil, 2, 0, false), 0, 1, true).
		AddItem(nil, 1, 0, false)

	// Set up input handlers
	v.setupContentViewInput()
	v.setupSearchInputHandler()
}

func (v *ContextMapViewer) setupContentViewInput() {
	v.contentView.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyEscape:
			v.Close()
			return nil
		case tcell.KeyUp:
			v.scrollUp(1)
			return nil
		case tcell.KeyDown:
			v.scrollDown(1)
			return nil
		case tcell.KeyPgUp:
			_, _, _, height := v.contentView.GetInnerRect()
			v.scrollUp(height)
			return nil
		case tcell.KeyPgDn:
			_, _, _, height := v.contentView.GetInnerRect()
			v.scrollDown(height)
			return nil
		case tcell.KeyHome:
			v.contentView.ScrollToBeginning()
			return nil
		case tcell.KeyEnd:
			v.contentView.ScrollToEnd()
			return nil
		case tcell.KeyRune:
			switch event.Rune() {
			case '/':
				v.app.SetFocus(v.searchInput)
				return nil
			case 'n':
				v.nextMatch()
				return nil
			case 'N':
				v.prevMatch()
				return nil
			case 'j':
				v.scrollDown(1)
				return nil
			case 'k':
				v.scrollUp(1)
				return nil
			case 'g':
				v.contentView.ScrollToBeginning()
				return nil
			case 'G':
				v.contentView.ScrollToEnd()
				return nil
			case 'q':
				v.Close()
				return nil
			}
		}
		return event
	})
}

func (v *ContextMapViewer) setupSearchInputHandler() {
	v.searchInput.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			term := v.searchInput.GetText()
			if term != v.searchTerm {
				v.searchTerm = term
				v.currentMatch = 0
				v.findMatches()
			}
			v.highlightAndDisplay()
			v.app.SetFocus(v.contentView)
		case tcell.KeyEscape:
			v.searchInput.SetText("")
			v.searchTerm = ""
			v.searchMatches = nil
			v.displayContent()
			v.app.SetFocus(v.contentView)
		}
	})

	// Also handle Escape while typing
	v.searchInput.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			v.searchInput.SetText("")
			v.searchTerm = ""
			v.searchMatches = nil
			v.displayContent()
			v.app.SetFocus(v.contentView)
			return nil
		}
		return event
	})
}

func (v *ContextMapViewer) scrollUp(lines int) {
	row, col := v.contentView.GetScrollOffset()
	newRow := row - lines
	if newRow < 0 {
		newRow = 0
	}
	v.contentView.ScrollTo(newRow, col)
}

func (v *ContextMapViewer) scrollDown(lines int) {
	row, col := v.contentView.GetScrollOffset()
	v.contentView.ScrollTo(row+lines, col)
}

func (v *ContextMapViewer) findMatches() {
	v.searchMatches = nil
	if v.searchTerm == "" {
		return
	}

	lowerSearch := strings.ToLower(v.searchTerm)
	for i, line := range v.lines {
		if strings.Contains(strings.ToLower(line), lowerSearch) {
			v.searchMatches = append(v.searchMatches, i)
		}
	}
}

func (v *ContextMapViewer) nextMatch() {
	if len(v.searchMatches) == 0 {
		return
	}
	v.currentMatch++
	if v.currentMatch >= len(v.searchMatches) {
		v.currentMatch = 0
	}
	v.highlightAndDisplay()
	v.scrollToCurrentMatch()
}

func (v *ContextMapViewer) prevMatch() {
	if len(v.searchMatches) == 0 {
		return
	}
	v.currentMatch--
	if v.currentMatch < 0 {
		v.currentMatch = len(v.searchMatches) - 1
	}
	v.highlightAndDisplay()
	v.scrollToCurrentMatch()
}

func (v *ContextMapViewer) scrollToCurrentMatch() {
	if len(v.searchMatches) == 0 || v.currentMatch >= len(v.searchMatches) {
		return
	}
	matchLine := v.searchMatches[v.currentMatch]
	// Scroll so the match is roughly centered
	_, _, _, height := v.contentView.GetInnerRect()
	targetRow := matchLine - height/2
	if targetRow < 0 {
		targetRow = 0
	}
	v.contentView.ScrollTo(targetRow, 0)
}

func (v *ContextMapViewer) displayContent() {
	v.contentView.SetText(escapeBrackets(v.rawContent))
	v.updateStatusBar()
}

func (v *ContextMapViewer) highlightAndDisplay() {
	if v.searchTerm == "" || len(v.searchMatches) == 0 {
		if v.searchTerm != "" && len(v.searchMatches) == 0 {
			v.contentView.SetText(escapeBrackets(v.rawContent) + "\n\n[red]No matches found[-]")
		} else {
			v.displayContent()
		}
		v.updateStatusBar()
		return
	}

	// Build highlighted content
	var highlighted strings.Builder
	matchSet := make(map[int]int) // line index -> match index
	for i, lineIdx := range v.searchMatches {
		matchSet[lineIdx] = i
	}

	for i, line := range v.lines {
		if matchIdx, isMatch := matchSet[i]; isMatch {
			if matchIdx == v.currentMatch {
				// Current match - yellow background
				highlighted.WriteString("[black:yellow]")
				highlighted.WriteString(escapeBrackets(line))
				highlighted.WriteString("[-:-:-]")
			} else {
				// Other matches - cyan foreground
				highlighted.WriteString("[cyan]")
				highlighted.WriteString(escapeBrackets(line))
				highlighted.WriteString("[-]")
			}
		} else {
			highlighted.WriteString(escapeBrackets(line))
		}
		highlighted.WriteString("\n")
	}

	v.contentView.SetText(highlighted.String())
	v.updateStatusBar()
}

func (v *ContextMapViewer) updateStatusBar() {
	if len(v.searchMatches) > 0 {
		v.statusBar.SetText(fmt.Sprintf(" [green]Match %d/%d[-]  [yellow]/[-] Search  [yellow]n/N[-] Next/Prev  [yellow]j/k[-] Scroll  [yellow]Esc[-] Close",
			v.currentMatch+1, len(v.searchMatches)))
	} else if v.searchTerm != "" {
		v.statusBar.SetText(" [red]No matches[-]  [yellow]/[-] Search  [yellow]n/N[-] Next/Prev  [yellow]j/k[-] Scroll  [yellow]Esc[-] Close")
	} else {
		v.statusBar.SetText(" [yellow]/[-] Search  [yellow]n/N[-] Next/Prev  [yellow]j/k[-] Scroll  [yellow]g/G[-] Top/Bottom  [yellow]Esc[-] Close")
	}
}

// Show displays the context map viewer with the given data
func (v *ContextMapViewer) Show(title string, data any) {
	if data == nil {
		return
	}

	// Format JSON
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return
	}

	v.rawContent = string(jsonBytes)
	v.lines = strings.Split(v.rawContent, "\n")
	v.searchTerm = ""
	v.searchMatches = nil
	v.currentMatch = 0

	v.contentView.SetTitle(title)
	v.displayContent()
	v.contentView.ScrollToBeginning()

	v.pages.AddPage("contextmap", v.modal, true, true)
	v.app.SetFocus(v.contentView)
}

// Close closes the context map viewer
func (v *ContextMapViewer) Close() {
	v.pages.RemovePage("contextmap")
	if v.onClose != nil {
		v.onClose()
	}
}

// ShowContextMap shows the context map for the given event data
func (c *ConsoleApp) showContextMapForEvent(data silky.StepProfilerData) {
	var contextData any
	var title string

	switch data.Type {
	case silky.EVENT_URL_COMPOSITION:
		contextData = data.Data["goTemplateContext"]
		title = " Template Context "
	case silky.EVENT_CONTEXT_SELECTION, silky.EVENT_CONTEXT_MERGE:
		contextData = data.Data["fullContextMap"]
		title = " Context Map "
	default:
		return
	}

	if contextData == nil {
		return
	}

	viewer := NewContextMapViewer(c.app, c.pages, func() {
		c.app.SetFocus(c.steps)
	})
	viewer.Show(title, contextData)
}
