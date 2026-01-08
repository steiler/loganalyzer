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
*   **Indicator Strip (Gutter):** Each log row has an interactive left margin (gutter).
    *   **"Interesting" Logs:** Displayed with a blue color for quick identification. Configured messages include "writeback synctree", "sync content", "synctree after sync apply", "received netconf response", "deviation tree", etc.
    *   **Ellipsis Indicator:** Interesting/expandable entries show an ellipsis (⋯) immediately after the log content text to signal additional data is available in the modal.
    *   **Marking:** Clicking the gutter (or any part of the strip) toggles a green background highlight (`marked-entry`) on the row for manual tracking.

### 2. Drill-Down Capabilities
*   **Modal Inspection:** Clicking a log line (with a slight debounce) opens a modal with the full, detailed log entry.
    *   **Prevent accidental opening:** Clicking does not open the modal if text is being selected.
    *   **Copy Behavior:** The modal "Copy" button copies the prettyfied, visually-formatted representation (built from flattened nodes) rather than raw JSON, matching what the user sees on screen.
*   **Recursive Parsing:** Specific string fields containing JSON (e.g., `content`, `updates`) are automatically parsed and expanded into nested JSON objects.
*   **XML Pretty-Printing:** Fields containing XML strings (specifically `response`) are detected, formatted with indentation, and rendered across multiple lines for readability. XML responses are excluded from the main overview list to reduce clutter.
*   **Protobuf Decoding:** Fields named `leafVariant` that contain base64-encoded strings are aggregated and sent to the backend in a single batch request (`/api/decode/batch`) to be decoded into human-readable JSON.

### 3. Filtering and Navigation
*   **Datastore Filtering:** A dropdown allows filtering logs by `datastore-name`.
*   **Time Jump:** Users can jump to a specific timestamp (supporting `HH:MM:SS` and `H:MM` formats). The system locates the nearest log entry and updates the view.
*   **Bidirectional Infinite Scrolling:** The frontend supports scrolling both down (to load newer logs) and up (to load older logs). When jumping to a specific time, previous logs are pre-fetched to allow immediate upward scrolling.
*   **Search Interaction:**
    *   **Double-Click:** Double-clicking text in a log entry automatically populates the search box with selected text and triggers highlighting. This action prevents the detail modal from opening.
    *   **Keyboard Shortcut:** Pressing `Ctrl+Shift+F` or `Alt+S` in the overview (when modal is closed) inserts the current text selection into the main search input and triggers highlighting. Does not interfere with typing in input fields.
    *   **Clear Search:** A dedicated "Clear" button and the ESC key (when no modal is open) reset the search filter.
    *   **ESC Key:** Closes the modal if open; otherwise clears search highlights.

### 4. Presentation
*   **Syntax Highlighting:** JSON in the modal is pretty-printed and syntax-highlighted (keys, strings, booleans, nulls).
*   **Formatting:** Newlines in JSON strings (specifically in `msg`, `content`, or XML responses) are respected and rendered on separate lines in the virtualized view.
*   **Modal Search:** In-modal search with next/prev navigation, match counters, and highlighting. Enter/Shift+Enter navigate matches.
*   **Collapse/Expand:** Modal content supports collapsing and expanding nested objects and arrays via gutter +/- buttons for easier navigation of large structures.

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
*   **Search/Highlighting:** Client-side search for loaded logs (filter text). Supports quick search via text selection (double-click) and manual clearing.

**Out-of-Scope:**
*   Real-time log tailing/streaming (file is loaded once at startup).
*   Persistent storage (database).
*   User authentication/authorization.
*   Editing or replaying logs.

## Open Questions / TODOs / Known Tradeoffs

*   **Memory Usage:** For extremely large files (GB+), the current `[]string` in-memory approach will exhaust RAM. A future improvement would be to index file offsets and seek on disk instead of loading all lines.
*   **Hardcoded Fields:** The list of fields to recurse into (`content`, `updates`, etc.), exclude from the main view, and "interesting" message patterns are hardcoded in the frontend. Configuration could be externalized.
*   **Ellipsis Positioning:** The ellipsis indicator is rendered via CSS `::after` pseudo-element on the content span, ensuring it appears immediately after text rather than at the row's right edge.

## Recent UX Improvements (Session Notes)

*   **Selection-to-Search Workflow:** Users can now quickly search for text by double-clicking (fills search box) or using `Ctrl+Shift+F`/`Alt+S` keyboard shortcuts when text is selected.
*   **Visual Consistency:** Ellipsis indicators for expandable entries now appear directly after content text, improving scannability.
*   **Copy Fidelity:** Modal copy outputs the formatted, indented view that matches the on-screen presentation rather than raw JSON.
*   **Keyboard Navigation:** ESC key intelligently closes modal or clears search depending on context; Enter/Shift+Enter navigate modal search results.


Treat this file as the authoritative source of truth.
When we make decisions that change scope, requirements, architecture, or assumptions, explicitly tell me that PROJECT_CONTEXT.md should be updated and provide the updated version.