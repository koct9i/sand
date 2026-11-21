package journal

import (
	"fmt"
)

type Journal interface {
	Write([]byte) (Ref, error)
	Read([]byte, Ref) error
	GetState() State

	AllocateSnapshot(length uint64) error
	CommitSnapshot(length uint64) error
	WriteSnapshot([]byte) (Ref, error)
}

// Ref is 24-bit segment (lower bit is epoch parity) | 40-bit offset
type Ref uint64

const (
	SegmentBits = 24
	OffsetBits  = 40
	MaxSegment  = 1 << SegmentBits
	MaxOffset   = 1 << OffsetBits
)

func (r Ref) Parity() bool {
	return r&(1<<OffsetBits) != 0
}

func (r Ref) Segment() uint32 {
	return uint32(r >> OffsetBits)
}

func (r Ref) Offset() uint64 {
	return uint64(r) & (MaxOffset - 1)
}

func (r Ref) NextSegment(inc uint32) Ref {
	return Ref((uint64(r)>>OffsetBits + uint64(inc)) << OffsetBits)
}

func (r *Ref) Set(segment uint32, offset uint64) {
	*r = Ref((uint64(segment) << OffsetBits) | offset)
}

func (r Ref) String() string {
	return fmt.Sprintf("%x:%x", r.Segment(), r.Offset())
}

func (r Ref) MarshalText() ([]byte, error) {
	return []byte(r.String()), nil
}

func (r Ref) UnmarshalText(text []byte) error {
	var segment uint32
	var offset uint64
	if n, err := fmt.Sscanf(string(text), "%x:%x", &segment, &offset); err != nil {
		return err
	} else if n != 2 || segment >= MaxSegment || offset >= MaxOffset {
		return fmt.Errorf("invalid ref: %q", string(text))
	}
	r.Set(segment, offset)
	return nil
}

type Epoch uint32

func (e Epoch) Before(o Epoch) bool {
	return int64(e-o) > 0
}

// GetParity returns one bit which each object needs to store for tracking snapshot epoch.
// Journal entries for object mutations against snapshot from previous epoch are marked.
func (e Epoch) Parity() bool {
	return e&1 == 1
}

func (e Epoch) Adjust(parity bool) Epoch {
	x := e
	if e.Parity() != parity {
		x -= 1
	}
	return x
}

// Segment A (Previous snapshot)
//   State
//     CurrentEpoch = X
// 	   CurrentSegment = A
//     SnapshotEpoch = X-1
//     SnapshotSegment = A
//     SnapshotObjects = 0
//   ...
// ...
// Segment A1  (Last segment of previous snapshot)
//   ...
//   State
//     CurrentEpoch = X
//     CurrentSegment = B
//     SnapshotEpoch = X
//     SnapshotSegment = A
//   ...
// ...
// Segment B  (Last segment before new snapshot)
//   State
//     CurrentEpoch = X
//     CurrentSegment = B
//     CurrentObjects = W1
//     SnapshotEpoch = X
//     SnapshotSegment = A
//     SnapshotObjects = W2
//   ...
//   State (CreateSnapshotSegments)
//     CurrentEpoch = X+1
//     CurrentSegment = B+k
//     CurrentObjects = W
//     SnapshotEpoch = X
//     SnapshotSegment = B+1
//     SnapshotObjects = 0
// ------ cut here after snapshot ------
// Segment B+1  (First snapshot segment)
//   State
//     CurrentEpoch = X+1
//     CurrentSegment = B+1
//     CurrentObjects = W
//     SnapshotEpoch = X
//     SnapshotSegment = B+1
//     SnapshotObjects = 0
//   ...
//   State
//     CurrentEpoch = X+1
//     CurrentSegment = B+2
//     CurrentObjects = W
//     SnapshotEpoch = X
//     SnapshotSegment = B+1
//     SnapshotObjects = <objects in first segment>
// Segment B+2  (Second snapshot segment)
//   State
//     CurrentEpoch = X+1
//     CurrentSegment = B+2
//     CurrentObjects = W
//     SnapshotEpoch = X
//     SnapshotSegment = B+1
//     SnapshotObjects = <objects in first segment>
//   ...
//   State
//     CurrentEpoch = X+1
//     CurrentSegment = B+3
//     CurrentObjects = W
//     SnapshotEpoch = X
//     SnapshotSegment = B+1
//     SnapshotObjects = <objects in first and second segments>
// ...
// Segment B+k (Fist segment after snapshot)
//   State
//     CurrentEpoch = X+1
//     CurrentSegment = B+k
//     CurrentObjects = W
//     SnapshotEpoch = X
//     SnapshotSegment = B+1
//     SnapshotObjects = W
//   ...
// Segment C (>= B+k)
//   ...
//   State (Snapshot committed)
//     CurrentEpoch = X+1
//     CurrentSegment = C
//     CurrentObjects = <current objects right now>
//     SnapshotEpoch = X+1
//     SnapshotSegment = B+1
//     SnapshotObjects = <object snapshots in B+1..C>
//   ...

type State struct {
	Head Ref `json:"head"`
	Tail Ref `json:"tail"`

	Entries         uint64 `json:"entries"`
	SnapshotEpoch   Epoch  `json:"snapshot_epoch"`
	SnapshotOffset  uint64 `json:"snapshot_offset"`
	SnapshotObjects uint64 `json:"snapshot_objects"`
}

type Entry interface {
	GetMethod() string
}

type Object interface {
	GetEpochParity() bool
	SetEpochParity(bool)
	SaveSnapshot() any
}
