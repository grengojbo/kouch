package driver

import (
	"bytes"
	"encoding/json"
)

// Row is a generic view result row.
type Row struct {
	// ID is the document ID of the result.
	ID string `json:"id"`
	// Key is the view key of the result. For built-in views, this is the same
	// as ID.
	Key json.RawMessage `json:"key"`
	// Value is the raw, un-decoded JSON value. For most built-in views (such as
	// /_all_docs), this is `{"rev":"X-xxx"}`.
	Value json.RawMessage `json:"value"`
	// Doc is the raw, un-decoded JSON document. This is only populated by views
	// which return docs, such as /_all_docs?include_docs=true.
	Doc json.RawMessage `json:"doc"`
}

// SequenceID is a CouchDB update sequence ID. This is just a string, but has
// a special JSON unmarshaler to work with both CouchDB 2.0.0 (which uses
// normal) strings for sequence IDs, and earlier versions (which use integers)
type SequenceID string

// UnmarshalJSON satisfies the json.Unmarshaler interface.
func (id *SequenceID) UnmarshalJSON(data []byte) error {
	sid := SequenceID(bytes.Trim(data, `""`))
	*id = sid
	return nil
}

// Rows is an iterator over a view's results.
type Rows interface {
	// Next is called to populate *Row with the values of the next row in a
	// result set.
	//
	// Next should return io.EOF when there are no more rows.
	Next(*Row) error
	// Close closes the rows iterator.
	Close() error
	// UpdateSeq is the update sequence of the database, if requested in the
	// result set.
	UpdateSeq() string
	// Offset is the offset where the result set starts.
	Offset() int64
	// TotalRows is the number of documents in the database/view.
	TotalRows() int64
}
