<!-- field:drawing_name hint="Name of the drawing or event" default="Beerocracy Prize Drawing" -->
<!-- field:winner_name hint="Winner name, handle, or ticket holder" default="Lucky Delegate" -->
<!-- field:prize hint="Prize awarded" default="Mystery Cooler Prize" -->
<!-- field:ticket_id hint="Winning ticket or claim number" default="DRAW-2040" -->
<!-- field:drawn_at hint="Date and time drawn" default="2026-06-25 20:26" -->
<!-- field:issuer hint="Person or station issuing the award" default="Prize Desk" -->
<!-- field:claim_by hint="Claim deadline or pickup instruction" default="Claim before closing ceremony" -->

# BEEROCRACY
## PRIZE DRAWING
#### Award Certificate

::line

Drawing | {{drawing_name}}
Winner | {{winner_name}}
Prize | {{prize}}
Ticket | {{ticket_id}}
Drawn | {{drawn_at}}
Issued by | {{issuer}}

::line

::center
Congratulations!
::left

This receipt confirms the random prize drawing award.
Present it at the prize desk to claim the listed prize.

Claim | {{claim_by}}

::line

::barcode code39 {{ticket_id}}

::feed 2
::cut partial 30
