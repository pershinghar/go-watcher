package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Chunk represents a single route entry in the routing table
type Chunk struct {
	StartLine int64
	EndLine   int64
	Hash      string
	Data      []byte
	Destination string
}

// DataTable manages the routing table file and its chunks
type DataTable struct {
	FilePath string
	Chunks   map[string]*Chunk // key is destination (e.g., "0.0.0.0/0")
	mu       sync.RWMutex
}

// NewDataTable creates a new DataTable instance
func NewDataTable(filePath string) *DataTable {
	return &DataTable{
		FilePath: filePath,
		Chunks:   make(map[string]*Chunk),
	}
}

// hashChunk computes SHA256 hash of the chunk data
func hashChunk(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// LoadDataTable loads the routing table file, chunks it by routes, and hashes each chunk
func (rt *DataTable) LoadDataTable() error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	file, err := os.Open(rt.FilePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var currentChunk *Chunk
	var chunkLines []string
	var lineNum int64
	var currentDestination string

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		
		// Check if this line starts a new route
		if strings.HasPrefix(line, "Destination:") {
			// Save previous chunk if exists
			if currentChunk != nil && len(chunkLines) > 0 {
				chunkData := []byte(strings.Join(chunkLines, "\n"))
				currentChunk.Data = chunkData
				currentChunk.Hash = hashChunk(chunkData)
				currentChunk.EndLine = lineNum - 1
				rt.Chunks[currentChunk.Destination] = currentChunk
			}
			
			// Extract destination from line (e.g., "Destination: 0.0.0.0/0")
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				currentDestination = parts[1]
			} else {
				currentDestination = fmt.Sprintf("unknown_%d", lineNum)
			}
			
			// Start new chunk
			currentChunk = &Chunk{
				StartLine:   lineNum,
				Destination: currentDestination,
			}
			chunkLines = []string{line}
		} else if currentChunk != nil {
			// Add line to current chunk
			chunkLines = append(chunkLines, line)
			
			// If we hit a blank line after some content, it might be end of route
			// But we'll continue until next Destination: to be safe
		}
	}

	// Save last chunk
	if currentChunk != nil && len(chunkLines) > 0 {
		chunkData := []byte(strings.Join(chunkLines, "\n"))
		currentChunk.Data = chunkData
		currentChunk.Hash = hashChunk(chunkData)
		currentChunk.EndLine = lineNum
		rt.Chunks[currentChunk.Destination] = currentChunk
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	fmt.Printf("Loaded %d route chunks from %s\n", len(rt.Chunks), rt.FilePath)
	return nil
}

// DetectChanges re-hashes chunks and returns list of changed destinations
func (rt *DataTable) DetectChanges() ([]string, error) {
	rt.mu.RLock()
	oldChunks := make(map[string]*Chunk)
	for k, v := range rt.Chunks {
		oldChunks[k] = v
	}
	rt.mu.RUnlock()

	// Create temporary routing table to load new state
	tempRT := NewDataTable(rt.FilePath)
	if err := tempRT.LoadDataTable(); err != nil {
		return nil, fmt.Errorf("failed to reload routing table: %w", err)
	}

	var changed []string

	// Compare hashes
	tempRT.mu.RLock()
	defer tempRT.mu.RUnlock()

	// Check existing chunks for changes
	for dest, oldChunk := range oldChunks {
		newChunk, exists := tempRT.Chunks[dest]
		if !exists {
			// Route was deleted
			changed = append(changed, dest)
		} else if newChunk.Hash != oldChunk.Hash {
			// Route was modified
			changed = append(changed, dest)
		}
	}

	// Check for new routes
	for dest := range tempRT.Chunks {
		if _, exists := oldChunks[dest]; !exists {
			changed = append(changed, dest)
		}
	}

	// Update our chunks with new state
	rt.mu.Lock()
	rt.Chunks = tempRT.Chunks
	rt.mu.Unlock()

	return changed, nil
}

