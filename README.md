# TunnelGate – SSH over WebSocket Gateway

**TunnelGate** is a production‑ready, fully automated SSH‑over‑WebSocket server that supports both plain HTTP (`ws://`) and TLS (`wss://`) on the same backend. It includes automatic certificate management, user accounts with expiry, an admin API, and a real‑time dashboard.

## Features

- **Dual‑protocol**: handles `ws://` and `wss://` with a single backend.
- **Zero‑touch TLS**: Let's Encrypt (HTTP‑01, DNS‑01 via Cloudflare) or Cloudflare Origin CA.
- **User management**: add, expire, lock, delete; stored in SQLite.
- **Admin API** + Web dashboard.
- **Prometheus metrics** for monitoring.
- **Systemd** supervision with auto‑restart.
- **One‑line install**: `curl -sSL https://get.tunnelgate.io | bash`.

## Quick Install

```bash
curl -sSL https://get.tunnelgate.io | bash
