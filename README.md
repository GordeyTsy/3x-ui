# 3X-UI — Tor upstream fork

This repository continues the work of the original [3X-UI project](https://github.com/MHSanaei/3x-ui) while focusing on seamless Tor integration for inbound proxies. It keeps the familiar management panel but extends it with tooling to launch dedicated Tor circuits for every client that connects through supported protocols.

## Highlights

- Per-client Tor upstream option for VLESS inbounds with automatic circuit lifecycle management.
- Shared proxy settings model that keeps Tor credentials in sync across CRUD operations and Xray config generation.
- Installer that compiles the panel from source and pulls the latest compatible Xray-core release automatically.

## Installation

Run the installer as root. It installs required packages, builds the panel, and wires up the service units:

```bash
bash <(curl -Ls https://raw.githubusercontent.com/GordeyTsy/3x-ui/master/install.sh)
```

The script expects a working Go toolchain. On most Linux distributions it is installed as part of the dependency step; if it is missing, install Go manually and re-run the script.

## Manual build

```bash
go build -trimpath -ldflags "-s -w" -o build/x-ui main.go
```

Copy the resulting binary alongside the `config`, `database`, `logger`, `media`, `sub`, `tor`, `util`, `web`, `windows_files`, and `xray` directories before launching it.

## Credits

- Original 3X-UI by [MHSanaei](https://github.com/MHSanaei/3x-ui)
- Xray-core by the [XTLS](https://github.com/XTLS) maintainers
- Community contributors adding Tor tooling and quality-of-life improvements