// FileWatcher handles file system notifications
type FileWatcher struct {
	watcher   *fsnotify.Watcher
	filePath  string
	onChange  func()
	debounce  time.Duration
	lastEvent time.Time
	timer     *time.Timer
	mu        sync.Mutex
}

// NewFileWatcher creates a new file watcher
func NewFileWatcher(filePath string, onChange func(), debounce time.Duration) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	// Watch the directory containing the file
	dir := filepath.Dir(filePath)
	if err := watcher.Add(dir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("failed to watch directory: %w", err)
	}

	fw := &FileWatcher{
		watcher:  watcher,
		filePath: filePath,
		onChange: onChange,
		debounce: debounce,
	}

	return fw, nil
}

// Start begins watching for file changes
func (fw *FileWatcher) Start() error {
	go fw.watch()
	return nil
}

// watch monitors file system events
func (fw *FileWatcher) watch() {
	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}
			
			// Check if it's our file
			if event.Name == fw.filePath {
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
					fw.handleChange()
				}
			}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			fmt.Printf("File watcher error: %v\n", err)
		}
	}
}

// handleChange debounces change events
func (fw *FileWatcher) handleChange() {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	// Cancel previous timer if exists
	if fw.timer != nil {
		fw.timer.Stop()
	}

	// Set new timer
	fw.timer = time.AfterFunc(fw.debounce, func() {
		fw.mu.Lock()
		fw.lastEvent = time.Now()
		fw.mu.Unlock()
		fw.onChange()
	})
}

// Close stops the file watcher
func (fw *FileWatcher) Close() error {
	fw.mu.Lock()
	if fw.timer != nil {
		fw.timer.Stop()
	}
	fw.mu.Unlock()
	return fw.watcher.Close()
}

func main() {
	// Setup command line flags
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -file <file>\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Watch a file for changes and detect modified content using hashing.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s -file .data/t.txt\n", os.Args[0])
	}

	var filePath string
	flag.StringVar(&filePath, "file", "", "Path to routing table file (required)")
	flag.Parse()

	// Check if file argument was provided
	if filePath == "" {
		fmt.Fprintf(os.Stderr, "Error: -file argument is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		fmt.Printf("Error: file %s does not exist\n", filePath)
		os.Exit(1)
	}

	// Create routing table
	rt := NewDataTable(filePath)
	
	fmt.Println("Loading  table...")
	start := time.Now()
	if err := rt.LoadDataTable(); err != nil {
		fmt.Printf("Error loading  table: %v\n", err)
		os.Exit(1)
	}
	loadDuration := time.Since(start)
	fmt.Printf("Loaded in %v\n", loadDuration)

	// Setup file watcher
	onChange := func() {
		fmt.Println("\n[File Change Detected] Detecting changes...")
		start := time.Now()
		changed, err := rt.DetectChanges()
		if err != nil {
			fmt.Printf("Error detecting changes: %v\n", err)
			return
		}
		detectDuration := time.Since(start)
		
		if len(changed) == 0 {
			fmt.Printf("No changes detected (checked in %v)\n", detectDuration)
		} else {
			fmt.Printf("Found %d changed routes (detected in %v):\n", len(changed), detectDuration)
			// Show first 10 changed routes
			maxShow := 10
			if len(changed) < maxShow {
				maxShow = len(changed)
			}
			for i := 0; i < maxShow; i++ {
				fmt.Printf("  - %s\n", changed[i])
			}
			if len(changed) > maxShow {
				fmt.Printf("  ... and %d more\n", len(changed)-maxShow)
			}
		}
	}

	watcher, err := NewFileWatcher(filePath, onChange, 500*time.Millisecond)
	if err != nil {
		fmt.Printf("Error creating file watcher: %v\n", err)
		os.Exit(1)
	}
	defer watcher.Close()

	if err := watcher.Start(); err != nil {
		fmt.Printf("Error starting file watcher: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Watching %s for changes... (press Ctrl+C to exit)\n", filePath)
	
	// Keep program running
	select {}
}

