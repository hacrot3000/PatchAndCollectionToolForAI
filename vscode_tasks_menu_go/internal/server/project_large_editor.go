package server

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

const (
	largeEditorStride    = 256
	largeEditorChunk     = 256 << 10
	largeEditorMaxWindow = 2 << 20
	largeEditorMaxLines  = 1024
)

type largeEditorIndex struct {
	size    int64
	mtime   int64
	info    os.FileInfo
	offsets []int64
	lines   int
	hash    string
}

var largeEditorIndexes = struct {
	sync.Mutex
	items map[string]*largeEditorIndex
	order []string
}{items: make(map[string]*largeEditorIndex)}

func editorLargeIndex(path string, file *os.File, info os.FileInfo) (*largeEditorIndex, error) {
	largeEditorIndexes.Lock()
	cached := largeEditorIndexes.items[path]
	if cached != nil && cached.size == info.Size() && cached.mtime == info.ModTime().UnixNano() && os.SameFile(cached.info, info) {
		largeEditorIndexes.Unlock()
		return cached, nil
	}
	largeEditorIndexes.Unlock()

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	idx := &largeEditorIndex{size: info.Size(), mtime: info.ModTime().UnixNano(), info: info, offsets: []int64{0}, lines: 1}
	digest := sha256.New()
	buf := make([]byte, largeEditorChunk)
	var scanned int64
	var carry []byte
	for {
		n, err := file.Read(buf)
		if n > 0 {
			piece := buf[:n]
			if bytes.IndexByte(piece, 0) >= 0 {
				return nil, fmt.Errorf("binary file contains NUL bytes")
			}
			if _, hashErr := digest.Write(piece); hashErr != nil {
				return nil, hashErr
			}
			merged := append(carry, piece...)
			carry = nil
			for at := 0; at < len(merged); {
				r, width := utf8.DecodeRune(merged[at:])
				if r == utf8.RuneError && width == 1 {
					if !utf8.FullRune(merged[at:]) {
						carry = append([]byte(nil), merged[at:]...)
						break
					}
					return nil, fmt.Errorf("invalid UTF-8 in large file")
				}
				at += width
			}
			for at, b := range piece {
				if b == '\n' {
					idx.lines++
					if (idx.lines-1)%largeEditorStride == 0 {
						idx.offsets = append(idx.offsets, scanned+int64(at)+1)
					}
				}
			}
			scanned += int64(n)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	if len(carry) > 0 {
		return nil, fmt.Errorf("truncated UTF-8 in large file")
	}
	if scanned != info.Size() {
		return nil, fmt.Errorf("file changed while indexing")
	}
	latest, err := file.Stat()
	if err != nil || latest.Size() != info.Size() || !latest.ModTime().Equal(info.ModTime()) || !os.SameFile(info, latest) {
		return nil, fmt.Errorf("file changed while indexing")
	}
	idx.hash = hex.EncodeToString(digest.Sum(nil))
	largeEditorIndexes.Lock()
	// At most 12 cached files; each stores one offset per 256 lines, never file contents.
	if previous := largeEditorIndexes.items[path]; previous != nil {
		for i, item := range largeEditorIndexes.order {
			if item == path {
				largeEditorIndexes.order = append(largeEditorIndexes.order[:i], largeEditorIndexes.order[i+1:]...)
				break
			}
		}
	}
	if len(largeEditorIndexes.order) >= 12 {
		delete(largeEditorIndexes.items, largeEditorIndexes.order[0])
		largeEditorIndexes.order = largeEditorIndexes.order[1:]
	}
	largeEditorIndexes.items[path] = idx
	largeEditorIndexes.order = append(largeEditorIndexes.order, path)
	largeEditorIndexes.Unlock()
	return idx, nil
}

func forgetLargeEditorIndex(path string) {
	largeEditorIndexes.Lock()
	delete(largeEditorIndexes.items, path)
	for i, item := range largeEditorIndexes.order {
		if item == path {
			largeEditorIndexes.order = append(largeEditorIndexes.order[:i], largeEditorIndexes.order[i+1:]...)
			break
		}
	}
	largeEditorIndexes.Unlock()
}

func readBoundedEditorLine(reader *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		part, err := reader.ReadSlice('\n')
		if len(line)+len(part) > largeEditorMaxWindow {
			return nil, fmt.Errorf("physical line exceeds large-editor window limit")
		}
		line = append(line, part...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line, err
	}
}

func largeEditorStart(file *os.File, idx *largeEditorIndex, line int) (*bufio.Reader, int64, error) {
	checkpoint := (line-1)/largeEditorStride
	if checkpoint >= len(idx.offsets) {
		return nil, 0, fmt.Errorf("line out of range")
	}
	position := idx.offsets[checkpoint]
	if _, err := file.Seek(position, io.SeekStart); err != nil {
		return nil, 0, err
	}
	reader := bufio.NewReaderSize(file, 64<<10)
	for at := checkpoint*largeEditorStride + 1; at < line; at++ {
		part, err := readBoundedEditorLine(reader)
		if err != nil {
			return nil, 0, fmt.Errorf("line out of range: %w", err)
		}
		position += int64(len(part))
	}
	return reader, position, nil
}

type largeEditorWindowResponse struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	TotalLines int    `json:"total_lines"`
	StartByte  int64  `json:"start_byte"`
	EndByte    int64  `json:"end_byte"`
	LineEnding string `json:"line_ending"`
	WindowSHA  string `json:"window_sha256"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	MtimeNS    int64  `json:"mtime_ns"`
	ReadOnly   bool   `json:"read_only"`
	LargeFile  bool   `json:"large_file"`
	Streamed   bool   `json:"streamed"`
}

func (s *Server) openLargeEditorFile(rel string) (*projectPinnedFile, *os.File, string, os.FileInfo, error) {
	path, err := s.resolveProjectPath(rel, false, false)
	if err != nil {
		return nil, nil, "", nil, err
	}
	root, _, err := s.projectRootForVirtualPath(rel)
	if err != nil {
		return nil, nil, "", nil, err
	}
	pinned, err := openProjectPinnedFile(root.Path, path)
	if err != nil {
		return nil, nil, "", nil, err
	}
	file, err := pinned.openCurrent()
	if err != nil {
		pinned.close()
		return nil, nil, "", nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		pinned.close()
		return nil, nil, "", nil, fmt.Errorf("file is not regular")
	}
	return pinned, file, path, info, nil
}

func (s *Server) projectLargeFileWindow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	rel, err := cleanProjectRelativePath(r.URL.Query().Get("path"), false)
	if err != nil {
		http.Error(w, "invalid file path", http.StatusBadRequest)
		return
	}
	start, _ := strconv.Atoi(r.URL.Query().Get("start"))
	if start < 1 {
		start = 1
	}
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count < 1 {
		count = 150
	}
	if count > largeEditorMaxLines {
		count = largeEditorMaxLines
	}
	pinned, file, path, info, err := s.openLargeEditorFile(rel)
	if err != nil {
		http.Error(w, "file unavailable", http.StatusNotFound)
		return
	}
	defer pinned.close()
	defer file.Close()
	idx, err := editorLargeIndex(path, file, info)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		return
	}
	if start > idx.lines {
		start = idx.lines
	}
	reader, startByte, err := largeEditorStart(file, idx, start)
	if err != nil {
		http.Error(w, err.Error(), http.StatusRequestedRangeNotSatisfiable)
		return
	}
	end := start - 1
	var content bytes.Buffer
	for end < idx.lines && end-start < count-1 {
		line, readErr := readBoundedEditorLine(reader)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			http.Error(w, readErr.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		if content.Len()+len(line) > largeEditorMaxWindow {
			if end < start {
				http.Error(w, "physical line exceeds 2 MiB", http.StatusRequestEntityTooLarge)
				return
			}
			break
		}
		if len(line) == 0 && errors.Is(readErr, io.EOF) && end >= start {
			break
		}
		content.Write(line)
		end++
		if readErr != nil {
			break
		}
	}
	if end < start {
		end = start
	}
	data := content.Bytes()
	digest := sha256.Sum256(data)
	// A previous cached index must not hide an external writer during this read.
	if latest, err := file.Stat(); err != nil || latest.Size() != idx.size || latest.ModTime().UnixNano() != idx.mtime {
		http.Error(w, "file changed while loading window", http.StatusConflict)
		return
	}
	writeJSON(w, http.StatusOK, largeEditorWindowResponse{
		Path: rel, Content: string(data), StartLine: start, EndLine: end, TotalLines: idx.lines,
		StartByte: startByte, EndByte: startByte + int64(len(data)),
		WindowSHA: hex.EncodeToString(digest[:]), SHA256: idx.hash,
		Size: info.Size(), MtimeNS: info.ModTime().UnixNano(),
		LineEnding: detectProjectLineEnding(data),
		ReadOnly: info.Mode().Perm()&0o222 == 0, LargeFile: true, Streamed: true,
	})
}

type largeEditorSaveInput struct {
	Path       string `json:"path"`
	StartByte  int64  `json:"start_byte"`
	EndByte    int64  `json:"end_byte"`
	Size       int64  `json:"expected_size"`
	MtimeNS    int64  `json:"expected_mtime_ns"`
	SHA256     string `json:"expected_sha256"`
	WindowSHA  string `json:"expected_window_sha256"`
	Content    string `json:"content"`
}

func (s *Server) projectLargeFileSave(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	var req largeEditorSaveInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, largeEditorMaxWindow*6))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		http.Error(w, "invalid large editor save", http.StatusBadRequest)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		http.Error(w, "exactly one JSON request required", http.StatusBadRequest)
		return
	}
	rel, err := cleanProjectRelativePath(req.Path, false)
	if err != nil {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if req.StartByte < 0 || req.EndByte < req.StartByte || req.EndByte > req.Size ||
		req.EndByte-req.StartByte > largeEditorMaxWindow || len(req.Content) > largeEditorMaxWindow ||
		!utf8.ValidString(req.Content) || strings.ContainsRune(req.Content, ' ') {
		http.Error(w, "invalid edited range", http.StatusBadRequest)
		return
	}
	if len(req.SHA256) != 64 || len(req.WindowSHA) != 64 {
		http.Error(w, "source fingerprint missing", http.StatusBadRequest)
		return
	}
	pinned, file, path, info, err := s.openLargeEditorFile(rel)
	if err != nil {
		http.Error(w, "file unavailable", http.StatusNotFound)
		return
	}
	defer pinned.close()
	defer file.Close()
	if info.Mode().Perm()&0o222 == 0 {
		http.Error(w, "file is read-only", http.StatusForbidden)
		return
	}
	lease, ok := s.acquireSharedMutation(w, r, "file.write", rel)
	if !ok {
		return
	}
	defer s.releaseSharedMutation(lease)
	if req.Size != info.Size() || req.MtimeNS != info.ModTime().UnixNano() {
		http.Error(w, "file changed outside editor", http.StatusConflict)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		http.Error(w, "file unavailable", http.StatusInternalServerError)
		return
	}
	currentHash := sha256.New()
	if _, err := io.Copy(currentHash, file); err != nil {
		http.Error(w, "file read failed", http.StatusInternalServerError)
		return
	}
	if !strings.EqualFold(hex.EncodeToString(currentHash.Sum(nil)), req.SHA256) {
		http.Error(w, "file content changed", http.StatusConflict)
		return
	}
	regionHash := sha256.New()
	if _, err := io.Copy(regionHash, io.NewSectionReader(file, req.StartByte, req.EndByte-req.StartByte)); err != nil {
		http.Error(w, "file range unavailable", http.StatusInternalServerError)
		return
	}
	if !strings.EqualFold(hex.EncodeToString(regionHash.Sum(nil)), req.WindowSHA) {
		http.Error(w, "file region changed", http.StatusConflict)
		return
	}
	stream := io.MultiReader(io.NewSectionReader(file, 0, req.StartByte),
		strings.NewReader(req.Content), io.NewSectionReader(file, req.EndByte, req.Size-req.EndByte))
	if err := pinned.writeTempStream(stream, info); err != nil {
		http.Error(w, "cannot prepare large-file save", http.StatusInternalServerError)
		return
	}
	latest, err := file.Stat()
	if err != nil || latest.Size() != req.Size || latest.ModTime().UnixNano() != req.MtimeNS || !os.SameFile(info, latest) {
		http.Error(w, "file changed during save", http.StatusConflict)
		return
	}
	if err := pinned.commitTemp(); err != nil {
		http.Error(w, "cannot commit large-file save", http.StatusInternalServerError)
		return
	}
	forgetLargeEditorIndex(path)
	s.invalidateProjectSymbolIndex()
	s.auditSharedSuccess(r, "file.write", "file", rel, map[string]any{
		"size": req.Size + int64(len(req.Content)) - (req.EndByte - req.StartByte), "windowed": true,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "path": rel,
		"size": req.Size + int64(len(req.Content)) - (req.EndByte - req.StartByte),
		"history_warning": "Large-file save does not create a local history snapshot; use Git for recovery.",
	})
}

func (s *Server) projectLargeFileSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	rel, err := cleanProjectRelativePath(r.URL.Query().Get("path"), false)
	if err != nil {
		http.Error(w, "invalid file path", http.StatusBadRequest)
		return
	}
	needle := r.URL.Query().Get("q")
	if len(needle) == 0 || len(needle) > 256 || strings.ContainsRune(needle, ' ') {
		http.Error(w, "invalid search term", http.StatusBadRequest)
		return
	}
	start, _ := strconv.Atoi(r.URL.Query().Get("start"))
	if start < 1 {
		start = 1
	}
	matchCase := r.URL.Query().Get("case") == "1"
	reverse := r.URL.Query().Get("reverse") == "1"
	if !matchCase {
		needle = strings.ToLower(needle)
	}
	pinned, file, path, info, err := s.openLargeEditorFile(rel)
	if err != nil {
		http.Error(w, "file unavailable", http.StatusNotFound)
		return
	}
	defer pinned.close()
	defer file.Close()
	idx, err := editorLargeIndex(path, file, info)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
		return
	}
	if start > idx.lines {
		start = idx.lines
	}
	search := func(begin, finish int) (int, int, error) {
		reader, _, err := largeEditorStart(file, idx, begin)
		if err != nil {
			return 0, 0, err
		}
		for line := begin; line < finish; line++ {
			data, err := readBoundedEditorLine(reader)
			if err != nil && !errors.Is(err, io.EOF) {
				return 0, 0, err
			}
			source := string(data)
			if !matchCase {
				source = strings.ToLower(source)
			}
			if col := strings.Index(source, needle); col >= 0 {
				return line, col, nil
			}
			if err != nil {
				break
			}
		}
		return 0, 0, nil
	}
	var line, col int
	if reverse {
		reader, _, seekErr := largeEditorStart(file, idx, 1)
		err = seekErr
		var lastLine, lastColumn int
		for n := 1; n <= idx.lines && err == nil; n++ {
			var raw []byte
			raw, err = readBoundedEditorLine(reader)
			if err != nil && !errors.Is(err, io.EOF) { break }
			eof := errors.Is(err, io.EOF)
			err = nil
			text := string(raw)
			if !matchCase { text = strings.ToLower(text) }
			if column := strings.Index(text, needle); column >= 0 {
				lastLine, lastColumn = n, column
				if n <= start { line, col = n, column }
			}
			if eof { break }
		}
		if line == 0 && err == nil { line, col = lastLine, lastColumn }
	} else {
		line, col, err = search(start, idx.lines+1)
		if line == 0 && err == nil && start > 1 {
			line, col, err = search(1, start)
		}
	}
	if err != nil {
		http.Error(w, "large-file search failed: "+err.Error(), http.StatusRequestEntityTooLarge)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path": rel, "line": line, "column": col, "found": line > 0, "total_lines": idx.lines,
	})
}
