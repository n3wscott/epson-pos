<!-- field:order_id hint="Order number or ticket id" default="1042" -->
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
