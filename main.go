package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type LogLine map[string]interface{}

type LogMeta struct {
	DatastoreName string `json:"datastore-name"`
	Time          string `json:"time"`
}

var (
	version = "dev"
	commit  = "none"

	rawLines    []string
	metaData    []LogMeta
	logFile     string
	port        int
	stateFile   string
	followMode  bool
	logMutex    sync.RWMutex
	lastOffset  int64
	partialLine string
	generation  int // Incremented on file truncation/reload
)

type HighlightTerm struct {
	Term  string `json:"term"`
	Color string `json:"color"`
}

type ViewState struct {
	HighlightTerms []HighlightTerm `json:"highlightTerms"`
	MarkedIndices  []int           `json:"markedIndices"`
	Offset         int             `json:"offset"`
	Datastore      string          `json:"datastore"`
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		log.Fatal(err)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "loganalyzer [log-file]",
		Short: "Analyze structured logs in a browser",
		Long:  "Log Analyzer runs a local web server for browsing structured JSON logs.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(args)
		},
	}

	cmd.PersistentFlags().StringVar(&logFile, "file", "", "Path to the log file")
	cmd.PersistentFlags().BoolVar(&followMode, "follow", false, "Follow log file for new entries (like tail -f)")
	cmd.PersistentFlags().IntVar(&port, "port", 8080, "HTTP server port")

	serveCmd := &cobra.Command{
		Use:   "serve [log-file]",
		Short: "Start the log analyzer web server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(args)
		},
	}

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(buildVersionString())
		},
	}

	cmd.AddCommand(serveCmd, versionCmd)
	return cmd
}

func runServe(args []string) error {
	selectedLogFile, err := resolveLogFile(args)
	if err != nil {
		return err
	}

	logFile = selectedLogFile
	stateFile = filepath.Join(filepath.Dir(logFile), filepath.Base(logFile)+".viewstate.json")

	if err := loadFile(logFile); err != nil {
		return err
	}

	if followMode {
		go followLogFile(logFile)
		fmt.Println("Follow mode enabled - polling for new log entries every 5 seconds")
	}

	registerHandlers()

	addr := fmt.Sprintf(":%d", port)
	fmt.Printf("Starting server on %s with log file: %s (%d lines)\n", addr, logFile, len(rawLines))
	return http.ListenAndServe(addr, nil)
}

func resolveLogFile(args []string) (string, error) {
	if len(args) > 1 {
		return "", fmt.Errorf("expected at most one log-file argument, got %d", len(args))
	}

	if len(args) == 1 {
		if logFile != "" && logFile != args[0] {
			return "", fmt.Errorf("both --file and positional log-file provided with different values")
		}
		return args[0], nil
	}

	if logFile != "" {
		return logFile, nil
	}

	return "", fmt.Errorf("please provide a log file via --file flag or as argument")
}

func registerHandlers() {
	// Register API handlers first (more specific routes)
	http.HandleFunc("/api/logs", handleLogs)
	http.HandleFunc("/api/offset", handleFindOffset)
	http.HandleFunc("/api/search", handleSearch)
	http.HandleFunc("/api/decode", handleDecode)
	http.HandleFunc("/api/decode/batch", handleDecodeBatch)
	http.HandleFunc("/api/datastores", handleDatastores)
	http.HandleFunc("/api/viewstate", handleViewState)

	// Register file server last (catch-all)
	http.Handle("/", http.FileServer(http.Dir("./static")))
}

func buildVersionString() string {
	return fmt.Sprintf("loganalyzer version %s (commit %s)", version, commit)
}

