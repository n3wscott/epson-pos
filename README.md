# epson-pos

Tools for experimenting with an Epson ESC/POS thermal printer from Go. The
project can compile a small markdown receipt dialect into ESC/POS commands,
preview receipts in a local web dashboard, save reusable receipt templates, and
print raw bytes to a printer over Ethernet or USB.

The current hardware target is an Epson M244A-compatible thermal printer on the
network at `192.168.86.22:9100`. This project talks to the printer as a raw
ESC/POS device. It does not depend on configuring the printer as a normal macOS
document printer queue.

## Quick start

Run or restart the local dashboard:

```sh
make
```

Open:

```text
http://127.0.0.1:8080/
```

The default Makefile settings are:

```make
ADDR=127.0.0.1:8080
PRINTER=192.168.86.22:9100
TEMPLATES_DIR=templates
STATE_FILE=printer_state.json
```

Use a different printer or templates directory:

```sh
make PRINTER=192.168.1.40:9100 TEMPLATES_DIR=/path/to/templates
```

Useful server commands:

```sh
make start
make stop
make restart
make status
make logs
```

## Dashboard

The `serve` command hosts a local receipt dashboard with two authoring modes:

- Draft mode: write markdown directly, preview the receipt, and print it.
- Template mode: choose a saved markdown template, fill generated form fields,
  preview the rendered receipt, and print it.

The dashboard API reads templates from disk each time, so adding or editing a
file under `templates/` is enough for it to appear after the browser refreshes.

Run the server directly without Make:

```sh
go run . serve --addr 127.0.0.1:8080 --printer 192.168.86.22:9100 --templates-dir templates --state-file printer_state.json
```

## HTTP API contract

The dashboard server is also the local printer gateway for other apps. External
callers should use the versioned markdown endpoints below.

### Preview markdown

```http
POST /api/v1/markdown/preview
Content-Type: application/json
```

Request:

```json
{
  "source": "# Lantern Market\n\nOrder | 1042\n::barcode code39 1042"
}
```

`printer` may also be included, but it is optional. Preview accepts it only so
preview and print requests can share the same payload shape.

Success response:

```json
{
  "preview": "                     Lantern Market\n\nOrder    1042",
  "bytes": 96,
  "pos": "'// Generated from markdown\n    ESC \"@\"\n..."
}
```

### Print markdown

```http
POST /api/v1/markdown/print
Content-Type: application/json
```

Request:

```json
{
  "source": "# Lantern Market\n\nOrder | 1042\n::barcode code39 1042\n::cut partial 30"
}
```

`source` is required and must be non-empty. `printer` is optional; when omitted,
the server uses the `--printer` value it was started with. If `--printer` is not
provided, the built-in default is the known network printer,
`192.168.86.22:9100`. The server currently prints from the HTTP API over raw
TCP, so any override must be a `HOST:PORT` printer target.

The server tracks the active physical printer target. If a print fails while
dialing the target, it scans local private subnets for reachable port `9100`
printers, then retries the print once against the discovered target. It does
not retry after write failures, because a partial write may have already
started a receipt. The last active/successful target is persisted in
`--state-file`; on the Pi this should be
`/var/lib/epson-pos/printer_state.json`.

On networks with other raw-print devices, set `--printer-mac` to the Epson
printer's MAC address. When this is set, the server validates a target against
the ARP table before writing receipt bytes and will not select unrelated port
`9100` devices.

Success response:

```json
{
  "ok": true,
  "bytes": 174
}
```

Errors are JSON responses with an `error` field:

- `400 Bad Request`: invalid JSON, empty source, or markdown/directive compile
  error.
- `502 Bad Gateway`: printer connection or write failure.
- `405 Method Not Allowed`: endpoint was called with a method other than
  `POST`.

Example:

```sh
curl -sS http://127.0.0.1:8080/api/v1/markdown/print \
  -H 'Content-Type: application/json' \
  --data '{"source":"# Lantern Market\n\n::barcode code39 1042\n::cut partial 30"}'
```

