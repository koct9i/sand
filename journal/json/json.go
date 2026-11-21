package json

import (
	"encoding/json"
	"fmt"
	"io"
)

type RecordType uint8

const (
	RecordTypeChange RecordType = iota
	RecordTypeSnapshot
	RecordTypeCreate
	RecordTypeDelete
	RecordTypeJournalState
)

type RecordHeader struct {
	Type RecordType
}

type Record interface {
	GetHeader() RecordHeader
	GetType() RecordType
	GetData(any) error
}

type JSONRecord struct {
	Header RecordHeader    `json:"header"`
	Data   json.RawMessage `json:"data"`
}

var _ Record = (*JSONRecord)(nil)

// NewJSONRecord captures data as JSON so subsequent changes to data do not
// affect the record.
func NewJSONRecord(header RecordHeader, data any) (*JSONRecord, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("journal: encode record data: %w", err)
	}
	return &JSONRecord{Header: header, Data: payload}, nil
}

func (r *JSONRecord) GetHeader() RecordHeader {
	return r.Header
}

func (r *JSONRecord) GetType() RecordType {
	return r.Header.Type
}

func (r *JSONRecord) GetData(data any) error {
	return json.Unmarshal(r.Data, data)
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