func followLogFile(path string) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		file, err := os.Open(path)
		if err != nil {
			log.Printf("Error opening log file for following: %v", err)
			continue
		}

		info, err := file.Stat()
		if err != nil {
			_ = file.Close()
			log.Printf("Error stating log file: %v", err)
			continue
		}

		currentSize := info.Size()

		// Check for file truncation/rotation
		if currentSize < lastOffset {
			log.Println("Log file truncated or rotated, reloading from start")
			logMutex.Lock()
			_ = file.Close()
			if err := loadFile(path); err != nil {
				log.Printf("Error reloading file: %v", err)
			}
			generation++ // Signal truncation to frontend
			logMutex.Unlock()
			continue
		}

		// No new data
		if currentSize == lastOffset {
			_ = file.Close()
			continue
		}

		// Seek to last known position
		if _, err := file.Seek(lastOffset, io.SeekStart); err != nil {
			_ = file.Close()
			log.Printf("Error seeking in log file: %v", err)
			continue
		}

		scanner := bufio.NewScanner(file)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 5*1024*1024)

		newLines := make([]string, 0)
		newMeta := make([]LogMeta, 0)

		// Handle partial line from previous read
		firstLine := true
		for scanner.Scan() {
			text := scanner.Text()

			// If we had a partial line, prepend it to the first complete line
			if firstLine && partialLine != "" {
				text = partialLine + text
				partialLine = ""
			}
			firstLine = false

			newLines = append(newLines, text)

			var meta LogMeta
			_ = json.Unmarshal([]byte(text), &meta)
			newMeta = append(newMeta, meta)
		}

		// Check if the last line was incomplete (no newline at EOF)
		// We'll buffer it until next read
		currentPos, _ := file.Seek(0, io.SeekCurrent)
		if currentPos < currentSize {
			// There's more data, but scanner stopped (likely no newline)
			// The last "line" we got might be incomplete
			if len(newLines) > 0 {
				partialLine = newLines[len(newLines)-1]
				newLines = newLines[:len(newLines)-1]
				newMeta = newMeta[:len(newMeta)-1]
			}
		}

		lastOffset = currentPos
		_ = file.Close()

		if len(newLines) > 0 {
			logMutex.Lock()
			rawLines = append(rawLines, newLines...)
			metaData = append(metaData, newMeta...)
			logMutex.Unlock()
			log.Printf("Added %d new log lines (total: %d)", len(newLines), len(rawLines))
		}

		if err := scanner.Err(); err != nil {
			log.Printf("Error scanning log file: %v", err)
		}
	}
}

func loadFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	reader := bufio.NewReader(file)

	// Robustly skip the first line, handling potentially huge lines
	for {
		_, isPrefix, err := reader.ReadLine()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if !isPrefix {
			break
		}
	}

	scanner := bufio.NewScanner(reader)
	// 5MB buffer
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 5*1024*1024)

	rawLines = make([]string, 0, 10000)
	metaData = make([]LogMeta, 0, 10000)

	for scanner.Scan() {
		text := scanner.Text()
		// Copy text to ensure it's safe from scanner buffer reuse?
		// Scanner.Text() returns a string which is immutable, so it should be fine allocation-wise?
		// Actually Text() allocates a new string.

		rawLines = append(rawLines, text)

		// Parse just metadata for filtering
		var meta LogMeta
		// We ignore errors here for speed/robustness, if it's not JSON it will have empty datastore
		_ = json.Unmarshal([]byte(text), &meta)
		metaData = append(metaData, meta)
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	// Track file offset for follow mode
	if followMode {
		if info, err := file.Stat(); err == nil {
			lastOffset = info.Size()
		}
	}

	return nil
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	logMutex.RLock()
	defer logMutex.RUnlock()

	offset := 0
	limit := 100
	datastore := r.URL.Query().Get("datastore")

	if o := r.URL.Query().Get("offset"); o != "" {
		_, _ = fmt.Sscanf(o, "%d", &offset)
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		_, _ = fmt.Sscanf(l, "%d", &limit)
	}

	filteredIndices := make([]int, 0)
	for i, m := range metaData {
		if datastore == "" || m.DatastoreName == datastore {
			filteredIndices = append(filteredIndices, i)
		}
	}

	if offset >= len(filteredIndices) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"logs":       make([]LogLine, 0),
			"total":      len(filteredIndices),
			"generation": generation,
		}); err != nil {
			log.Printf("Error encoding response: %v", err)
		}
		return
	}

	end := offset + limit
	if end > len(filteredIndices) {
		end = len(filteredIndices)
	}

	resultIndices := filteredIndices[offset:end]
	logs := make([]LogLine, 0, len(resultIndices))

	for _, idx := range resultIndices {
		var line LogLine
		if err := json.Unmarshal([]byte(rawLines[idx]), &line); err != nil {
			log.Printf("Failed to parse line %d: %v", idx, err)
			continue
		}
		logs = append(logs, line)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"logs":       logs,
		"total":      len(filteredIndices),
		"generation": generation,
	}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func handleFindOffset(w http.ResponseWriter, r *http.Request) {
	logMutex.RLock()
	defer logMutex.RUnlock()

	targetTime := r.URL.Query().Get("time")
	datastore := r.URL.Query().Get("datastore")

	if targetTime == "" {
		http.Error(w, "Missing time parameter", http.StatusBadRequest)
		return
	}

	filteredIndices := make([]int, 0)
	for i, m := range metaData {
		if datastore == "" || m.DatastoreName == datastore {
			filteredIndices = append(filteredIndices, i)
		}
	}

	foundIdx := -1
	for i, globalIdx := range filteredIndices {
		if metaData[globalIdx].Time >= targetTime {
			foundIdx = i
			break
		}
	}

	if foundIdx == -1 {
		foundIdx = len(filteredIndices) - 1
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"offset": foundIdx,
	}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	logMutex.RLock()
	defer logMutex.RUnlock()

	query := r.URL.Query().Get("q")
	datastore := r.URL.Query().Get("datastore")

	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"matches": make([]int, 0),
			"total":   0,
		}); err != nil {
			log.Printf("Error encoding response: %v", err)
		}
		return
	}

	queryLower := strings.ToLower(query)

	filteredIndices := make([]int, 0)
	for i, m := range metaData {
		if datastore == "" || m.DatastoreName == datastore {
			filteredIndices = append(filteredIndices, i)
		}
	}

	matches := make([]int, 0)
	for filteredIdx, globalIdx := range filteredIndices {
		if strings.Contains(strings.ToLower(rawLines[globalIdx]), queryLower) {
			matches = append(matches, filteredIdx)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"matches": matches,
		"total":   len(matches),
	}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func handleDatastores(w http.ResponseWriter, r *http.Request) {
	logMutex.RLock()
	defer logMutex.RUnlock()

	set := make(map[string]bool)
	for _, m := range metaData {
		if m.DatastoreName != "" {
			set[m.DatastoreName] = true
		}
	}

	list := make([]string, 0, len(set))
	for k := range set {
		list = append(list, k)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(list); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func handleViewState(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		state, err := loadViewState()
		if err != nil {
			http.Error(w, "Failed to load view state: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := json.NewEncoder(w).Encode(state); err != nil {
			log.Printf("Error encoding response: %v", err)
		}
	case http.MethodPost:
		var st ViewState
		// Handle both application/json and beacon requests
		if err := json.NewDecoder(r.Body).Decode(&st); err != nil {
			http.Error(w, "Invalid view state payload", http.StatusBadRequest)
			return
		}
		if st.Offset < 0 {
			st.Offset = 0
		}
		if err := saveViewState(st); err != nil {
			http.Error(w, "Failed to save view state: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			log.Printf("Error encoding response: %v", err)
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func loadViewState() (ViewState, error) {
	var st ViewState
	if stateFile == "" {
		return st, fmt.Errorf("state file path not set")
	}
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return st, err
	}
	if len(data) == 0 {
		return st, nil
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return st, err
	}
	return st, nil
}

func saveViewState(st ViewState) error {
	if stateFile == "" {
		return fmt.Errorf("state file path not set")
	}
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(stateFile, data, 0644)
}

func handleDecode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	decoded, err := decodeTypedValue(req.Value)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"decoded": decoded,
	}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func handleDecodeBatch(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Values []string `json:"values"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	results := make([]interface{}, len(req.Values))
	for i, val := range req.Values {
		decoded, err := decodeTypedValue(val)
		if err != nil {
			results[i] = map[string]interface{}{"error": err.Error()}
		} else {
			results[i] = decoded
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"decoded": results,
	}); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func decodeTypedValue(encodedVal string) (interface{}, error) {
	data, err := base64.StdEncoding.DecodeString(encodedVal)
	if err != nil {
		return nil, fmt.Errorf("base64 decode error: %w", err)
	}

	var tv sdcpb.TypedValue
	if err := proto.Unmarshal(data, &tv); err != nil {
		return nil, fmt.Errorf("protobuf unmarshal error: %w", err)
	}

	jsonBytes, err := protojson.Marshal(&tv)
	if err != nil {
		return nil, fmt.Errorf("json marshal error: %w", err)
	}

	var result interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, fmt.Errorf("json unmarshal error: %w", err)
	}

	return result, nil
}
