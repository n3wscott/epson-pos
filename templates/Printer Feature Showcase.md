<!-- field:store_name hint="Receipt header or venue name" default="Lantern Market" -->
<!-- field:order_id hint="Order number for receipt text, CODE39, and CODE128" default="1042" -->
<!-- field:cashier hint="Cashier, station, or operator name" default="Sam" -->
<!-- field:qr_url hint="URL or text to encode as a QR code" default="https://lantern.test/orders/1042" -->
<!-- field:upca hint="UPC-A needs 11 or 12 numeric digits" default="042100005264" -->
<!-- field:upce hint="UPC-E usually uses 6 to 8 numeric digits" default="01234565" -->
<!-- field:ean13 hint="EAN-13/JAN-13 needs 12 or 13 numeric digits" default="5901234123457" -->
<!-- field:ean8 hint="EAN-8/JAN-8 needs 7 or 8 numeric digits" default="96385074" -->
<!-- field:itf hint="ITF needs an even number of numeric digits" default="12345678" -->
<!-- field:codabar hint="CODABAR starts and ends with A, B, C, or D" default="A12345B" -->
<!-- field:code93 hint="CODE93 text payload" default="CODE93-42" -->

# {{store_name}}
## Printer Feature Showcase
### Markdown + ESC/POS
#### Template order {{order_id}}

Order | {{order_id}}
Cashier | {{cashier}}
Mode | Ethernet raw 9100

::line

**Inline bold** and normal text on one line.
This line includes `inline code` and [a markdown link](https://example.test) rendered as plain text.

- Unordered list item
1. Ordered list item

Item | Qty | Price
--- | --- | ---
Latte | 1 | $4.50
Bagel | 1 | $3.25

::line

::center
Centered text
::right
Right aligned text
::left
Back to left aligned text

::bold on
Bold directive on
::bold off

::font a
Font A sample
::font b
Font B sample

::size 2x1
Wide text
::size 1x2
Tall text
::size 2x2
Big text
::size 1x1
Normal size again

::line
#### Native Stored Image Slot
::image A1

#### QR Code
::qr {{qr_url}}

#### Image + QR Row
::row image:A1 qr:{{qr_url}}

#### UPC-A
::barcode upca {{upca}}

#### UPC-E
::barcode upce {{upce}}

#### EAN-13 / JAN-13
::barcode ean13 {{ean13}}

#### EAN-8 / JAN-8
::barcode ean8 {{ean8}}

#### CODE39
::barcode code39 {{order_id}}

#### ITF
::barcode itf {{itf}}

#### CODABAR / NW-7
::barcode codabar {{codabar}}

#### CODE93
::barcode code93 {{code93}}

#### CODE128
::barcode code128 ORDER-{{order_id}}

::line
Feed two lines before cutting.
::feed 2
::cut partial 30
