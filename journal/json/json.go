package json

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/koct9i/sand/journal"
)

type record struct {
	Type  journal.RecordType `json:"type"`
	Name  string             `json:"name,omitempty"`
	Data  json.RawMessage    `json:"data,omitempty"`
	State *journal.State     `json:"state,omitempty"`
}

var _ journal.Record = (*record)(nil)

func (r *record) GetType() journal.RecordType {
	return r.Type
}

func (r *record) GetName() string {
	return r.Name
}

func (r *record) GetData(data any) error {
	return json.Unmarshal(r.Data, data)
}

func (r *record) GetState() *journal.State {
	return r.State
}

// JSONReader reads a stream of JSON records. It does not apply records to objects
// or recover a truncated final record; those policies belong to the caller.
type JSONReader struct {
	decoder *json.Decoder
}

func NewJSONReader(reader io.Reader) *JSONReader {
	return &JSONReader{decoder: json.NewDecoder(reader)}
}

// ReadRecord returns io.EOF at the end of the stream.
func (r *JSONReader) ReadRecord() (*JSONRecord, error) {
	var record struct {
		Header *RecordHeader   `json:"header"`
		Data   json.RawMessage `json:"data"`
	}
	if err := r.decoder.Decode(&record); err != nil {
		if err == io.EOF {
			return nil, io.EOF
		}
		return nil, fmt.Errorf("journal: read record: %w", err)
	}
	if record.Header == nil || len(record.Data) == 0 {
		return nil, fmt.Errorf("journal: record requires header and data")
	}
	if record.Header.Type > RecordTypeJournalState {
		return nil, fmt.Errorf("journal: unknown record type %d", record.Header.Type)
	}
	return &JSONRecord{Header: *record.Header, Data: record.Data}, nil
}
