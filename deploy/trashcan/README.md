# trashcan systemd deployment

`trashcan` runs the Epson POS dashboard/API as a normal systemd service.

- Service name: `epson-pos.service`
- Binary: `/opt/epson-pos/epson-pos`
- Config: `/etc/epson-pos/epson-pos.env`
- Templates: `/var/lib/epson-pos/templates`
- Printer state: `/var/lib/epson-pos/printer_state.json`
- Logs: `journalctl -u epson-pos.service`
- HTTP address: `0.0.0.0:9100`
- Status endpoint: `http://127.0.0.1:9100/api/status`

The current trashcan config uses `EPSON_POS_PRINTER=192.168.86.48:9100` and
`EPSON_POS_PRINTER_MAC=b0:e8:92:fc:dd:26` so the server does not select a
non-Epson port `9100` device. Update `/etc/epson-pos/epson-pos.env` if the
active Epson endpoint changes. On July 6, 2026, `trashcan` also saw raw-port
devices at `192.168.86.56:9100` and `192.168.86.246:9100`; do not use those
unless their MAC matches the Epson MAC.

Install or refresh from a checked-out repo on `trashcan`:

```sh
cd ~/src/epson-pos
git pull --ff-only
go test ./...
go build -o epson-pos .
sudo useradd --system --home /var/lib/epson-pos --shell /usr/sbin/nologin epson-pos 2>/dev/null || true
sudo install -d -o epson-pos -g epson-pos -m 0755 /var/lib/epson-pos /var/lib/epson-pos/templates
sudo install -d -o root -g root -m 0755 /opt/epson-pos /etc/epson-pos
sudo install -o root -g root -m 0755 epson-pos /opt/epson-pos/epson-pos
sudo install -o root -g root -m 0644 deploy/trashcan/epson-pos.env /etc/epson-pos/epson-pos.env
sudo install -o root -g root -m 0644 deploy/trashcan/epson-pos.service /etc/systemd/system/epson-pos.service
sudo cp templates/*.md /var/lib/epson-pos/templates/
sudo chown epson-pos:epson-pos /var/lib/epson-pos/templates/*.md
sudo systemctl daemon-reload
sudo systemctl enable --now epson-pos.service
```

Operations:

```sh
sudo systemctl status epson-pos.service
sudo journalctl -u epson-pos.service -f
sudo systemctl restart epson-pos.service
curl -sS http://127.0.0.1:9100/api/status
```

Deploy a new version:

```sh
cd ~/src/epson-pos
git pull --ff-only
go test ./...
go build -o epson-pos .
sudo install -o root -g root -m 0755 epson-pos /opt/epson-pos/epson-pos
sudo systemctl restart epson-pos.service
```
