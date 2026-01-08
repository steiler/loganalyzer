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
*   **Field Exclusion:** Lengthy fields (`content`, `raw-request`, `updates`, `deletes`, `raw-response`, `response`, `explicit-deletes`, `updates-owner`, `deletes-owner`, `tree`, `data`) are excluded from the main table view to reduce visual noise and scrolling.

### 2. Drill-Down Capabilities
*   **Modal Inspection:** Clicking a log line opens a modal with the full, detailed log entry.
*   **Recursive Parsing:** Specific string fields containing JSON (e.g., `content`, `updates`) are automatically parsed and expanded into nested JSON objects.
*   **XML Pretty-Printing:** Fields containing XML strings (specifically `response`) are detected, formatted with indentation, and rendered across multiple lines for readability.
*   **Protobuf Decoding:** Fields named `leafVariant` that contain base64-encoded strings are aggregated and sent to the backend in a single batch request (`/api/decode/batch`) to be decoded into human-readable JSON.

### 3. Filtering and Navigation
*   **Datastore Filtering:** A dropdown allows filtering logs by `datastore-name`.
*   **Time Jump:** Users can jump to a specific timestamp (supporting `HH:MM:SS` and `H:MM` formats). The system locates the nearest log entry and updates the view.
*   **Bidirectional Infinite Scrolling:** The frontend supports scrolling both down (to load newer logs) and up (to load older logs). When jumping to a specific time, previous logs are pre-fetched to allow immediate upward scrolling.

### 4. Presentation
*   **Syntax Highlighting:** JSON in the modal is pretty-printed and syntax-highlighted (keys, strings, booleans, nulls).
*   **Formatting:** Newlines in JSON strings (specifically in `msg`, `content`, or XML responses) are respected and rendered on separate lines in the virtualized view.

## Technical Stack

*   **Backend:** Go (Golang)
    *   **Endpoints:** 
        *   `/api/logs`: Fetch log slices.
        *   `/api/offset`: Find file offset by timestamp.
        *   `/api/decode`: Single value decoding (legacy).
        *   `/api/decode/batch`: Batch decoding of multiple values.
        *   `/api/datastores`: List available datastores.
    *   **Dependencies:** `github.com/sdcio/sdc-protos` (for `sdcpb` decoding), `google.golang.org/protobuf`.
*   **Frontend:** Vanilla HTML, CSS, JavaScript (no external framework dependencies).
*   **Transport:** REST / JSON.

## Key Architectural Decisions & Rationale

### 1. Performance Strategy for Large Files
*   **Memory Efficiency:** The backend reads the log file into memory as raw strings (`[]string`) plus a lightweight metadata slice (`LogMeta` containing `Time` and `DatastoreName`) for filtering and seeking.
*   **On-Demand Parsing:** JSON unmarshalling occurs only for the specific slice of lines requested by the frontend pagination.
*   **Lazy Loading:** The frontend uses bidirectional infinite scrolling to fetch batches of logs.
*   **Frontend Virtualization (Modal):** The detail modal uses a **Flattened Virtual Scroll** strategy.
    *   Instead of recursive DOM creation (which blocks the main thread on large trees), the nested JSON is flattened into a linear array of lightweight descriptor objects (`flatNodes`).
    *   A virtual window renders only the usage nodes currently visible in the viewport.
    *   Large multiline strings (like XML dumps) are split into individual virtual rows to ensure consistent row heights and proper rendering.
*   **Batch Processing:** To solve network latency issues when decoding hundreds of `leafVariant` fields in large trees, the frontend aggregates all encoded values and sends a single **Batch Decode** request to the backend.

### 2. Robustness
*   **Corruption Handling:** The file loader explicitly skips the first line relative to the file reader to prevent `bufio.Scanner: token too long` errors or JSON parse errors caused by log rotation or truncation.
*   **Recursive Parsing:** The frontend attempts to parse nested JSON strings but fails gracefully (leaving the string as-is) if parsing fails.

### 3. Decoding Strategy
*   **Server-Side Decoding:** Protobuf decoding is offloaded to a backend endpoint (`/api/decode` and `/api/decode/batch`) to leverage the existing Go Protobuf definitions (`sdc-protos`) rather than rewriting proto logic in JavaScript.

## In-Scope vs Out-of-Scope

**In-Scope:**
*   Reading local log files.
*   Decoding specific field patterns (`leafVariant`).
*   Performance for files up to ~200MB+.
*   **Frontend Search:** Basic client-side search (filter text) for loaded logs and modal content. Regex support was removed to simplify the UX.

**Out-of-Scope:**
*   Real-time log tailing/streaming (file is loaded once at startup).
*   Persistent storage (database).
*   User authentication/authorization.
*   Editing or replaying logs.

## Open Questions / TODOs / Known Tradeoffs

*   **Memory Usage:** For extremely large files (GB+), the current `[]string` in-memory approach will exhaust RAM. A future improvement would be to index file offsets and seek on disk instead of loading all lines.
*   **Hardcoded Fields:** The list of fields to recurse into (`content`, `updates`, etc.) and exclude from the main view is hardcoded in the frontend. Configuration could be externalized.


Treat this file as the authoritative source of truth.
When we make decisions that change scope, requirements, architecture, or assumptions, explicitly tell me that PROJECT_CONTEXT.md should be updated and provide the updated version.