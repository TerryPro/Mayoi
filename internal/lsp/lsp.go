// Package lsp runs a language server over stdio: the JSON-RPC messages it
// speaks, its documents, and the diagnostics it publishes. It is the
// client only: a server is a program of the system, started for a folder
// as the editor needs it.
package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Position is a place in a document: a line and a character, both 0-based,
// the character in UTF-16 units, as LSP has them.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range is a run of a document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Diagnostic is a problem a server found in a document: Severity 1 is an
// error, 2 a warning, 3 information and 4 a hint.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"`
	Source   string `json:"source"`
	Message  string `json:"message"`
}

// Location is a place a definition is at.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// FoldingRange is a run of lines a server folds: the lines after StartLine
// through EndLine, while StartLine stays. Kind is "comment" for a comment's
// block, "imports" for an import block, "region" for one marked by the
// language, and "" for code.
type FoldingRange struct {
	StartLine      int    `json:"startLine"`
	StartCharacter int    `json:"startCharacter"`
	EndLine        int    `json:"endLine"`
	EndCharacter   int    `json:"endCharacter"`
	Kind           string `json:"kind"`
}

// TextEdit is a change of a document's text: the text NewText in place of
// the run of Range.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// CompletionItem is a word a server offers at a place: its Label, what
// Detail says of it, and the Text it inserts, in place of Edit's range when
// the server names one.
type CompletionItem struct {
	Label            string    `json:"label"`
	Kind             int       `json:"kind"`
	Detail           string    `json:"detail"`
	Documentation    any       `json:"documentation"`
	SortText         string    `json:"sortText"`
	FilterText       string    `json:"filterText"`
	InsertText       string    `json:"insertText"`
	InsertTextFormat int       `json:"insertTextFormat"`
	TextEdit         *TextEdit `json:"textEdit"`
}

// CompletionList is what a server offers at a place: its items, and
// whether more may come as the text goes on.
type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

// Hover is what a server says of the symbol at a place: its Contents, as
// text or markup, and the Range they are of.
type Hover struct {
	Contents hoverContents `json:"contents"`
	Range    *Range        `json:"range"`
}

// CodeAction is a change a server offers to a problem: what Title says it
// does, and the Edit that does it.
type CodeAction struct {
	Title       string         `json:"title"`
	Kind        string         `json:"kind"`
	IsPreferred bool           `json:"isPreferred"`
	Edit        *WorkspaceEdit `json:"edit"`
	Command     any            `json:"command"`
}

// WorkspaceEdit is a change of documents: the edits of each by its URI,
// or the edits of each document the server names.
type WorkspaceEdit struct {
	Changes         map[string][]TextEdit `json:"changes"`
	DocumentChanges []TextDocumentEdit    `json:"documentChanges"`
}

// TextDocumentEdit is the edits of one document.
type TextDocumentEdit struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Edits []TextEdit `json:"edits"`
}

// Text returns the hover's contents as text: its markup's value, or the
// strings of its list.
func (h Hover) Text() string { return h.Contents.Value }

// hoverContents is a hover's contents, which the protocol may spell as a
// string, a markup content, or a list of either.
type hoverContents struct {
	Kind  string
	Value string
}

func (h *hoverContents) UnmarshalJSON(data []byte) error {
	var s string
	if json.Unmarshal(data, &s) == nil {
		h.Value = s
		return nil
	}
	var content struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	}
	if json.Unmarshal(data, &content) == nil && content.Value != "" {
		h.Kind, h.Value = content.Kind, content.Value
		return nil
	}
	var list []json.RawMessage
	if json.Unmarshal(data, &list) == nil {
		var parts []string
		for _, raw := range list {
			var one hoverContents
			if one.UnmarshalJSON(raw) == nil && one.Value != "" {
				parts = append(parts, one.Value)
			}
		}
		h.Kind, h.Value = "markdown", strings.Join(parts, "\n\n")
	}
	return nil
}

// ErrUnsupported is what a request returns for a method the server does not
// implement.
var ErrUnsupported = errors.New("lsp: the request is not supported")

