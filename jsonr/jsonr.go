// Package jsonr encodes and decodes relaxed JSON using encoding/json/v2.
//
// In addition to standard JSON, decoding accepts //, /* */, and # comments,
// optional and trailing commas in objects and arrays, single-quoted strings, and
// bare strings matching [A-Za-z_$][A-Za-z0-9_$]*. The tokens true, false, and null
// retain their JSON meanings; object names with those spellings require quotes.
// Outside strings, structural punctuation ({}[]:,) may be backslash-escaped
// for shell completion. The backslashes are discarded while reading tokens.
// Single-quoted strings use JSON escapes and additionally allow \'. All other
// JSON syntax and the validation and Go value semantics of encoding/json/v2
// are preserved.
//
// Encoding omits commas between elements and members, and omits quotes from
// strings accepted as bare strings when their encoded content is at most 256
// bytes. Compact output uses spaces as separators. Other strings remain
// double-quoted. Comments, quote styles, and trailing commas are not preserved
// through a round trip.
// Both encoding/json/v2 and encoding/json/jsontext options are accepted.
// Decoder error byte offsets refer to the relaxed source.
package jsonr

import (
	"bufio"
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
)

// Marshal encodes in as relaxed JSON without commas or a terminating newline.
func Marshal(in any, opts ...json.Options) ([]byte, error) {
	b, err := json.Marshal(in, opts...)
	if err != nil {
		return nil, err
	}
	var w RelaxedWriter
	return w.appendRelaxed(b[:0], b), nil
}

// Unmarshal decodes exactly one relaxed JSON value into out, which must be a
// non-nil pointer. Surrounding whitespace and comments are allowed.
func Unmarshal(in []byte, out any, opts ...json.Options) error {
	src := NewRelaxedReader(bytes.NewReader(in))
	return src.SourceError(json.UnmarshalRead(src, out, opts...))
}

const maxUnquotedStringBytes = 256

// RelaxedWriter converts valid JSON to relaxed JSON, omitting commas and
// unquoting bare strings with at most 256 bytes of encoded content. It does not
// validate syntax. Short strings may be buffered across writes; complete JSON
// values need no additional flush.
type RelaxedWriter struct {
	out      io.Writer
	buf      []byte
	str      []byte // Encoded content and surrounding quotes.
	inString bool
	escaped  bool
	quoted   bool
	comma    bool
	err      error
}

// NewRelaxedWriter writes relaxed JSON to out.
func NewRelaxedWriter(out io.Writer) *RelaxedWriter {
	return &RelaxedWriter{out: out}
}

// Write converts p to relaxed JSON. The returned count is the number of input
// bytes consumed. An output error is retained and returned by subsequent writes.
func (w *RelaxedWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if len(p) == 0 {
		return 0, nil
	}
	w.buf = w.appendRelaxed(w.buf[:0], p)
	if len(w.buf) > 0 {
		var n int
		n, w.err = w.out.Write(w.buf)
		if n < len(w.buf) && w.err == nil {
			w.err = io.ErrShortWrite
		}
	}
	return len(p), w.err
}

// appendString consumes the current string and returns the remaining input.
func (w *RelaxedWriter) appendString(out, p []byte) ([]byte, []byte) {
	for i, c := range p {
		if w.escaped {
			w.escaped = false
		} else if c == '\\' {
			w.escaped = true
		} else if c == '"' {
			w.inString = false
			if w.quoted {
				out = append(out, '"')
			} else {
				w.str = append(w.str, '"')
				decoded, err := jsontext.AppendUnquote(out, w.str)
				if err == nil && isBareString(decoded[len(out):]) {
					out = decoded
				} else {
					out = append(out, w.str...)
				}
			}
			return out, p[i+1:]
		}
		if !w.quoted && len(w.str) == maxUnquotedStringBytes+1 {
			out = append(out, w.str...)
			w.quoted = true
		}
		if w.quoted {
			out = append(out, c)
		} else {
			w.str = append(w.str, c)
		}
	}
	return out, nil
}

// appendRelaxed can convert a complete JSON value in place with out = p[:0].
func (w *RelaxedWriter) appendRelaxed(out, p []byte) []byte {
	if w.inString {
		out, p = w.appendString(out, p)
	}
	for len(p) > 0 {
		c := p[0]
		p = p[1:]
		if w.comma {
			if !isSpace(c) {
				out = append(out, ' ')
			}
			w.comma = false
		}
		switch c {
		case ',':
			w.comma = true
		case '"':
			w.inString = true
			w.quoted = false
			w.str = append(w.str[:0], '"')
			out, p = w.appendString(out, p)
		default:
			out = append(out, c)
		}
	}
	return out
}

