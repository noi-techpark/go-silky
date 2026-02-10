# Silky Binaries

This directory contains pre-built binaries for different platforms.

## Available Binaries

- **linux/amd64**: `silky-linux-amd64` (9.68 MB)
- **linux/arm64**: `silky-linux-arm64` (9.00 MB)
- **darwin/amd64**: `silky-darwin-amd64` (9.90 MB)
- **darwin/arm64**: `silky-darwin-arm64` (9.26 MB)
- **windows/amd64**: `silky-windows-amd64.exe` (9.96 MB)
- **windows/arm64**: `silky-windows-arm64.exe` (9.12 MB)

## Usage

The extension automatically selects the appropriate binary for your platform.

You can also run the binary directly from the command line:

```bash
./silky-<platform>-<arch> -config path/to/config.silky.yaml -profiler
```

### Flags

- `-config <path>`: Path to configuration file (required)
- `-profiler`: Enable profiler output (JSON per step)
- `-validate`: Only validate configuration without running

## Building from Source

To rebuild binaries:

```bash
npm run build:binary           # Build for current platform
npm run build:binary:all        # Build for all platforms
```
