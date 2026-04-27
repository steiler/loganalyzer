# Log Analyzer

A high-performance web application for analyzing and inspecting structured application logs. Built with Go and vanilla JavaScript, Log Analyzer provides an interactive interface for browsing complex transaction logs with nested JSON structures and Protocol Buffer encoded values.

This tool is specifically intended for analyzing logs from the `data-server` component of the `sdc` project.

## Features

### Core Capabilities
- **Dense Log Visualization** – View logs in a scrollable, non-wrapping list with optimized columns
- **Real-Time Log Tailing** – Follow live log files with efficient offset-based polling (`-follow` flag)
- **Advanced Search** – Server-side full-text search with client-side multi-term highlighting
- **Drill-Down Details** – Modal inspection of individual log entries with full structure visibility
- **Recursive Parsing** – Automatic detection and expansion of nested JSON and XML content
- **Protobuf Decoding** – Batch decoding of `sdcpb.TypedValue` fields from `sdc-protos` library
- **Filtering & Navigation** – Jump to timestamps, filter by datastore, and bidirectional infinite scrolling
- **State Persistence** – Auto-save highlights, marked rows, scroll position, and filters to `.viewstate.json`

### Presentation
- **Syntax Highlighting** – Pretty-printed JSON with color-coded keys and values
- **Horizontal Scrolling** – Long lines remain readable without wrapping
- **Collapsible Structure** – Expand/collapse nested objects and arrays in the detail modal
- **Visual Indicators** – Blue "interesting" log markers, ellipsis indicators, and new-row flash cues

## Installation

### Requirements
- A structured log file (JSON lines format supported)

### Install from GitHub Releases (Primary)

```bash
curl -fsSL https://raw.githubusercontent.com/steiler/loganalyzer/main/install.sh | bash
```

This installs `loganalyzer` into your PATH (default: `/usr/local/bin`).

### Build from Source (Alternative)

Requirements for source builds:
- Go 1.19 or later

```bash
git clone https://github.com/steiler/loganalyzer.git
cd loganalyzer
go mod tidy
go build -o loganalyzer
sudo install -m 0755 loganalyzer /usr/local/bin/loganalyzer
```

## Usage

### Basic Command

```bash
loganalyzer serve <path-to-log-file>
```

You can also run the default root command without `serve`:

```bash
loganalyzer <path-to-log-file>
```

### Options

| Flag | Default | Description |
|------|---------|-------------|
| `--file` | (required unless positional arg used) | Path to the log file to analyze |
| `--port` | `8080` | HTTP server port |
| `--follow` | `false` | Enable real-time log tailing (polls every 5 seconds) |

### Examples

```bash
# Basic usage – open log file
loganalyzer app.log

# Listen on custom port
loganalyzer serve app.log --port 3000

# Follow live log file (e.g., for streaming logs)
loganalyzer --file app.log --follow

# Print build version info
loganalyzer version
```

### Analyze a Running data-server Instance (Kubernetes)

You can stream `data-server` logs from Kubernetes into a local file and analyze them live with Log Analyzer.

```bash
# Terminal 1: stream all current+new logs into a local file
kubectl logs -n sdc-system statefulsets/data-server-controller data-server -f > data-server.log

# Terminal 2: analyze the same file in follow mode
loganalyzer --file data-server.log --follow
```

For long-running instances, start from a recent window instead of the full log history:

```bash
# Terminal 1: stream last 50 lines, then continue following
kubectl logs -n sdc-system statefulsets/data-server-controller data-server --tail 50 -f > data-server.log

# Terminal 2: analyze live updates
loganalyzer --file data-server.log --follow
```

Then open your browser to `http://localhost:8080`.

## Keyboard Shortcuts

### Overview Mode
| Shortcut | Action |
|----------|--------|
| `Ctrl+Shift+F` / `Alt+S` | Add current text selection as highlight term |
| `Enter` | Navigate to next search match |
| `Shift+Enter` | Navigate to previous search match |
| `Esc` | Close modal or clear search highlights |
| `?` | Open help/usage modal |

### Modal (Detail View)
| Shortcut | Action |
|----------|--------|
| `Enter` / `Shift+Enter` | Navigate modal search matches |
| `Esc` | Close modal |

## UI Guide

