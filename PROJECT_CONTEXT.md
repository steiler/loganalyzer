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
*   **jsonVal Decoding:** Fields named `jsonVal` (and variants `json_val`, `json-val`) that contain base64-encoded JSON are decoded on the frontend; if the decoded content is valid JSON it is parsed and rendered as structured data under `jsonVal_decoded`, otherwise shown as a UTF-8 string. This ensures pretty-printed display in the modal.

### 3. Filtering and Navigation
*   **Datastore Filtering:** A dropdown allows filtering logs by `datastore-name`.
*   **Time Jump:** Users can jump to a specific timestamp (supporting `HH:MM:SS` and `H:MM` formats). The system locates the nearest log entry and updates the view.
*   **Bidirectional Infinite Scrolling:** The frontend supports scrolling both down (to load newer logs) and up (to load older logs). When jumping to a specific time, previous logs are pre-fetched to allow immediate upward scrolling.
*   **Server-Side Search:** Search queries are executed on the backend, which scans all log lines against the query (case-insensitive substring matching) and returns global match indices relative to the filtered dataset. This enables searching across logs not yet loaded on the frontend.
*   **Search Navigation with Gap-Filling:**
    *   When user navigates to a match position that is not currently visible in the viewport, the frontend loads logs around that position to fill the gap.
    *   The system maintains continuity by loading sufficient context (buffers above and below) around the target match.
    *   Prev/next navigation wraps around at boundaries.
*   **Search Interaction:**
    *   **Double-Click:** Double-clicking text in a log entry automatically populates the search box with selected text and triggers server-side search. This action prevents the detail modal from opening.
    *   **Keyboard Shortcut:** Pressing `Ctrl+Shift+F` or `Alt+S` in the overview (when modal is closed) inserts the current text selection into the main search input and triggers server-side search. Does not interfere with typing in input fields.
    *   **Clear Search:** A dedicated "Clear" button and the ESC key (when no modal is open) reset the search filter.
    *   **ESC Key:** Closes the modal if open; otherwise clears search highlights.

### 4. Presentation
*   **Syntax Highlighting:** JSON in the modal is pretty-printed and syntax-highlighted (keys, strings, booleans, nulls).
*   **Formatting:** Newlines in JSON strings (specifically in `msg`, `content`, or XML responses) are respected and rendered on separate lines in the virtualized view.
*   **Overview Search:** Client-side search in the overview displays a total match count (e.g., "N matches") next to the search input. Users explicitly navigate between matches using prev/next buttons, Enter key (next), or Shift+Enter (previous). No automatic jump to first match; user initiates navigation.
*   **Modal Search:** In-modal search with next/prev navigation, match counters, and highlighting. Enter/Shift+Enter navigate matches.
*   **Collapse/Expand:** Modal content supports collapsing and expanding nested objects and arrays via gutter +/- buttons for easier navigation of large structures.
*   **Horizontal Scroll:** The modal preserves long-line formatting and enables horizontal scrolling for overflow content to avoid line wrapping and clipping.

## Technical Stack

*   **Backend:** Go (Golang)
    *   **Endpoints:** 
        *   `/api/logs`: Fetch log slices.
        *   `/api/offset`: Find file offset by timestamp.
        *   `/api/search`: Server-side search across all logs, respecting datastore filter; returns array of match indices in filtered view and total count.
        *   `/api/decode`: Single value decoding (legacy).
        *   `/api/decode/batch`: Batch decoding of multiple values.
        *   `/api/datastores`: List available datastores.
    *   **Dependencies:** `github.com/sdcio/sdc-protos` (for `sdcpb` decoding), `google.golang.org/protobuf`.
*   **Frontend:** Vanilla HTML, CSS, JavaScript (no external framework dependencies).
*   **Transport:** REST / JSON.

## Key Architectural Decisions & Rationale

### 4. Performance Strategy for Large Files
*   **Memory Efficiency:** The backend reads the log file into memory as raw strings (`[]string`) plus a lightweight metadata slice (`LogMeta` containing `Time` and `DatastoreName`) for filtering and seeking.
*   **On-Demand Parsing:** JSON unmarshalling occurs only for the specific slice of lines requested by the frontend pagination.
*   **Lazy Loading:** The frontend uses bidirectional infinite scrolling to fetch batches of logs with the following refinements:
    *   **Smooth Scroll Maintenance:** When prepending logs (loading upward), scroll position is preserved by tracking a reference element's visual position on screen before and after rendering, ensuring the user's viewing context remains intact.
    *   **Debounced Scroll Events:** Scroll event listeners use a 50ms debounce to prevent rapid, redundant load requests during fast scrolling.
    *   **Reference-Based Positioning:** Instead of relying on height calculations which can be unreliable, the frontend uses `getBoundingClientRect()` to track visual position and `scrollBy()` to maintain it, providing smoother and more predictable scrolling.
