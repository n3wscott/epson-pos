# Pi Direct Printer Network

The Raspberry Pi keeps Wi-Fi as its normal LAN connection and uses `eth0` only
for the direct Epson printer cable.

- Pi `eth0`: `10.77.0.1/24`
- DHCP range: `10.77.0.50` through `10.77.0.100`
- Epson printer reservation: `10.77.0.85`
- Raw ESC/POS port: `9100`

Install the DHCP config:

```sh
sudo install -D -m 0644 deploy/network/epson-printer-dhcp.conf /etc/dnsmasq.d/epson-printer-dhcp.conf
sudo systemctl enable --now dnsmasq
```

Install or update the NetworkManager profile:

```sh
sudo install -D -m 0600 deploy/network/eth0-printer-network.nmconnection /etc/NetworkManager/system-connections/epson-printer-eth0.nmconnection
sudo nmcli connection reload
sudo nmcli connection up epson-printer-eth0
```

Keep backup files out of `/etc/dnsmasq.d`; dnsmasq reads every file in that
directory.