### Main View
- **Gutter (Left Margin)** – Click to mark/unmark rows; blue indicates "interesting" logs
- **Datastore Dropdown** – Filter logs by `datastore-name` field
- **Search Box** – Enter search terms (case-insensitive substring matching)
- **Highlight Input** – Add multi-term client-side highlights with custom colors
- **Time Jump** – Jump to specific timestamp (`HH:MM:SS` or `H:MM` format)
- **Latest Button** – Jump to the newest log entries

### Detail Modal
- **Copy Button** – Copy formatted (not raw JSON) view to clipboard
- **Collapse/Expand** – Toggle nested structures with +/- buttons
- **Search** – Find text within the current entry
- **Horizontal Scroll** – Scroll long lines without wrapping

## Architecture

### Backend (Go)
- **In-Memory Indexing** – Log lines stored as `[]string` with lightweight metadata for O(1) filtering
- **On-Demand Parsing** – JSON unmarshalling only for requested slices
- **Server-Side Search** – Scans raw strings to avoid parsing overhead
- **Protobuf Decoding** – Batch endpoint for efficient decoding of multiple encoded values
- **Offset-Based Tailing** – Real-time log following via file offset tracking

#### API Endpoints
- `GET /api/logs` – Fetch paginated log slice
- `GET /api/offset` – Find file offset by timestamp
- `GET /api/search` – Server-side search (returns match indices)
- `POST /api/decode/batch` – Batch decode Protobuf values
- `GET /api/datastores` – List available datastore filters

### Frontend (Vanilla JavaScript)
- **Virtualized Modal** – Flattened virtual scroll for large nested structures
- **Bidirectional Infinite Scroll** – Load logs forward and backward seamlessly
- **Gap-Filling** – Automatic loading to fill gaps when navigating search results
- **Real-Time Polling** – Background polling (5-second interval) for live log updates
- **View State Persistence** – localStorage-backed auto-save to `.viewstate.json`

## Performance Characteristics

| Metric | Capability |
|--------|-----------|
| File Size | Up to ~200MB+ (limited by available RAM) |
| Search Speed | Sub-second for most queries on typical log files |
| Scroll Latency | <50ms (debounced scroll events) |
| Batch Decode | Hundreds of values in single request |

## Tech Stack

- **Backend** – Go 1.19+
  - `github.com/sdcio/sdc-protos` (Protobuf definitions)
  - `google.golang.org/protobuf` (Protobuf runtime)
- **Frontend** – Vanilla HTML, CSS, JavaScript (no external frameworks)
- **Transport** – REST/JSON over HTTP

## View State Persistence

Log Analyzer automatically saves user interactions to a `.viewstate.json` file adjacent to your log file:

```
app.log
app.log.viewstate.json  ← Auto-generated
```

**Saved State Includes:**
- Highlight terms with assigned colors
- Manually marked rows (by global log index)
- Current scroll position (first visible entry)
- Active datastore filter
- Search filters

State is restored automatically on reload.

## Real-Time Log Following

When launched with `-follow`, the server polls the log file every 5 seconds:

- **Efficient** – Only new appended data is read (offset-based)
- **Resilient** – Handles log rotation and truncation automatically
- **Transparent** – Frontend seamlessly integrates new logs without interruption
- **Smart Scrolling** – Auto-follows end of file when near bottom; pauses when scrolled away

## Limitations & Future Improvements

- **Memory Usage** – For files >1GB, consider building a disk-based index instead of in-memory storage
- **Hardcoded Patterns** – Field names and "interesting" message patterns are currently hardcoded (could be externalized to config)
- **Authentication** – No built-in user auth/authorization (suitable for internal dev tools)
- **Editing** – Read-only view (logs cannot be modified)

## Building Releases

The project uses GoReleaser for automated multi-platform builds:

```bash
goreleaser release
```

**Supported Platforms:**
- Windows 64-bit (`.zip`)
- Linux amd64 & arm64 (`.tar.gz`)
- macOS amd64 & arm64 (`.tar.gz`)

See `goreleaser.yaml` for configuration.

## Contributing

Contributions are welcome! Please:
1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Submit a pull request

## License

See LICENSE file for details.

## Support

For issues, questions, or suggestions, please open an issue on GitHub.
