<!-- field:ticket_id hint="Parking ticket number" default="PK-2040" -->
<!-- field:plate hint="Vehicle plate or camp cart ID" default="BEER-042" -->
<!-- field:space hint="Parking space, camp, or zone" default="Tapline Zone" -->
<!-- field:issued_at hint="Issue date and time" default="2026-06-24 20:26" -->
<!-- field:officer hint="Issuer name or badge" default="Officer Barley" -->
<!-- field:violation hint="Reason for ticket" default="Parked outside the democratic process" -->
<!-- field:fine hint="Fine, fee, or required action" default="One vote and one cold beverage" -->
<!-- field:due hint="Due date or deadline" default="Before last call" -->

# BEEROCRACY
## PARKING TICKET
#### Notice {{ticket_id}}

::line

Ticket | {{ticket_id}}
Plate | {{plate}}
Space | {{space}}
Issued | {{issued_at}}
Officer | {{officer}}

::line

### Violation
{{violation}}

### Fine
{{fine}}

Due | {{due}}

::line

::center
Retain this ticket for appeal,
payment, or ceremonial debate.
::left

::barcode code39 {{ticket_id}}

::line

#### Beerocracy Info
::qr https://wiki.toorcamp.org/Beerocracy
::feed 2
::cut partial 30
