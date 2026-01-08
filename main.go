package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	sdcpb "github.com/sdcio/sdc-protos/sdcpb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type LogLine map[string]interface{}

type LogMeta struct {
	DatastoreName string `json:"datastore-name"`
	Time          string `json:"time"`
}

var (
	rawLines []string
	metaData []LogMeta
	logFile  string
)

func main() {
	flag.StringVar(&logFile, "file", "", "Path to the log file")
	flag.Parse()

	if logFile == "" {
		if len(os.Args) > 1 {
			if os.Args[1][0] != '-' {
				logFile = os.Args[1]
			}
		}
	}

	if logFile == "" {
		log.Fatal("Please provide a log file via -file flag or as argument")
	}

	// Pre-load file
	if err := loadFile(logFile); err != nil {
		log.Fatal(err)
	}

	http.Handle("/", http.FileServer(http.Dir("./static")))
	http.HandleFunc("/api/logs", handleLogs)
	http.HandleFunc("/api/offset", handleFindOffset)
	http.HandleFunc("/api/decode", handleDecode)
	http.HandleFunc("/api/decode/batch", handleDecodeBatch)
	http.HandleFunc("/api/datastores", handleDatastores)

	fmt.Printf("Starting server on :8080 with log file: %s (%d lines)\n", logFile, len(rawLines))
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func loadFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

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
	return scanner.Err()
}

func handleDatastores(w http.ResponseWriter, r *http.Request) {
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
	json.NewEncoder(w).Encode(list)
}

func handleFindOffset(w http.ResponseWriter, r *http.Request) {
	targetTime := r.URL.Query().Get("time")
	datastore := r.URL.Query().Get("datastore")

	// Normalize input time (e.g. 8:20 -> 08:20)
	if len(targetTime) > 0 && targetTime[0] != '0' && strings.Contains(targetTime, ":") {
		parts := strings.Split(targetTime, ":")
		if len(parts) > 0 && len(parts[0]) == 1 {
			targetTime = "0" + targetTime
		}
	}

	count := 0
	foundOffset := -1

	for _, meta := range metaData {
		if datastore != "" && meta.DatastoreName != datastore {
			continue
		}

		tVal := meta.Time
		if tVal != "" {
			// Extract time from ISO string "2006-01-02T15:04:05..."
			// We split by T and take the second part
			if idx := strings.Index(tVal, "T"); idx != -1 {
				timePart := tVal[idx+1:]
				// If valid part, compare
				// We compare prefix length to support "15:39" vs "15:39:47.123"
				compareLen := len(targetTime)
				if len(timePart) >= compareLen {
					if timePart[:compareLen] >= targetTime {
						foundOffset = count
						break
					}
				} else {
					// Time in log is shorter than target? Rare but compare directly
					if timePart >= targetTime {
						foundOffset = count
						break
					}
				}
			}
		}
		count++
	}

	if foundOffset == -1 {
		// Not found, default to end? Or 0?
		// Let's return 0 if nothing found so user sees something, or handle in frontend
		// Usually if time > all logs, we probably want to show the End.
		// If we return count (which is total filtered items), we show nothing (empty).
		foundOffset = count
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int{"offset": foundOffset})
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	// Query params: offset, limit, datastore
	offset := 0
	limit := 100
	datastore := r.URL.Query().Get("datastore")

	if o := r.URL.Query().Get("offset"); o != "" {
		fmt.Sscanf(o, "%d", &offset)
	}
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	if limit > 1000 {
		limit = 1000
	}

	var result []LogLine

	// Filtering and Paginating
	// If datastore is empty, we just slice rawLines
	// If datastore is present, we must scan metaData to find matching indices

	count := 0
	skipped := 0

	for i, meta := range metaData {
		if datastore != "" && meta.DatastoreName != datastore {
			continue
		}

		// Valid match
		if skipped < offset {
			skipped++
			continue
		}

		if count >= limit {
			break
		}

		// Parse full line
		var line LogLine
		if err := json.Unmarshal([]byte(rawLines[i]), &line); err == nil {
			result = append(result, line)
		} else {
			result = append(result, LogLine{"raw": rawLines[i]})
		}
		count++
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

type DecodeRequest struct {
	Value string `json:"value"`
}

func handleDecode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DecodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	decodedBytes, err := base64.StdEncoding.DecodeString(req.Value)
	if err != nil {
		http.Error(w, "Failed to base64 decode: "+err.Error(), http.StatusBadRequest)
		return
	}

	tv := &sdcpb.TypedValue{}
	if err := proto.Unmarshal(decodedBytes, tv); err != nil {
		http.Error(w, "Failed to unmarshal as TypedValue: "+err.Error(), http.StatusBadRequest)
		return
	}

	marshaller := protojson.MarshalOptions{Multiline: true, EmitUnpopulated: false}
	jsonBytes, err := marshaller.Marshal(tv)
	if err != nil {
		http.Error(w, "Failed to marshal to JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes)
}

func handleDecodeBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var values []string
	if err := json.NewDecoder(r.Body).Decode(&values); err != nil {
		http.Error(w, "Invalid request body (expected array of strings)", http.StatusBadRequest)
		return
	}

	results := make([]interface{}, len(values))
	// Reuse the marshaller
	marshaller := protojson.MarshalOptions{Multiline: true, EmitUnpopulated: false}

	for i, v := range values {
		// Prepare a result object for this item
		res := make(map[string]interface{})

		decodedBytes, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			res["error"] = "Base64 error: " + err.Error()
			results[i] = res
			continue
		}

		tv := &sdcpb.TypedValue{}
		if err := proto.Unmarshal(decodedBytes, tv); err != nil {
			res["error"] = "Proto error: " + err.Error()
			results[i] = res
			continue
		}

		jsonBytes, err := marshaller.Marshal(tv)
		if err != nil {
			res["error"] = "JSON error: " + err.Error()
			results[i] = res
			continue
		}

		// Unmarshal back to interface{} to embed in the larger JSON response properly
		var jsonVal interface{}
		if err := json.Unmarshal(jsonBytes, &jsonVal); err != nil {
			res["error"] = "Re-unmarshal error: " + err.Error() // Should not happen
			results[i] = res
		} else {
			results[i] = jsonVal
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