// RelaxedReader transforms JSON tokens without syntax validation.
// Whitespace and comments between tokens are discarded.
type RelaxedReader struct {
	in   *bufio.Reader
	buf  []byte
	last byte
	err  error
	// Wrap input with "[]" or "{}".
	wrap     byte
	openWrap bool // Leave an implicit container open at EOF for completion.
	// Shift from input to output at the start of last token.
	lastShift int64
	lastStart int64 // Source byte offset at the start of the last token.
	triviaEnd int64 // Source byte offset after the last whitespace or comment.
	inOffset  int64
	outOffset int64
	inComment bool
}

func NewRelaxedReader(in io.Reader) *RelaxedReader {
	return &RelaxedReader{in: bufio.NewReader(in)}
}

func (r *RelaxedReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(r.buf) == 0 && r.err == nil {
		r.err = r.fill()
		if r.err == io.EOF {
			if r.outOffset == 0 && r.wrap != 0 {
				r.emit(r.wrap)
			}
			if !r.openWrap {
				switch r.wrap {
				case '[':
					r.emit(']')
				case '{':
					r.emit('}')
				}
			}
		}
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	if len(r.buf) == 0 {
		return n, r.err
	}
	return n, nil
}

func (r *RelaxedReader) SourceError(err error) error {
	switch e := err.(type) {
	case *jsontext.SyntacticError:
		copy := *e
		if r.err == io.EOF && copy.ByteOffset == r.outOffset {
			// EOF follows any discarded trailing trivia.
			copy.ByteOffset = r.inOffset
		} else {
			copy.ByteOffset -= r.lastShift
		}
		return &copy
	case *json.SemanticError:
		copy := *e
		copy.ByteOffset -= r.lastShift
		copy.Err = r.SourceError(e.Err)
		return &copy
	default:
		return err
	}
}

// Wrap encloses the input in an array ('[') or object ('{').
// Object wrapping is skipped when the first token, after whitespace and
// comments, is already '{'. Array wrapping always adds an outer array.
// Call it before reading; zero leaves the input unwrapped.
func (r *RelaxedReader) Wrap(wrap byte) {
	r.wrap = wrap
}

func (r *RelaxedReader) emit(b ...byte) {
	r.buf = append(r.buf, b...)
	r.outOffset += int64(len(b))
}

// peek recognizes escaped punctuation without consuming either source byte.
func (r *RelaxedReader) peek() (byte, error) {
	b, err := r.in.Peek(1)
	if err != nil {
		return 0, err
	}
	c := b[0]
	if c == '\\' {
		if pair, err := r.in.Peek(2); err == nil {
			switch pair[1] {
			case '{', '}', '[', ']', ':', ',':
				return pair[1], nil
			}
		}
	}
	return c, nil
}

func (r *RelaxedReader) readByte() (byte, error) {
	b, err := r.in.ReadByte()
	if err == nil {
		r.inOffset++
	}
	return b, err
}

func (r *RelaxedReader) fill() error {
	if ok, err := r.trivia(); ok || err != nil {
		return err
	}
	if r.outOffset == 0 && r.wrap == '[' {
		r.emit('[')
		return nil
	}
	c, err := r.peek()
	if err != nil {
		return err
	}
	if r.needsComma(c) {
		r.lastShift = r.outOffset - r.inOffset
		r.emit(',')
		r.last = ','
		return nil
	}
	startShift := r.outOffset - r.inOffset
	r.lastStart = r.inOffset
	b, err := r.readByte()
	if err != nil {
		return err
	}
	r.lastShift = startShift
	if b != c {
		// peek recognized escaped punctuation. Consume the punctuation too,
		// counting both source bytes for offsets and completion replacements.
		r.readByte() // already peeked
	}
	switch c {
	case '{':
		if r.outOffset == 0 && r.wrap == '{' {
			r.wrap = 0
		}
		r.emit(c)
	case '[', '}', ']', ':':
		r.emit(c)
	case ',':
		err = r.comma()
	case '"', '\'':
		err = r.string(c)
	default:
		// Leave room for an implicit object opener and the opening quote.
		r.buf = append(r.buf, 0, 0, c)
		for {
			var next byte
			next, err = r.peek()
			if err != nil || isDelimiter(next) {
				break
			}
			r.readByte() // already peeked
			r.buf = append(r.buf, next)
		}
		b := r.buf[2:]
		r.buf = r.buf[:0]
		if isBareString(b) {
			r.startString()
			r.emit(b...)
			r.emit('"')
		} else {
			r.emit(b...)
		}
	}
	r.last = c
	return err
}

func (r *RelaxedReader) needsComma(next byte) bool {
	// Adjacent values, including object names, need a separator.
	switch r.last {
	case 0, '{', '[', ':', ',':
		return false
	}
	return next != ',' && next != ':' && next != '}' && next != ']'
}

func (r *RelaxedReader) comma() error {
	// Keep the comma's width. The preceding token
	// keeps empty or repeated commas from disappearing during normalization.
	r.emit(',')
	for {
		ok, err := r.trivia()
		if err != nil && err != io.EOF {
			return err
		}
		if !ok || err == io.EOF {
			break
		}
	}
	next, err := r.peek()
	if err != nil && err != io.EOF {
		return err
	}
	switch r.last {
	case 0, '{', '[', ':', ',':
	default:
		if next == '}' || next == ']' || err == io.EOF && (r.wrap == '[' || r.wrap == '{') {
			r.buf[0] = ' '
		}
	}
	return err
}

func (r *RelaxedReader) startString() {
	if r.outOffset == 0 && r.wrap == '{' {
		r.emit('{')
		r.lastShift++
	}
	r.emit('"')
}

func (r *RelaxedReader) string(quote byte) error {
	r.startString()
	for {
		c, err := r.readByte()
		if err != nil {
			return err
		}
		switch {
		case c == quote:
			r.emit('"')
			return nil
		case c == '\\':
			next, err := r.readByte()
			if err != nil {
				r.emit('\\')
				return err
			}
			if quote == '\'' && next == '\'' {
				r.emit('\'')
			} else {
				r.emit('\\', next)
			}
		case c == '"': // unescaped double quote in a single-quoted string
			r.emit('\\')
			r.emit('"')
		default:
			r.emit(c)
		}
	}
}

// trivia discards one whitespace byte or comment. needsComma keeps adjacent
// values separate, including when a comment separates them, e.g. 1/*c*/2.
func (r *RelaxedReader) trivia() (bool, error) {
	c, err := r.peek()
	if err != nil {
		return false, err
	}
	switch {
	case isSpace(c):
		r.readByte()
	case c == '#':
		r.readByte()
		err = r.lineComment()
	case c == '/':
		prefix, peekErr := r.in.Peek(2)
		if peekErr != nil {
			if peekErr == io.EOF {
				return false, nil // let jsontext reject the lone slash
			}
			return false, peekErr
		}
		switch prefix[1] {
		case '/':
			r.in.Discard(2)
			r.inOffset += 2 // already peeked
			err = r.lineComment()
		case '*':
			r.in.Discard(2)
			r.inOffset += 2 // already peeked
			r.inComment = true
			start := len(r.buf)
			r.buf = append(r.buf, '/', '*')
			var prev byte
			for {
				c, err := r.readByte()
				if err != nil {
					// Leave an unfinished comment for jsontext to reject.
					if err == io.EOF {
						b := r.buf[start:]
						r.buf = r.buf[:start]
						r.emit(b...)
					} else {
						r.buf = r.buf[:start]
					}
					return true, err
				}
				r.buf = append(r.buf, c)
				if prev == '*' && c == '/' {
					r.inComment = false
					r.buf = r.buf[:start]
					break
				}
				prev = c
			}
		default:
			return false, nil
		}
	default:
		return false, nil
	}
	if r.last == 0 {
		r.lastShift = r.outOffset - r.inOffset
	}
	r.triviaEnd = r.inOffset
	return true, err
}

func (r *RelaxedReader) lineComment() error {
	r.inComment = true
	for {
		c, err := r.readByte()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if c == '\n' || c == '\r' {
			r.inComment = false
			return nil
		}
	}
}

func isDelimiter(c byte) bool {
	return isSpace(c) || c == '{' || c == '}' || c == '[' || c == ']' ||
		c == ',' || c == ':' || c == '"' || c == '\'' || c == '/' || c == '#'
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

func isBareString(b []byte) bool {
	switch string(b) {
	case "true", "false", "null":
		return false
	}
	return isIdentifier(b)
}

func isIdentifier(b []byte) bool {
	for i, c := range b {
		if c == '_' || c == '$' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return len(b) > 0
}
