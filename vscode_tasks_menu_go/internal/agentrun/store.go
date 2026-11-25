package agentrun

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (m *Manager) file(runID string) string { return filepath.Join(m.dir, runID+".json") }

// ReadRecords returns the durable run journal without opening a Manager or
// starting workers. Active records from a prior process are presented as
// interrupted so callers never treat stale execution as live.
func ReadRecords(dir string) ([]Record, error) {
	records, err := readRunRecords(dir)
	if err != nil {
		return nil, err
	}
	for i := range records {
		if records[i].Status == Running || records[i].Status == WaitingTool {
			markInterrupted(&records[i])
		}
	}
	return records, nil
}

func readRunRecords(dir string) ([]Record, error) {
	if dir == "" || !filepath.IsAbs(dir) {
		return nil, errors.New("absolute run journal directory required")
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("unsafe run journal directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		record, err := readRunRecord(filepath.Join(dir, entry.Name()))
		if err != nil { return nil, err }
		records = append(records, record)
		if len(records) > maxStoredRuns {
			return nil, errors.New("run journal exceeds capacity")
		}
	}
	return records, nil
}

// ReadCompletedRecord resolves one exact private run journal without loading
// unrelated runs or starting a Manager. Active and ambiguous runs are never
// returned as successful evidence.
func ReadCompletedRecord(dir, runID string) (Record, error) {
 if dir=="" || !filepath.IsAbs(dir) || len(runID)==0 || len(runID)>64 ||
  strings.Trim(runID,"ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_.-")!="" ||
  runID=="." || runID==".." {
  return Record{},errors.New("invalid run journal lookup")
 }
 info,err:=os.Lstat(dir)
 if err!=nil{return Record{},err}
 if !info.IsDir()||info.Mode()&os.ModeSymlink!=0||info.Mode().Perm()&0077!=0{
  return Record{},errors.New("unsafe run journal directory")
 }
 record,err:=readRunRecord(filepath.Join(dir,runID+".json"))
 if err!=nil{return Record{},err}
 if record.Status!=Completed||record.FinishedAt==nil{return Record{},errors.New("run has no completed durable receipt")}
 return record,nil
}

func readRunRecord(path string)(Record,error){
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 || info.Size() > 8<<20 {
		return Record{}, errors.New("unsafe run journal record")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(body, &record); err != nil {
		return Record{}, errors.New("invalid run journal record")
	}
	if err := record.Spec.Validate(); err != nil || filepath.Base(path) != record.Spec.RunID+".json" {
		return Record{}, errors.New("invalid run journal binding")
	}
	switch record.Status {
	case Queued, Running, WaitingTool, Completed, Failed, Stopped, Interrupted:
	default:
		return Record{}, errors.New("invalid run status")
	}
 return record,nil
}

func markInterrupted(record *Record) {
	record.Status = Interrupted
	record.RetrySafe = false
	record.LastError = "TaskDeck restarted while run was active; inspect side effects before explicit resume"
	now := time.Now().UTC()
	record.UpdatedAt, record.FinishedAt = now, &now
}

func (m *Manager) load() error {
	records, err := readRunRecords(m.dir)
	if err != nil {
		return err
	}
	for _, record := range records {
		if record.Status == Running || record.Status == WaitingTool {
			markInterrupted(&record)
			if err := m.saveLocked(record); err != nil {
				return err
			}
		}
		m.runs[record.Spec.RunID] = record
	}
	return nil
}

func (m *Manager) saveLocked(record Record) error {
	body, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if len(body) > 8<<20 {
		return errors.New("run record exceeds storage limit")
	}
	tmp, err := os.CreateTemp(m.dir, ".run-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(append(body, '\n'))
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmpPath, m.file(record.Spec.RunID)); err != nil {
		return err
	}
	return syncRunDir(m.dir)
}

func syncRunDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