Override the server default for a single request:

```sh
curl -sS http://127.0.0.1:8080/api/v1/markdown/print \
  -H 'Content-Type: application/json' \
  --data '{"source":"# Test\n\n::cut partial 30","printer":"192.168.1.40:9100"}'
```

The unversioned dashboard endpoints `/api/preview` and `/api/print` use the
same request and response bodies, but external callers should prefer the
versioned `/api/v1/markdown/...` paths.

### Printer status

```http
GET /api/status
```

`/api/printer` is an alias. The response includes the configured target, active
target, last successful target, last scan time, last print error, and whether
the active target is currently reachable.

```json
{
  "configured_target": "10.77.0.85:9100",
  "active_target": "192.168.86.56:9100",
  "last_successful_target": "192.168.86.56:9100",
  "last_scan_time": "2026-06-22T10:30:00Z",
  "reachable": true
}
```

## Templates

Templates are markdown files stored in `templates/` by default. The template
parser discovers fields from HTML comments and replaces `{{field_name}}`
placeholders with form values.

```md
<!-- field:order_id hint="Order number or ticket id" default="1042" -->
<!-- field:total hint="Receipt total" default="$7.75" -->

# Lantern Market

Order | {{order_id}}
Total | {{total}}

::barcode code39 {{order_id}}
::cut partial 30
```

The repo includes:

- `templates/Market Order.md`: a small receipt template.
- `templates/Printer Feature Showcase.md`: a broad demo covering headings,
  formatting directives, QR codes, NV images, and every supported barcode type.

## Markdown receipt dialect

Plain markdown is compiled into the project's textual ESC/POS format, then
converted to printer bytes. Supported markdown features:

```md
# 3x3 centered heading
## 2x2 centered heading
### 2x1 centered heading
#### bold centered heading

**inline bold**
Item | Price
--- | ---
Latte | $4.50

- Unordered list item
1. Ordered list item

---
```

Links render as their label text, and backticks are stripped from inline code.

Printer directives start with `::`:

```md
::align left
::align center
::align right
::left
::center
::right

::bold on
::bold off
::font a
::font b
::size 1x1
::size 2x1
::size 1x2
::size 2x2

::line
::feed 2
::cut partial 30
::cut full 30
```

Native symbol/image directives:

```md
::qr https://example.test/order/1042
::image A1
::nv-image A1
::row image:A1 qr:https://example.test/order/1042
```

`::row image:A1 qr:...` composes a local copy of the stored-image artwork and QR
code into one raster row, then prints it with `GS v 0`. The image key must have
a matching local asset under `pkg/escpos/assets/`, such as `A1.png`.

Supported barcode directives:

```md
::barcode upca 042100005264
::barcode upce 01234565
::barcode ean13 5901234123457
::barcode ean8 96385074
::barcode code39 1042
::barcode itf 12345678
::barcode codabar A12345B
::barcode code93 CODE93-42
::barcode code128 ORDER-1042
```

Barcode payload rules are enforced mostly by the printer, not by the markdown
parser. Use valid data for the barcode symbology you choose.

## CLI

Inspect commands:

```sh
go run . --help
go run . print --help
go run . serve --help
```

Print a textual ESC/POS file over Ethernet/raw TCP:

```sh
go run . print 192.168.86.22:9100 --file examples/simple.pos
```

Print from stdin:

```sh
go run . print 192.168.86.22:9100 --file - < examples/simple.pos
```

Convert an image into the project's textual ESC/POS format:

```sh
go run . convert examples/aruna.png > /tmp/aruna.pos
```

Discover USB devices visible to the system CUPS USB backend:

```sh
go run . devices
```

Print over USB using a raw `usb://...` device URI:

```sh
go run . print 'usb://EPSON/...' --transport usb --file examples/simple.pos
```

USB mode writes ESC/POS bytes directly through the CUPS USB backend.

