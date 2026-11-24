# Installation

## Install script

macOS, Linux and FreeBSD:

```sh
curl -fsSL https://raw.githubusercontent.com/rafalmaciejewski/ctlctl/main/install.sh | sh
```

Windows (PowerShell):

```powershell
irm https://raw.githubusercontent.com/rafalmaciejewski/ctlctl/main/install.ps1 | iex
```

The scripts download the archive for your platform from the
[latest release](https://github.com/rafalmaciejewski/ctlctl/releases/latest),
check it against the release's SHA-256 checksums and install the binary. On
Windows, the script also adds the install directory to your user `PATH`.

These environment variables change what the scripts do:

| Variable             | Default                                                    | Meaning                                  |
| -------------------- | ---------------------------------------------------------- | ---------------------------------------- |
| `CTLCTL_INSTALL_DIR` | `~/.local/bin` (Windows: `%LOCALAPPDATA%\Programs\ctlctl`) | Where to put the binary                  |
| `CTLCTL_VERSION`     | the latest release                                         | The version to install, such as `1.2.0`  |
| `CTLCTL_BASE_URL`    | GitHub Releases                                            | A mirror that serves the release assets  |

For example, to install version 1.2.0 to `/usr/local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/rafalmaciejewski/ctlctl/main/install.sh | CTLCTL_INSTALL_DIR=/usr/local/bin CTLCTL_VERSION=1.2.0 sh
```

To update, run the script again.

## Go

With Go 1.27 or later:

```sh
go install github.com/rafalmaciejewski/ctlctl@latest
```

## Manual download

Each [release](https://github.com/rafalmaciejewski/ctlctl/releases) has an
archive for each platform, plus `checksums.txt`:

| Platform              | Archive                     |
| --------------------- | --------------------------- |
| macOS (Apple silicon) | `ctlctl_darwin_arm64.tar.gz` |
| macOS (Intel)         | `ctlctl_darwin_amd64.tar.gz` |
| Linux (x86-64)        | `ctlctl_linux_amd64.tar.gz`  |
| Linux (ARM64)         | `ctlctl_linux_arm64.tar.gz`  |
| Linux (ARMv7)         | `ctlctl_linux_armv7.tar.gz`  |
| Windows (x86-64)      | `ctlctl_windows_amd64.zip`   |
| Windows (ARM64)       | `ctlctl_windows_arm64.zip`   |
| FreeBSD (x86-64)      | `ctlctl_freebsd_amd64.tar.gz` |
| FreeBSD (ARM64)       | `ctlctl_freebsd_arm64.tar.gz` |

Extract `ctlctl` (or `ctlctl.exe`) and put it in a directory on your `PATH`.

## Uninstall

Delete the binary. ctlctl also keeps its command history in
`~/.local/state/ctlctl/history` (Windows: `%LOCALAPPDATA%\ctlctl\history`) and
may have a config file in `~/.config/ctlctl/` (Windows: `%APPDATA%\ctlctl\`).
