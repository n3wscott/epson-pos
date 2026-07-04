/*
Copyright 2022 Scott Nichols
SPDX-License-Identifier: Apache-2.0
*/

package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/n3wscott/escpos/pkg/escpos"
)

const defaultPrinterTarget = "192.168.86.22:9100"

func Serve() *cobra.Command {
	var addr string
	var printer string
	var printerMAC string
	var stateFile string
	var templatesDir string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the local ESC/POS dashboard.",
		RunE: func(cmd *cobra.Command, args []string) error {
			app, err := newDashboardWithState(addr, printer, printerMAC, templatesDir, stateFile)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Dashboard: http://%s/\nPrinter:   %s\nPrinter MAC: %s\nTemplates: %s\nState:     %s\n", addr, printer, printerMAC, templatesDir, stateFile)
			return app.listenAndServe()
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8080", "Dashboard listen address.")
	cmd.Flags().StringVar(&printer, "printer", defaultPrinterTarget, "Raw ESC/POS printer target.")
	cmd.Flags().StringVar(&printerMAC, "printer-mac", "", "Optional expected printer MAC address; when set, discovery only accepts this device.")
	cmd.Flags().StringVar(&stateFile, "state-file", "printer_state.json", "Path for persistent printer target state.")
	cmd.Flags().StringVar(&templatesDir, "templates-dir", "templates", "Directory for markdown receipt templates.")
	return cmd
}

type dashboard struct {
	addr         string
	printer      string
	printerMAC   string
	printers     *printerManager
	templatesDir string
	tpl          *template.Template
}

type dashboardPage struct {
	Printer  string
	Sample   string
	Template string
}

type printRequest struct {
	Source  string `json:"source"`
	Printer string `json:"printer"`
}

type templateFieldsRequest struct {
	Template string `json:"template"`
}

type templateDocument struct {
	Name   string `json:"name"`
	Source string `json:"source"`
}

type templateListResponse struct {
	Templates []templateDocument `json:"templates"`
	Error     string             `json:"error,omitempty"`
}

type templateFieldsResponse struct {
	Fields []escpos.TemplateField `json:"fields"`
	Error  string                 `json:"error,omitempty"`
}

type previewResponse struct {
	Preview string `json:"preview"`
	Bytes   int    `json:"bytes"`
	POS     string `json:"pos,omitempty"`
	Error   string `json:"error,omitempty"`
}

type printResponse struct {
	OK    bool   `json:"ok"`
	Bytes int    `json:"bytes,omitempty"`
	Error string `json:"error,omitempty"`
}

func newDashboard(addr, printer, templatesDir string) (*dashboard, error) {
	return newDashboardWithState(addr, printer, "", templatesDir, filepath.Join(templatesDir, "printer_state.json"))
}

func newDashboardWithState(addr, printer, printerMAC, templatesDir, stateFile string) (*dashboard, error) {
	tpl, err := template.New("dashboard").Parse(dashboardHTML)
	if err != nil {
		return nil, err
	}
	if err := ensureTemplatesDir(templatesDir); err != nil {
		return nil, err
	}
	printers, err := newPrinterManager(printer, printerMAC, stateFile)
	if err != nil {
		return nil, err
	}
	return &dashboard{
		addr:         addr,
		printer:      printer,
		printerMAC:   printerMAC,
		printers:     printers,
		templatesDir: templatesDir,
		tpl:          tpl,
	}, nil
}

func (d *dashboard) listenAndServe() error {
	server := &http.Server{
		Addr:    d.addr,
		Handler: d.routes(),
	}
	return server.ListenAndServe()
}

func (d *dashboard) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", d.handleIndex)
	mux.HandleFunc("/api/preview", d.handlePreview)
	mux.HandleFunc("/api/print", d.handlePrint)
	mux.HandleFunc("/api/status", d.handleStatus)
	mux.HandleFunc("/api/printer", d.handleStatus)
	mux.HandleFunc("/api/v1/markdown/preview", d.handlePreview)
	mux.HandleFunc("/api/v1/markdown/print", d.handlePrint)
	mux.HandleFunc("/api/templates", d.handleTemplates)
	mux.HandleFunc("/api/template/fields", d.handleTemplateFields)
	return mux
}

