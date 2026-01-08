# Project Context: Log Analyzer

## Project Goal and Scope
**Goal:** Develop a high-performance Golang web application for analyzing structured application logs. The tool is designed to assist developers in inspecting complex transaction logs, specifically those containing nested JSON structures and Protocol Buffer encoded values.

**Scope:**
*   A standalone web server providing a read-only view of a log file.
*   Interactive frontend for browsing, filtering, and drilling down into complex log entries.
*   Specific support for decoding `sdcpb.TypedValue` (from `github.com/sdcio/sdc-protos`) used in the logs.

## Core Features and Requirements

### 1. Log Visualization
*   **Scrollable View:** Display logs in a dense, non-wrapping list.
*   **Column Ordering:** Priority fields must be valid and shown first: `time`, `msg`, `datastore-name`, `logger`, `transaction-id`.
*   **Field Exclusion:** Lengthy fields (`content`, `raw-request`, `updates`, `deletes`, `raw-response`, `explicit-deletes`, `updates-owner`, `deletes-owner`, `tree`) are excluded from the main table view to reduce visual noise and scrolling.

### 2. Drill-Down Capabilities
*   **Modal Inspection:** Clicking a log line opens a modal with the full, detailed log entry.
*   **Recursive Parsing:** Specific string fields containing JSON (e.g., `content`, `updates`) are automatically parsed and expanded into nested JSON objects.
*   **Protobuf Decoding:** Fields named `leafVariant` that contain base64-encoded strings are sent to the backend to be decoded into human-readable JSON using `sdc-protos`.

### 3. Filtering and Navigation
*   **Datastore Filtering:** A dropdown allows filtering logs by `datastore-name`.
*   **Infinite Scrolling:** The frontend loads data in chunks to handle large datasets effectively.

### 4. Presentation
*   **Syntax Highlighting:** JSON in the modal is pretty-printed and syntax-highlighted (keys, strings, booleans, nulls).
*   **Formatting:** Newlines in JSON strings (specifically in `msg` or `content`) are rendered effectively in the modal.

## Technical Stack

*   **Backend:** Go (Golang)
    *   **Dependencies:** `github.com/sdcio/sdc-protos` (for `sdcpb` decoding), `google.golang.org/protobuf`.
*   **Frontend:** Vanilla HTML, CSS, JavaScript (no external framework dependencies).
*   **Transport:** REST / JSON.

## Key Architectural Decisions & Rationale

### 1. Performance Strategy for Large Files
*   **Memory Efficiency:** The backend reads the log file into memory as raw strings (`[]string`) plus a lightweight metadata slice for filtering. It does *not* unmarshal the entire JSON content of every line at startup. This reduces memory pressure and GC overhead.
*   **On-Demand Parsing:** JSON unmarshalling occurs only for the specific slice of lines requested by the frontend pagination.
*   **Lazy Loading:** The frontend uses infinite scrolling (triggered by scroll position) to fetch batches of logs (initially 100, then 50 at a time).

### 2. Robustness
*   **Corruption Handling:** The file loader explicitly skips the first line relative to the file reader to prevent `bufio.Scanner: token too long` errors or JSON parse errors caused by log rotation or truncation.
*   **Recursive Parsing:** The frontend attempts to parse nested JSON strings but fails gracefully (leaving the string as-is) if parsing fails.

### 3. Decoding Strategy
*   **Server-Side Decoding:** Protobuf decoding is offloaded to a backend endpoint (`/api/decode`) to leverage the existing Go Protobuf definitions (`sdc-protos`) rather than rewriting proto logic in JavaScript.

## In-Scope vs Out-of-Scope

**In-Scope:**
*   Reading local log files.
*   Decoding specific field patterns (`leafVariant`).
*   Performance for files up to ~200MB+.

**Out-of-Scope:**
*   Real-time log tailing/streaming (file is loaded once at startup).
*   Persistent storage (database).
*   User authentication/authorization.
*   Editing or replaying logs.

## Open Questions / TODOs / Known Tradeoffs

*   **Memory Usage:** For extremely large files (GB+), the current `[]string` in-memory approach will exhaust RAM. A future improvement would be to index file offsets and seek on disk instead of loading all lines.
*   **Search:** Text search across the message body is not implemented; currently only filtering by `datastore-name` is supported.
*   **Hardcoded Fields:** The list of fields to recurse into (`content`, `updates`, etc.) and exclude from the main view is hardcoded in the frontend. Configuration could be externalized.