// Client is a language server's process and the messages it exchanges.
type Client struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out *bufio.Reader

	mu      sync.Mutex
	next    atomic.Int64
	pending map[int64]chan result
	diag    func(uri string, diagnostics []Diagnostic)
	done    chan struct{}

	closeOnce sync.Once
}

type result struct {
	raw json.RawMessage
	err error
}

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Start runs command in dir, and returns the client that speaks to it.
// onDiagnostics is called on a goroutine of the client's own for each
// document whose problems change.
func Start(command string, args []string, dir string, onDiagnostics func(uri string, diagnostics []Diagnostic)) (*Client, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	cmd.Stderr = io.Discard
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &Client{
		cmd: cmd, in: in, out: bufio.NewReader(out),
		pending: map[int64]chan result{}, diag: onDiagnostics, done: make(chan struct{}),
	}
	go c.read()
	return c, nil
}

// Initialize starts the server's session in root.
func (c *Client) Initialize(ctx context.Context, root string) error {
	params := map[string]any{
		"processId": nil,
		"rootUri":   URI(root),
		// A server publishes diagnostics only for a client that says it
		// takes them, as typescript-language-server checks.
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"publishDiagnostics": map[string]any{"relatedInformation": true, "versionSupport": true},
				// Only the lines of a folding range are folded, and the
				// kinds a server sends are all taken.
				"foldingRange": map[string]any{
					"lineFoldingOnly":  true,
					"foldingRangeKind": map[string]any{"valueSet": []string{"comment", "imports", "region"}},
				},
				"completion": map[string]any{
					"completionItem": map[string]any{
						"snippetSupport":      false,
						"documentationFormat": []string{"markdown", "plaintext"},
					},
				},
				"hover": map[string]any{"contentFormat": []string{"markdown", "plaintext"}},
			},
			"workspace": map[string]any{"workspaceFolders": true},
		},
		"workspaceFolders": []map[string]any{{"uri": URI(root), "name": filepath.Base(root)}},
	}
	var raw json.RawMessage
	if err := c.call(ctx, "initialize", params, &raw); err != nil {
		return err
	}
	return c.notify("initialized", map[string]any{})
}

// DidOpen tells the server of a document it has not seen.
func (c *Client) DidOpen(uri, languageID, text string) error {
	return c.notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": uri, "languageId": languageID, "version": 1, "text": text},
	})
}

// DidChange tells the server of a document's text as it is now.
func (c *Client) DidChange(uri string, version int, text string) error {
	return c.notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": uri, "version": version},
		"contentChanges": []map[string]any{{"text": text}},
	})
}

// DidClose tells the server that a document is no longer open.
func (c *Client) DidClose(uri string) error {
	return c.notify("textDocument/didClose", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	})
}

// Definition asks where the symbol at pos is defined.
func (c *Client) Definition(ctx context.Context, uri string, pos Position) ([]Location, error) {
	var raw json.RawMessage
	if err := c.call(ctx, "textDocument/definition", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     pos,
	}, &raw); err != nil {
		return nil, err
	}
	var locations []Location
	if err := json.Unmarshal(raw, &locations); err == nil {
		return locations, nil
	}
	var one Location
	if err := json.Unmarshal(raw, &one); err != nil {
		return nil, err
	}
	return []Location{one}, nil
}

// FoldingRange returns the runs of lines of a document that its server
// folds, [] when it has none.
func (c *Client) FoldingRange(ctx context.Context, uri string) ([]FoldingRange, error) {
	var ranges []FoldingRange
	if err := c.call(ctx, "textDocument/foldingRange", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	}, &ranges); err != nil {
		return nil, err
	}
	return ranges, nil
}

// Completion asks what the server offers at pos, as the text stands there.
func (c *Client) Completion(ctx context.Context, uri string, pos Position) (CompletionList, error) {
	var raw json.RawMessage
	if err := c.call(ctx, "textDocument/completion", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     pos,
	}, &raw); err != nil {
		return CompletionList{}, err
	}
	var items []CompletionItem
	if json.Unmarshal(raw, &items) == nil {
		return CompletionList{Items: items}, nil
	}
	var list CompletionList
	if err := json.Unmarshal(raw, &list); err != nil {
		return CompletionList{}, err
	}
	return list, nil
}