*   **Server-Side Search Efficiency:** Search scans the raw strings in memory, avoiding JSON parsing overhead until results are needed. Returns match indices for efficient gap-filling.
*   **Frontend Virtualization (Modal):** The detail modal uses a **Flattened Virtual Scroll** strategy.
    *   Instead of recursive DOM creation (which blocks the main thread on large trees), the nested JSON is flattened into a linear array of lightweight descriptor objects (`flatNodes`).
    *   A virtual window renders only the usage nodes currently visible in the viewport.
    *   Large multiline strings (like XML dumps) are split into individual virtual rows to ensure consistent row heights and proper rendering.
*   **Batch Processing:** To solve network latency issues when decoding hundreds of `leafVariant` fields in large trees, the frontend aggregates all encoded values and sends a single **Batch Decode** request to the backend.

### 2. Robustness
*   **Corruption Handling:** The file loader explicitly skips the first line relative to the file reader to prevent `bufio.Scanner: token too long` errors or JSON parse errors caused by log rotation or truncation.
*   **Recursive Parsing:** The frontend attempts to parse nested JSON strings but fails gracefully (leaving the string as-is) if parsing fails.
*   **Route Registration Order:** API handlers are registered before the catch-all file server to ensure `/api/*` requests reach their handlers instead of being intercepted by the static file server.
*   **Empty Result Handling:** The backend initializes result slices as empty arrays (`make([]LogLine, 0)`) rather than nil to ensure JSON encoding produces `[]` instead of `null`, preventing frontend errors when no logs match a query.

### 3. Decoding Strategy
*   **Server-Side Decoding:** Protobuf decoding is offloaded to a backend endpoint (`/api/decode` and `/api/decode/batch`) to leverage the existing Go Protobuf definitions (`sdc-protos`) rather than rewriting proto logic in JavaScript.
*   **Client-Side Decoding (jsonVal):** Base64 JSON in `jsonVal` fields is decoded in the browser for immediacy and to avoid server calls; decoded objects render via the modal's virtualized pretty view.

## In-Scope vs Out-of-Scope

**In-Scope:**
*   Reading local log files.
*   Decoding specific field patterns (`leafVariant`).
*   Performance for files up to ~200MB+.
*   **Search:** Server-side search for all logs with case-insensitive substring matching. Gap-filling when navigating to off-screen matches. Efficient handling of large datasets via index-based navigation.

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

*   **Selection-to-Search Workflow:** Users can now quickly search for text by double-clicking (fills search box) or using `Ctrl+Shift+F`/`Alt+S` keyboard shortcuts when text is selected in the overview.
*   **Visual Consistency:** Ellipsis indicators (⋯) for expandable entries now appear directly after content text (via CSS pseudo-element on content span), improving scannability and positioning at the right point in the line.
*   **jsonVal Decoding:** Base64-encoded JSON in `jsonVal` fields is now decoded client-side, parsed, and displayed as structured data under `jsonVal_decoded` in the modal for immediate visibility and pretty-printing.
*   **Modal Horizontal Scrolling:** Long lines in the modal now support horizontal scrolling instead of wrapping, preserving formatting and enabling inspection of complete values.
*   **Overview Search Match Counter:** The overview search box displays a live count of total matches across the entire filtered dataset (e.g., "42 matches"), computed server-side. When actively navigating, it shows "N of M" to indicate position in the result set.
*   **Search Navigation with Gap-Filling:** Users can explicitly navigate between matches using prev/next buttons or Enter/Shift+Enter. When a match is not currently visible, the frontend automatically loads logs between the current view and the target match, filling the gap seamlessly to maintain a continuous scrollable view.
    *   **Buttons:** Prev/Next buttons toggle between highlighted matches.
    *   **Keyboard:** Enter key jumps to next match; Shift+Enter jumps to previous match.
    *   **Behavior:** Active match is highlighted and scrolled into view; navigation wraps around at boundaries.
*   **Copy Fidelity:** Modal copy outputs the formatted, indented view that matches the on-screen presentation rather than raw JSON.
*   **Keyboard Navigation:** ESC key intelligently closes modal or clears search depending on context; Enter/Shift+Enter navigate overview search results or modal search results.
*   **Multi-Platform Releases:** GoReleaser configuration (`goreleaser.yaml`) enables automated builds and releases for Windows 64-bit, Linux (amd64 and arm64), and macOS (amd64 and arm64) with appropriate archive formats (.zip for Windows, .tar.gz for others).


Treat this file as the authoritative source of truth.
When we make decisions that change scope, requirements, architecture, or assumptions, explicitly tell me that PROJECT_CONTEXT.md should be updated and provide the updated version.