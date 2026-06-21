# epson-pos
Experiment with Epson POS Printer.

## CLI

Build and inspect the CLI:

```sh
go run . --help
```

Convert an image into the project's text ESC/POS format:

```sh
go run . convert examples/aruna.png > /tmp/aruna.pos
```

Print over Ethernet/raw TCP:

```sh
go run . print 192.168.86.22:9100 --file examples/simple.pos
```

Run the local dashboard for editing, previewing, and printing:

```sh
make
```

Templates are stored as markdown files in `templates/` by default. To use a
different folder:

```sh
make TEMPLATES_DIR=/path/to/templates
```

Template inputs are declared with markdown comments and used with placeholders:

```md
<!-- field:order_id hint="Order number or ticket id" default="1042" -->

# Lantern Market
Order | {{order_id}}
::barcode code39 {{order_id}}
```

Stop the dashboard:

```sh
make stop
```

## Markdown receipts

The dashboard editor accepts markdown plus printer directives:

```md
# 3x3 centered heading
## 2x2 centered heading
### 2x1 centered heading
#### bold centered heading
**bold text**
Item | Price
- Bullet item

::barcode code39 1042
::barcode code128 ORDER-1042
::qr https://example.test/1042
::align left
::align center
::align right
::size 1x1
::size 2x2
::line
::feed 2
::cut
```

Barcode and QR directives compile to native ESC/POS commands before printing.

Discover USB devices visible to the system CUPS USB backend:

```sh
go run . devices
```

Print over USB using a raw `usb://...` device URI:

```sh
go run . print 'usb://EPSON/...' --transport usb --file examples/simple.pos
```

USB mode writes ESC/POS bytes directly through the CUPS USB backend and does
not require configuring the printer as a normal document printer queue.
