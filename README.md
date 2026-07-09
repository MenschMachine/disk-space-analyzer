# dsa

`dsa` is a fast disk space analyzer CLI for macOS and Linux.

The first version reports directories only. A directory's size is based on apparent file size, not allocated disk blocks.

## Usage

```sh
go run ./cmd/dsa [flags] [path]
```

If `path` is omitted, `dsa` scans the current working directory.

## Flags

- `--format table|json`: output format. Defaults to `table`.
- `--limit N`: maximum number of directories to report. Defaults to `50`.
- `--size-mode recursive|top-level`: directory aggregation mode. Defaults to `recursive`.
- `--exclude GLOB`: exclude a path by glob pattern. May be repeated.
- `--cross-fs`: descend into directories on other filesystems. By default, `dsa` stays on the scanned root's filesystem.
- `--no-device-check`: skip directory device checks. This can be faster on slow filesystems and may cross filesystem boundaries.
- `--regular-files-only`: count only regular file entries. Symlinks and special files are ignored.
- `--stream`: continuously refresh the current top directories while scanning. Requires table output.
- `--workers N`: scanner worker count. Defaults to logical CPUs.
- `--version`: print the version and exit.

## Install

On Debian-based Linux systems, install the latest GitHub release with:

```sh
curl -fsSL https://raw.githubusercontent.com/MenschMachine/disk-space-analyzer/main/install.sh | bash
```

The installer downloads the latest `.deb`, verifies it against the release `checksums.txt`, and installs it with `apt-get`.

## Size Modes

- `recursive`: a directory's size is the inclusive sum of all non-excluded file entries under that directory.
- `top-level`: a directory's size is the sum of only direct non-excluded file entries in that directory.

Symlinks are not followed. A symlink entry is counted by the symlink entry's apparent size.

With `--regular-files-only`, only regular file entries are counted. Directory traversal still occurs, but symlink entries and special files do not contribute to directory sizes.

## Errors

Permission and read errors do not stop the scan. JSON output includes them in the top-level `errors` array. Table output prints a warning count when scan errors occurred.