## Raspberry Pi service

The Pi deployment uses the checked-in unit at `deploy/epson-pos.service`. It
binds the dashboard/API to `0.0.0.0:8080`, stores templates under
`/var/lib/epson-pos/templates`, and persists printer recovery state at
`/var/lib/epson-pos/printer_state.json`.

For a direct printer Ethernet cable, keep Wi-Fi as the Pi's normal LAN and use
`eth0` as a small private printer network. The checked-in files under
`deploy/network/` configure `eth0` as `10.77.0.1/24` and run dnsmasq DHCP with
the printer reserved at `10.77.0.85`. The systemd unit also passes
`--printer-mac b0:e8:92:fc:dd:26` so self-healing discovery cannot print to a
LaserJet or other non-Epson port `9100` device.

Install or refresh on the Pi:

```sh
go build -o epson-pos .
sudo install -D -m 0755 epson-pos /opt/epson-pos/epson-pos
sudo install -D -m 0644 deploy/epson-pos.service /etc/systemd/system/epson-pos.service
sudo systemctl daemon-reload
sudo systemctl enable --now epson-pos.service
```

Check the active target:

```sh
curl -sS http://127.0.0.1:8080/api/status
```

## trashcan service

The Debian server `trashcan` uses the deployment files under
`deploy/trashcan/`.

- Service name: `epson-pos.service`
- HTTP bind address: `0.0.0.0:9100`
- Runtime config: `/etc/epson-pos/epson-pos.env`
- Binary: `/opt/epson-pos/epson-pos`
- Templates: `/var/lib/epson-pos/templates`
- Printer state: `/var/lib/epson-pos/printer_state.json`
- Logs: `journalctl -u epson-pos.service`

The checked-in config currently uses `EPSON_POS_PRINTER=192.168.86.48:9100` and
`EPSON_POS_PRINTER_MAC=b0:e8:92:fc:dd:26`. If the Epson printer has a different
reachable host/port from `trashcan`, update `/etc/epson-pos/epson-pos.env` and
restart the service. Do not use another raw port `9100` device unless its MAC
matches the Epson MAC.

Useful commands on `trashcan`:

```sh
sudo systemctl status epson-pos.service
sudo journalctl -u epson-pos.service -f
sudo systemctl restart epson-pos.service
curl -sS http://127.0.0.1:9100/api/status
```

Update/deploy a new version:

```sh
cd ~/src/epson-pos
git pull --ff-only
go test ./...
go build -o epson-pos .
sudo install -o root -g root -m 0755 epson-pos /opt/epson-pos/epson-pos
sudo systemctl restart epson-pos.service
```

Full first-time install commands are documented in
`deploy/trashcan/README.md`.

## Architecture

- `pkg/commands`: Cobra CLI commands for `print`, `convert`, `devices`, and
  `serve`.
- `pkg/commands/serve.go`: local dashboard, HTTP API, embedded HTML/CSS/JS, and
  template file storage.
- `pkg/escpos/markdown.go`: markdown and printer directive compiler.
- `pkg/escpos/preview.go`: text preview renderer for dashboard receipts.
- `pkg/escpos/template.go`: template field discovery and placeholder rendering.
- `pkg/escpos/converter.go`: textual ESC/POS to raw printer byte conversion.
- `pkg/transport`: TCP and CUPS USB raw transport helpers.
- `templates/`: shared markdown receipt templates.

The print path is:

```text
markdown/template input
  -> MarkdownToPOS textual ESC/POS
  -> Convert raw ESC/POS bytes
  -> TCP socket or CUPS USB backend
  -> printer
```

The preview uses the same markdown compiler, then renders a best-effort
fixed-width receipt preview from the textual ESC/POS.

## Development

Run checks:

```sh
go test ./...
go vet ./...
```

When changing the dashboard UI, restart the server:

```sh
make restart
```

When adding markdown features, update both the compiler and preview behavior,
then add focused tests under `pkg/escpos/`.