func (d *dashboard) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := d.tpl.Execute(w, dashboardPage{Printer: d.printer, Sample: sampleReceipt, Template: sampleTemplate}); err != nil {
		log.Printf("render dashboard: %v", err)
	}
}

func (d *dashboard) handlePreview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, err := readPrintRequest(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, previewResponse{Error: err.Error()})
		return
	}

	pos, compiled, err := compileMarkdown(req.Source)
	resp := previewResponse{Preview: escpos.Preview(pos), Bytes: len(compiled), POS: pos}
	if err != nil {
		resp.Error = err.Error()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (d *dashboard) handlePrint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, err := readPrintRequest(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, printResponse{Error: err.Error()})
		return
	}
	_, compiled, err := compileMarkdown(req.Source)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, printResponse{Error: err.Error()})
		return
	}

	if err := d.printers.Print(r.Context(), compiled, req.Printer); err != nil {
		writeJSON(w, http.StatusBadGateway, printResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, printResponse{OK: true, Bytes: len(compiled)})
}

func (d *dashboard) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, d.printers.Status(r.Context()))
}

func (d *dashboard) handleTemplateFields(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req templateFieldsRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, templateFieldsResponse{Error: fmt.Errorf("invalid JSON: %w", err).Error()})
		return
	}
	writeJSON(w, http.StatusOK, templateFieldsResponse{Fields: escpos.ParseTemplateFields(req.Template)})
}

func (d *dashboard) handleTemplates(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		templates, err := listTemplateDocuments(d.templatesDir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, templateListResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, templateListResponse{Templates: templates})
	case http.MethodPost:
		var doc templateDocument
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&doc); err != nil {
			writeJSON(w, http.StatusBadRequest, templateListResponse{Error: fmt.Errorf("invalid JSON: %w", err).Error()})
			return
		}
		if err := writeTemplateDocument(d.templatesDir, doc); err != nil {
			writeJSON(w, http.StatusBadRequest, templateListResponse{Error: err.Error()})
			return
		}
		templates, err := listTemplateDocuments(d.templatesDir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, templateListResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, templateListResponse{Templates: templates})
	case http.MethodDelete:
		name := r.URL.Query().Get("name")
		if err := deleteTemplateDocument(d.templatesDir, name); err != nil {
			writeJSON(w, http.StatusBadRequest, templateListResponse{Error: err.Error()})
			return
		}
		templates, err := listTemplateDocuments(d.templatesDir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, templateListResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, templateListResponse{Templates: templates})
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func readPrintRequest(in io.Reader) (printRequest, error) {
	var req printRequest
	if err := json.NewDecoder(io.LimitReader(in, 1<<20)).Decode(&req); err != nil {
		return req, fmt.Errorf("invalid JSON: %w", err)
	}
	if strings.TrimSpace(req.Source) == "" {
		return req, fmt.Errorf("source is empty")
	}
	return req, nil
}

func compileMarkdown(source string) (pos string, out []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	pos, err = escpos.MarkdownToPOS(source)
	if err != nil {
		return "", nil, err
	}
	var b bytes.Buffer
	if err := escpos.Convert(strings.NewReader(pos), &b); err != nil {
		return "", nil, err
	}
	return pos, b.Bytes(), nil
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("write JSON: %v", err)
	}
}

var templateNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _.-]*$`)

func ensureTemplatesDir(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	templates, err := listTemplateDocuments(dir)
	if err != nil {
		return err
	}
	if len(templates) > 0 {
		return nil
	}
	return writeTemplateDocument(dir, templateDocument{Name: "Market Order", Source: sampleTemplate})
}

func listTemplateDocuments(dir string) ([]templateDocument, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	templates := []templateDocument(nil)
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		templates = append(templates, templateDocument{Name: name, Source: string(data)})
	}
	return templates, nil
}

func writeTemplateDocument(dir string, doc templateDocument) error {
	name := strings.TrimSpace(doc.Name)
	if name == "" {
		return fmt.Errorf("template name is required")
	}
	if !templateNamePattern.MatchString(name) {
		return fmt.Errorf("template name can use letters, numbers, spaces, dot, dash, and underscore")
	}
	if strings.TrimSpace(doc.Source) == "" {
		return fmt.Errorf("template source is required")
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(templateDocumentPath(dir, name), []byte(doc.Source), 0644)
}

func deleteTemplateDocument(dir, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("template name is required")
	}
	if !templateNamePattern.MatchString(name) {
		return fmt.Errorf("invalid template name")
	}
	path := templateDocumentPath(dir, name)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func templateDocumentPath(dir, name string) string {
	file := strings.ReplaceAll(strings.TrimSpace(name), string(filepath.Separator), "-") + ".md"
	return filepath.Join(dir, file)
}

const sampleReceipt = `# Lantern Market

Order | #1042
Latte | $4.50
Bagel | $3.25

::line
::barcode code39 1042

Thank you
::cut partial 30
`

const sampleTemplate = `<!-- field:order_id hint="Order number or ticket id" default="1042" -->
<!-- field:latte_price hint="Latte line price" default="$4.50" -->
<!-- field:bagel_price hint="Bagel line price" default="$3.25" -->

# Lantern Market

Order | {{order_id}}
Latte | {{latte_price}}
Bagel | {{bagel_price}}

::line
::barcode code39 {{order_id}}

