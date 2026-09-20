# Journal design

The journal is an ordered log of object creates, changes, deletes, snapshots,
and journal state records. Object identity and content belong to the record
payload; the journal tracks metadata and object counts without interpreting
application data.

`State` tracks the current epoch, segment, and object count, along with the
snapshot epoch, starting segment, and number of captured objects. A state
checkpoint records this metadata; it is not a snapshot of the objects.

The intended snapshot lifecycle is:

1. Reserve snapshot segments, advance the current epoch, and direct new changes
   to segments after the reserved range.
2. Write object snapshots into the reserved segments and checkpoint progress as
   each segment is committed. Each object stores one epoch parity bit to help
   track its snapshot generation.
3. Once the snapshot is complete, record its commitment by advancing the snapshot
   epoch to the current epoch. Earlier segments can then be reclaimed after the
   snapshot and its commitment are durable.

Recovery is intended to restore a completed snapshot and replay subsequent
changes. Coordination between object updates and snapshot capture, recovery,
and segment reclamation remain to be implemented.

The JSON prototype writes one record per line, with a `header` and a `data`
payload. Record types are numeric: change `0`, snapshot `1`, create `2`, delete
`3`, and journal state `4`. For example, an application could write:

```json
{"header":{"Type":2},"data":{"id":"example","value":"hello"}}
```

`JSONRecord` captures data at construction and retains it as `json.RawMessage`
until decoded into an application type. `JSONReader` reads records sequentially;
it does not apply them to objects.

`JSONJournal` starts a single segment with a caller-supplied state checkpoint.
Appends accept creates, changes, and deletes, updating the object count only
after a complete write. Any write failure prevents further writes because the
stream may contain a partial record. Snapshot methods currently return
`ErrSnapshotsUnsupported`.

The caller serializes access and owns flushing, syncing, and closing the writer.
A successful append or checkpoint does not guarantee durability. Opening an
existing journal, replay, and recovery of a truncated final record are outside
the current prototype.
