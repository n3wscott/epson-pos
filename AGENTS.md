# AGENTS.md

## Project Overview

This repo is a Go-based Epson ESC/POS printer playground. It provides:

- A Cobra CLI named `escpos`.
- Raw printing to Ethernet printers over TCP, usually `HOST:9100`.
- Raw USB printing through the macOS CUPS USB backend.
- A local web dashboard for editing, previewing, templating, and printing
  markdown receipts.
- A small markdown dialect that compiles to textual ESC/POS and then to raw
  printer bytes.

The known network printer target is `192.168.86.22:9100`.

## Important Commands

Use these from the repository root:

```sh
make
make restart
make stop
make status
make logs
go test ./...
go vet ./...
```

The default dashboard URL is `http://127.0.0.1:8080/`.

The Makefile accepts:

```sh
make PRINTER=192.168.86.22:9100
make TEMPLATES_DIR=/path/to/templates
make ADDR=127.0.0.1:8081
```

## Code Map

- `pkg/commands/commands.go`: command registration.
- `pkg/commands/print.go`: raw print command and transport selection.
- `pkg/commands/devices.go`: USB device discovery through CUPS.
- `pkg/commands/serve.go`: dashboard server, API handlers, embedded frontend.
- `pkg/escpos/markdown.go`: markdown/directive to textual ESC/POS compiler.
- `pkg/escpos/preview.go`: dashboard text preview from textual ESC/POS.
- `pkg/escpos/template.go`: template field parsing and rendering.
- `pkg/escpos/converter.go`: textual ESC/POS to raw byte conversion.
- `pkg/transport/`: TCP and USB transport implementations.
- `templates/`: markdown receipt templates stored as user-shareable files.

## Receipt Pipeline

The core pipeline is:

```text
template markdown
  -> RenderTemplate
  -> MarkdownToPOS
  -> Preview for dashboard display
  -> Convert for raw ESC/POS bytes
  -> transport.DialTCP or transport.OpenCUPSUSBBackend
```

Keep the preview and print compiler paths aligned. If a markdown directive
prints, the preview should either show it or clearly show a placeholder.

## HTTP API Contract

External callers should use the versioned markdown endpoints:

- `POST /api/v1/markdown/preview`
- `POST /api/v1/markdown/print`

Both accept JSON:

```json
{
  "source": "# Lantern Market\n\n::barcode code39 1042"
}
```

`source` is required. `printer` is optional and defaults to the server's
`--printer` value. If `--printer` is omitted at startup, the default is
`192.168.86.22:9100`. Preview accepts the same shape but does not connect to the
printer. The HTTP print endpoint currently prints over raw TCP only. A caller
may include `"printer":"HOST:PORT"` to override the server default for one
request.

Preview success response:

```json
{
  "preview": "...",
  "bytes": 96,
  "pos": "..."
}
```

Print success response:

```json
{
  "ok": true,
  "bytes": 174
}
```

Keep `/api/preview` and `/api/print` working for the dashboard, but document and
test the `/api/v1/markdown/...` paths as the external integration contract.

## Markdown Dialect

Supported markdown and directives include:

- `#` through `####` headings mapped to printer size/bold/alignment.
- `**bold**`, basic markdown links, inline code stripping.
- Simple table rows using `|`.
- Ordered and unordered list items.
- Rules through markdown rules or `::line`.
- Alignment: `::align left|center|right`, `::left`, `::center`, `::right`.
- Style: `::bold on|off`, `::font a|b`, `::size WIDTHxHEIGHT`.
- Paper movement: `::feed N`, `::cut partial N`, `::cut full N`.
- Images: `::image A1`, `::nv-image A1`.
- QR: `::qr VALUE`.
- Row layout: `::row image:A1 qr:VALUE` composes the local image asset and QR
  into one raster row with `GS v 0`; it intentionally avoids page mode because
  this printer rendered the page-mode test incorrectly.
- Barcodes: `upca`, `upce`, `ean13`, `ean8`, `code39`, `itf`, `codabar`,
  `code93`, and `code128`.

Template fields are declared with comments:

```md
<!-- field:order_id hint="Order number" default="1042" -->
Order | {{order_id}}
```

## Testing Expectations

Run `go test ./...` and `go vet ./...` after code changes. Add tests under
`pkg/escpos/` for markdown, preview, template, and converter behavior.

Useful existing regression areas:

- Quoted strings in textual ESC/POS must preserve spaces during `Convert`.
- Preview lines should fit the configured receipt width.
- Template markdown files should render with defaults and compile to bytes.

## Frontend Notes

The dashboard frontend is embedded in `pkg/commands/serve.go`. Keep CSS scoped
under `.shell` so browser annotations and extension UI are not affected by
global app styles. The receipt preview is intentionally fixed-width and should
not soft-wrap barcode or separator lines.

## Printer Notes

Treat this as a raw ESC/POS device, not a normal document printer. For Ethernet,
send bytes to port `9100`. For USB, discover device URIs with `go run . devices`
and print with:

```sh
go run . print 'usb://EPSON/...' --transport usb --file examples/simple.pos
```

Avoid sending arbitrary test prints unless the user asks. Prefer compiling and
preview/API checks first.