// Hover asks what the server says of the symbol at pos, nil when it says
// nothing.
func (c *Client) Hover(ctx context.Context, uri string, pos Position) (*Hover, error) {
	var raw json.RawMessage
	if err := c.call(ctx, "textDocument/hover", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     pos,
	}, &raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var hover Hover
	if err := json.Unmarshal(raw, &hover); err != nil {
		return nil, err
	}
	if hover.Text() == "" {
		return nil, nil
	}
	return &hover, nil
}

// CodeAction asks what the server offers to fix the problems diags of the
// range rng, nil when it offers nothing.
func (c *Client) CodeAction(ctx context.Context, uri string, rng Range, diags []Diagnostic) ([]CodeAction, error) {
	var actions []CodeAction
	if err := c.call(ctx, "textDocument/codeAction", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"range":        rng,
		"context":      map[string]any{"diagnostics": diags},
	}, &actions); err != nil {
		return nil, err
	}
	return actions, nil
}

// Close ends the server's session, and its process.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		var raw json.RawMessage
		_ = c.call(ctx, "shutdown", nil, &raw)
		_ = c.notify("exit", nil)
		_ = c.in.Close()
		done := make(chan struct{}, 1)
		go func() { _ = c.cmd.Wait(); done <- struct{}{} }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			if c.cmd.Process != nil {
				_ = c.cmd.Process.Kill()
			}
		}
	})
	return nil
}

// URI returns the URI of a path, as LSP spells file paths.
func URI(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// Path returns the path of a URI, "" for one that is not a file.
func Path(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	p := u.Path
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

// Key returns the key a document's diagnostics are kept by, which folds
// case on Windows, where a path's drive letter may be either, as a server
// spells it.
func Key(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}

func (c *Client) notify(method string, params any) error {
	return c.writeJSON(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *Client) call(ctx context.Context, method string, params, out any) error {
	id := c.next.Add(1)
	ch := make(chan result, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	if err := c.writeJSON(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return err
	}
	select {
	case r := <-ch:
		if r.err != nil {
			return r.err
		}
		if out != nil {
			return json.Unmarshal(r.raw, out)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return errors.New("lsp: the server exited")
	}
}

func (c *Client) writeJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := fmt.Fprintf(c.in, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
		return err
	}
	_, err = c.in.Write(data)
	return err
}

func (c *Client) read() {
	defer close(c.done)
	for {
		msg, err := c.readMessage()
		if err != nil {
			c.fail(err)
			return
		}
		if msg.ID != nil {
			c.mu.Lock()
			ch := c.pending[*msg.ID]
			delete(c.pending, *msg.ID)
			c.mu.Unlock()
			if ch == nil {
				continue
			}
			if msg.Error != nil {
				if msg.Error.Code == -32601 {
					ch <- result{err: ErrUnsupported}
				} else {
					ch <- result{err: fmt.Errorf("lsp: %s", msg.Error.Message)}
				}
			} else {
				ch <- result{raw: msg.Result}
			}
			continue
		}
		if msg.Method == "textDocument/publishDiagnostics" {
			var p struct {
				URI         string       `json:"uri"`
				Diagnostics []Diagnostic `json:"diagnostics"`
			}
			if json.Unmarshal(msg.Params, &p) == nil && c.diag != nil {
				c.diag(p.URI, p.Diagnostics)
			}
		}
	}
}

func (c *Client) readMessage() (*message, error) {
	length := -1
	for {
		line, err := c.out.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				return nil, err
			}
			length = n
		}
	}
	if length < 0 {
		return nil, errors.New("lsp: a message without Content-Length")
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(c.out, body); err != nil {
		return nil, err
	}
	var msg message
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// fail tells the calls waiting of a server that ended, and keeps its error.
func (c *Client) fail(err error) {
	c.mu.Lock()
	for id, ch := range c.pending {
		ch <- result{err: err}
		delete(c.pending, id)
	}
	c.mu.Unlock()
}