Thank you
::cut partial 30
`

const dashboardHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>ESC/POS Dashboard</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f6f7f9;
      --panel: #ffffff;
      --ink: #1f252b;
      --muted: #66717d;
      --line: #d9dee5;
      --accent: #0f766e;
      --accent-dark: #115e59;
      --danger: #b42318;
      --paper: #fffdf7;
      --paper-line: #e6dfcf;
      font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    .shell, .shell * { box-sizing: border-box; }
    body {
      margin: 0;
      background: var(--bg);
      color: var(--ink);
    }
    .shell button, .shell input, .shell textarea {
      font: inherit;
    }
    .shell {
      min-height: 100vh;
      display: grid;
      grid-template-rows: auto 1fr;
    }
    .shell header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      padding: 14px 18px;
      border-bottom: 1px solid var(--line);
      background: #ffffff;
    }
    .shell h1 {
      margin: 0;
      font-size: 18px;
      font-weight: 650;
      letter-spacing: 0;
    }
    .status {
      display: flex;
      align-items: center;
      gap: 8px;
      color: var(--muted);
      font-size: 13px;
      white-space: nowrap;
    }
    .dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      background: var(--accent);
      display: inline-block;
    }
    .shell main {
      display: grid;
      grid-template-columns: minmax(340px, 1fr) minmax(320px, 480px);
      gap: 18px;
      padding: 18px;
      min-height: 0;
    }
    .workspace, .previewPane {
      min-height: 0;
      display: flex;
      flex-direction: column;
      gap: 12px;
    }
    .toolbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      min-height: 36px;
    }
    .tabs {
      display: inline-flex;
      width: max-content;
      border: 1px solid var(--line);
      border-radius: 8px;
      overflow: hidden;
      background: #fff;
    }
    .shell button.tab {
      border: 0;
      border-right: 1px solid var(--line);
      border-radius: 0;
      min-height: 34px;
      padding: 7px 12px;
      color: var(--muted);
      background: #fff;
    }
    .shell button.tab:last-child {
      border-right: 0;
    }
    .shell button.tab.active {
      color: #fff;
      background: var(--accent);
    }
    .modePanel {
      min-height: 0;
      display: flex;
      flex: 1;
      flex-direction: column;
      gap: 12px;
    }
    .modePanel[hidden] {
      display: none;
    }
    .field {
      display: flex;
      align-items: center;
      gap: 8px;
      min-width: 260px;
      color: var(--muted);
      font-size: 13px;
    }
    .field input {
      width: min(320px, 42vw);
      border: 1px solid var(--line);
      border-radius: 6px;
      padding: 7px 9px;
      color: var(--ink);
      background: #fff;
    }
    .actions {
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .shell button {
      border: 1px solid var(--line);
      border-radius: 6px;
      background: #fff;
      color: var(--ink);
      padding: 8px 10px;
      cursor: pointer;
      min-height: 36px;
    }
    .shell button.primary {
      background: var(--accent);
      border-color: var(--accent);
      color: #fff;
      font-weight: 650;
    }
    .shell button.primary:hover {
      background: var(--accent-dark);
      border-color: var(--accent-dark);
    }
    .shell button:disabled {
      cursor: not-allowed;
      opacity: .6;
    }
    .shell textarea {
      flex: 1;
      min-height: 420px;
      resize: none;
      border: 1px solid var(--line);
      border-radius: 8px;
      padding: 14px;
      line-height: 1.45;
      color: #111827;
      background: #fff;
      font-family: "SFMono-Regular", Consolas, "Liberation Mono", monospace;
      font-size: 13px;
      tab-size: 4;
    }
    .templateTools {
      display: grid;
      grid-template-columns: minmax(160px, 1fr) minmax(160px, 1fr) auto auto;
      gap: 8px;
      align-items: center;
    }
    .templateTools input, .templateTools select {
      width: 100%;
      border: 1px solid var(--line);
      border-radius: 6px;
      padding: 7px 9px;
      color: var(--ink);
      background: #fff;
    }
    .templateSource {
      min-height: 280px;
    }
    .fields {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 10px;
    }
    .fieldCard {
      border: 1px solid var(--line);
      border-radius: 8px;
      background: #fff;
      padding: 10px;
      display: flex;
      flex-direction: column;
      gap: 6px;
    }
    .fieldCard label {
      font-size: 12px;
      font-weight: 650;
      color: var(--ink);
    }
    .fieldCard input {
      border: 1px solid var(--line);
      border-radius: 6px;
      padding: 7px 9px;
      color: var(--ink);
      background: #fff;
    }
    .fieldHint {
      color: var(--muted);
      font-size: 12px;
      min-height: 16px;
    }
    .cheats {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 10px;
      color: var(--muted);
      font-size: 12px;
      line-height: 1.45;
    }
    .cheatGroup {
      border: 1px solid var(--line);
      border-radius: 8px;
      background: #ffffff;
      padding: 10px;
    }
    .cheatGroup strong {
      display: block;
      color: var(--ink);
      font-size: 12px;
      margin-bottom: 6px;
    }
    .cheatGroup code {
      display: block;
      padding: 2px 0;
      color: #344054;
      font-family: "SFMono-Regular", Consolas, "Liberation Mono", monospace;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .receiptWrap {
      flex: 1;
      display: flex;
      justify-content: center;
      overflow: auto;
      border: 1px solid var(--line);
      border-radius: 8px;
      background: #e8ebef;
      padding: 18px;
    }
    .paper {
      width: calc(56ch + 30px);
      min-width: calc(56ch + 30px);
      min-height: 520px;
      background: var(--paper);
      color: #1c1c1c;
      border: 1px solid var(--paper-line);
      box-shadow: 0 12px 28px rgba(31,37,43,.14);
      padding: 18px 14px 28px;
      font-family: "SFMono-Regular", Consolas, "Liberation Mono", monospace;
      font-size: 11px;
      line-height: 1.28;
      white-space: pre;
      overflow-wrap: normal;
      word-break: normal;
    }
    .meta {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 10px;
      color: var(--muted);
      font-size: 13px;
      min-height: 20px;
    }
    .message {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      text-align: right;
    }
    .message.error {
      color: var(--danger);
    }
    @media (max-width: 880px) {
      .shell header, .toolbar {
        align-items: flex-start;
        flex-direction: column;
      }
      .shell main {
        grid-template-columns: 1fr;
      }
      textarea {
        min-height: 360px;
      }
      .cheats {
        grid-template-columns: 1fr;
      }
      .templateTools, .fields {
        grid-template-columns: 1fr;
      }
      .field, .field input {
        width: 100%;
      }
      .actions {
        width: 100%;
      }
      .shell button {
        flex: 1;
      }
    }
  </style>
</head>
<body>
  <div class="shell">
    <header>
      <h1>ESC/POS Dashboard</h1>
      <div class="status"><span class="dot"></span><span id="printerLabel">{{.Printer}}</span></div>
    </header>
    <main>
      <section class="workspace">
        <div class="toolbar">
          <label class="field">Printer <input id="printer" value="{{.Printer}}" spellcheck="false"></label>
          <div class="actions">
            <button id="reset" type="button" title="Restore sample">Reset</button>
            <button id="print" class="primary" type="button" title="Print receipt">Print</button>
          </div>
        </div>
        <div class="tabs" role="tablist" aria-label="Receipt mode">
          <button id="draftTab" class="tab active" type="button" role="tab" aria-selected="true" aria-controls="draftPanel">Draft</button>
          <button id="templateTab" class="tab" type="button" role="tab" aria-selected="false" aria-controls="templatePanel">Template</button>
        </div>
        <div id="draftPanel" class="modePanel" role="tabpanel" aria-labelledby="draftTab">
          <textarea id="source" spellcheck="false" aria-label="Markdown receipt source">{{.Sample}}</textarea>
          <div class="cheats" aria-label="Markdown printer cheat sheet">
            <div class="cheatGroup">
              <strong>Markdown</strong>
              <code># 3x3 centered heading</code>
              <code>## 2x2 centered heading</code>
              <code>### 2x1 centered heading</code>
              <code>#### bold centered heading</code>
              <code>**bold text**</code>
              <code>Item | Price</code>
              <code>- Bullet item</code>
            </div>
            <div class="cheatGroup">
              <strong>Printer Codes</strong>
              <code>::barcode code39 1042</code>
              <code>::barcode code128 ORDER-1042</code>
              <code>::qr https://example.test/1042</code>
              <code>::row image:A1 qr:https://example.test/1042</code>
              <code>::align left|center|right</code>
              <code>::size 1x1, ::size 2x2</code>
              <code>::line, ::feed 2, ::cut</code>
            </div>
          </div>
        </div>
        <div id="templatePanel" class="modePanel" role="tabpanel" aria-labelledby="templateTab" hidden>
          <div class="templateTools">
            <select id="templateSelect" aria-label="Receipt template"></select>
            <input id="templateName" aria-label="Template name" placeholder="Template name" spellcheck="false">
            <button id="saveTemplate" type="button">Save</button>
            <button id="deleteTemplate" type="button">Delete</button>
          </div>
          <textarea id="templateSource" class="templateSource" spellcheck="false" aria-label="Markdown receipt template">{{.Template}}</textarea>
          <div class="cheatGroup">
            <strong>Template Fields</strong>
            <code>&lt;!-- field:order_id hint="Order number" default="1042" --&gt;</code>
            <code>Use values with &#123;&#123;order_id&#125;&#125; placeholders.</code>
          </div>
          <div id="fieldForm" class="fields" aria-label="Template field inputs"></div>
        </div>
      </section>
      <section class="previewPane">
        <div class="meta">
          <span id="byteCount">0 bytes</span>
          <span id="message" class="message">Ready</span>
        </div>
        <div class="receiptWrap">
          <pre id="preview" class="paper"></pre>
        </div>
      </section>
    </main>
  </div>
  <script>
    const source = document.getElementById('source');
    const templateSource = document.getElementById('templateSource');
    const templateSelect = document.getElementById('templateSelect');
    const templateName = document.getElementById('templateName');
    const fieldForm = document.getElementById('fieldForm');
    const draftTab = document.getElementById('draftTab');
    const templateTab = document.getElementById('templateTab');
    const draftPanel = document.getElementById('draftPanel');
    const templatePanel = document.getElementById('templatePanel');
    const preview = document.getElementById('preview');
    const byteCount = document.getElementById('byteCount');
    const message = document.getElementById('message');
    const printer = document.getElementById('printer');
    const printerLabel = document.getElementById('printerLabel');
    const printButton = document.getElementById('print');
    const resetButton = document.getElementById('reset');
    const sample = source.value;
    let mode = 'draft';
    let templates = [];
    let previewTimer = 0;

    function setMessage(text, isError) {
      message.textContent = text;
      message.classList.toggle('error', Boolean(isError));
    }

    async function postJSON(url, body) {
      const response = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      });
      const data = await response.json();
      if (!response.ok) {
        throw new Error(data.error || response.statusText);
      }
      return data;
    }

    async function requestJSON(url, options) {
      const response = await fetch(url, options || {});
      const data = await response.json();
      if (!response.ok) {
        throw new Error(data.error || response.statusText);
      }
      return data;
    }

    async function loadTemplates(selectedName) {
      try {
        const data = await requestJSON('/api/templates');
        templates = data.templates || [];
        renderTemplateOptions(selectedName);
        if (templates.length > 0) {
          loadSelectedTemplate();
        }
      } catch (err) {
        setMessage(err.message, true);
      }
    }

    function renderTemplateOptions(selectedName) {
      templateSelect.innerHTML = '';
      templates.forEach((item) => {
        const option = document.createElement('option');
        option.value = item.name;
        option.textContent = item.name;
        templateSelect.appendChild(option);
      });
      if (templates.length === 0) {
        const option = document.createElement('option');
        option.value = '';
        option.textContent = 'No templates';
        templateSelect.appendChild(option);
      }
      if (selectedName && templates.some((item) => item.name === selectedName)) {
        templateSelect.value = selectedName;
      }
    }

    function selectedTemplate() {
      return templates.find((item) => item.name === templateSelect.value) || templates[0] || { name: '', source: templateSource.value };
    }

    function loadSelectedTemplate() {
      const item = selectedTemplate();
      templateName.value = item.name;
      templateSource.value = item.source;
      refreshTemplateFields();
    }

    function currentFieldValues() {
      const values = {};
      fieldForm.querySelectorAll('input[data-field]').forEach((input) => {
        values[input.dataset.field] = input.value;
      });
      return values;
    }

    async function refreshTemplateFields() {
      const previous = currentFieldValues();
      try {
        const data = await postJSON('/api/template/fields', { template: templateSource.value });
        fieldForm.innerHTML = '';
        (data.fields || []).forEach((field) => {
          const card = document.createElement('div');
          card.className = 'fieldCard';
          const label = document.createElement('label');
          const inputId = 'field_' + field.name;
          label.htmlFor = inputId;
          label.textContent = field.name;
          const input = document.createElement('input');
          input.id = inputId;
          input.dataset.field = field.name;
          input.placeholder = field.hint || field.name;
          input.value = Object.prototype.hasOwnProperty.call(previous, field.name) ? previous[field.name] : (field.default || '');
          const hint = document.createElement('div');
          hint.className = 'fieldHint';
          hint.textContent = field.hint || ' ';
          input.addEventListener('input', queuePreview);
          card.append(label, input, hint);
          fieldForm.appendChild(card);
        });
        if (!data.fields || data.fields.length === 0) {
          const empty = document.createElement('div');
          empty.className = 'fieldHint';
          empty.textContent = 'No fields found. Add comments like <!-- field:order_id hint="Order number" --> and placeholders like {' + '{order_id}' + '}.';
          fieldForm.appendChild(empty);
        }
      } catch (err) {
        setMessage(err.message, true);
      }
      queuePreview();
    }

    function renderTemplateSource() {
      const values = currentFieldValues();
      return templateSource.value.replace(/\{\{([A-Za-z_][A-Za-z0-9_-]*)\}\}/g, (token, name) => {
        return Object.prototype.hasOwnProperty.call(values, name) ? values[name] : token;
      });
    }

    function activeSource() {
      return mode === 'template' ? renderTemplateSource() : source.value;
    }

    function setMode(nextMode) {
      mode = nextMode;
      const templateMode = mode === 'template';
      draftPanel.hidden = templateMode;
      templatePanel.hidden = !templateMode;
      draftTab.classList.toggle('active', !templateMode);
      templateTab.classList.toggle('active', templateMode);
      draftTab.setAttribute('aria-selected', String(!templateMode));
      templateTab.setAttribute('aria-selected', String(templateMode));
      queuePreview();
    }

    async function renderPreview() {
      try {
        const data = await postJSON('/api/preview', { source: activeSource(), printer: printer.value });
        preview.textContent = data.preview || '';
        byteCount.textContent = (data.bytes || 0) + ' bytes';
        if (data.error) {
          setMessage(data.error, true);
        } else {
          setMessage('Ready', false);
        }
      } catch (err) {
        setMessage(err.message, true);
      }
    }

    function queuePreview() {
      clearTimeout(previewTimer);
      previewTimer = setTimeout(renderPreview, 120);
    }

    async function printReceipt() {
      printButton.disabled = true;
      setMessage('Printing...', false);
      try {
        const data = await postJSON('/api/print', { source: activeSource(), printer: printer.value });
        setMessage('Printed ' + (data.bytes || 0) + ' bytes', false);
      } catch (err) {
        setMessage(err.message, true);
      } finally {
        printButton.disabled = false;
      }
    }

    function saveTemplate() {
      const name = templateName.value.trim();
      if (!name) {
        setMessage('Template name is required', true);
        return;
      }
      postJSON('/api/templates', { name, source: templateSource.value })
        .then((data) => {
          templates = data.templates || [];
          renderTemplateOptions(name);
          setMessage('Template saved', false);
        })
        .catch((err) => setMessage(err.message, true));
    }

    function deleteTemplate() {
      const name = templateSelect.value;
      if (!name) {
        setMessage('No template selected', true);
        return;
      }
      fetch('/api/templates?name=' + encodeURIComponent(name), { method: 'DELETE' })
        .then(async (response) => {
          const data = await response.json();
          if (!response.ok) {
            throw new Error(data.error || response.statusText);
          }
          templates = data.templates || [];
          renderTemplateOptions();
          loadSelectedTemplate();
          setMessage('Template deleted', false);
        })
        .catch((err) => setMessage(err.message, true));
    }

    source.addEventListener('input', queuePreview);
    templateSource.addEventListener('input', refreshTemplateFields);
    templateSelect.addEventListener('change', loadSelectedTemplate);
    draftTab.addEventListener('click', () => setMode('draft'));
    templateTab.addEventListener('click', () => setMode('template'));
    document.getElementById('saveTemplate').addEventListener('click', saveTemplate);
    document.getElementById('deleteTemplate').addEventListener('click', deleteTemplate);
    printer.addEventListener('input', () => {
      printerLabel.textContent = printer.value;
    });
    printButton.addEventListener('click', printReceipt);
    resetButton.addEventListener('click', () => {
      if (mode === 'template') {
        loadSelectedTemplate();
      } else {
        source.value = sample;
        renderPreview();
      }
    });
    loadTemplates('Market Order');
    renderPreview();
  </script>
</body>
</html>`
