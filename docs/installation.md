# Installation

Requirements:

- Linux or macOS
- Ansible (`ansible-core`) in your `$PATH`. playwatch needs no other Python dependency.
- Go 1.27 or later, to build from source

## Debian / Ubuntu

```bash
wget https://github.com/axel-tpt/playwatch/releases/latest/download/playwatch_linux_amd64.deb && sudo apt install ./playwatch_linux_amd64.deb && rm playwatch_linux_amd64.deb
```

Use `playwatch_linux_arm64.deb` on ARM. Running the same command again upgrades an existing installation. Ansible is recommended by the package but not installed for you.

## Other distributions and macOS

Each [release](https://github.com/axel-tpt/playwatch/releases) also provides `.rpm` and `.apk` packages (Linux) and `tar.gz` archives (Linux and macOS, amd64 and arm64) with a checksums file. For the archives, extract the binary and put it in your `$PATH`.

## From source

From a clone of the repository:

```bash
go install ./cmd/playwatch
```

This installs the `playwatch` binary in `$(go env GOPATH)/bin` (usually `~/go/bin`), which must be in your `$PATH`.

Or without cloning:

```bash
go install github.com/axel-tpt/playwatch/cmd/playwatch@latest
```
