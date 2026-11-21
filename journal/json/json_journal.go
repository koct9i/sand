package json

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/koct9i/sand/journal"
)

// ErrSnapshotsUnsupported marks the segment storage missing from this prototype.
var ErrSnapshotsUnsupported = errors.New("journal: snapshot segments are not implemented")

// State adds the live object count tracked by the JSON record stream to the
// core journal's storage metadata.
type State struct {
	journal.State
	CurrentObjects uint64 `json:"current_objects"`
}

// JSONJournal writes one JSON record per line to a single segment. The caller
// owns the writer and is responsible for flushing, syncing, and closing it.
// Calls must be serialized by the caller. A write error makes the journal
// unusable because the last record may have been written only partially.
type JSONJournal struct {
	writer io.Writer
	state  State
	err    error
}

// NewJSONJournal starts a segment with a state record. It does not replay an
// existing stream; state must describe the objects before subsequent appends.
func NewJSONJournal(writer io.Writer, state State) (*JSONJournal, error) {
	j := &JSONJournal{writer: writer, state: state}
	if err := j.Checkpoint(); err != nil {
		return nil, err
	}
	return j, nil
}

func (j *JSONJournal) GetState() State {
	return j.state
}

// AppendRecord accepts changes, creates, and deletes. The record must support
// GetData into a *json.RawMessage, as JSONRecord does.
func (j *JSONJournal) AppendRecord(record Record) error {
	if j.err != nil {
		return j.err
	}
	if record == nil {
		return errors.New("journal: nil record")
	}
	header := record.GetHeader()
	if header.Type != record.GetType() {
		return errors.New("journal: record type disagrees with header")
	}
	state := j.state
	switch header.Type {
	case RecordTypeChange:
	case RecordTypeCreate:
		if state.CurrentObjects == ^uint64(0) {
			return errors.New("journal: object count overflow")
		}
		state.CurrentObjects++
	case RecordTypeDelete:
		if state.CurrentObjects == 0 {
			return errors.New("journal: cannot delete from an empty journal")
		}
		state.CurrentObjects--
	default:
		return fmt.Errorf("journal: record type %d cannot be appended as a change", header.Type)
	}

	var data json.RawMessage
	if err := record.GetData(&data); err != nil {
		return fmt.Errorf("journal: get record data: %w", err)
	}
	if err := j.writeRecord(&JSONRecord{Header: header, Data: data}); err != nil {
		return err
	}
	// Publish the new state only after the complete record has been written.
	j.state = state
	return nil
}

// Checkpoint records the current state without changing it. It does not flush or
// sync the writer, so success alone does not imply that the state is durable.
func (j *JSONJournal) Checkpoint() error {
	record, err := NewJSONRecord(RecordHeader{Type: RecordTypeJournalState}, j.state)
	if err != nil {
		return err
	}
	return j.writeRecord(record)
}

func (j *JSONJournal) writeRecord(record *JSONRecord) error {
	if j.err != nil {
		return j.err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("journal: encode record: %w", err)
	}
	data = append(data, '\n')
	n, err := j.writer.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		j.err = fmt.Errorf("journal: write record: %w", err)
		return j.err
	}
	return nil
}

func (j *JSONJournal) CreateSnapshotSegments(count uint64) error {
	return ErrSnapshotsUnsupported
}

func (j *JSONJournal) CommitSnapshotSegment() error {
	return ErrSnapshotsUnsupported
}

func (j *JSONJournal) RecordSnapshot(record Record) error {
	return ErrSnapshotsUnsupported
}
