package main

import (
	"bufio"
	"encoding/json"
	"io"
	"strconv"
	"unicode/utf8"
)

// encoding/json builds the whole document in memory before writing it, so
// serving a tree of millions of nodes allocates a buffer of hundreds of MB
// (plus the slack from growing it). The functions below write the tree node
// by node through a small bufio.Writer instead; memory use is independent
// of the tree size. The output is the same JSON that encoding/json
// produces for FileNode.

const streamBufSize = 64 << 10

// writeTree writes n as JSON with the same field names as FileNode's tags.
func writeTree(w *bufio.Writer, n *FileNode) error {
	if n == nil {
		_, err := w.WriteString("null")
		return err
	}
	w.WriteString(`{"n":`)
	writeJSONString(w, n.Name)
	w.WriteString(`,"s":`)
	writeInt(w, n.Size)
	w.WriteString(`,"m":`)
	writeInt(w, n.Mtime)
	if n.IsDir {
		w.WriteString(`,"d":true`)
	}
	if len(n.Children) > 0 {
		w.WriteString(`,"c":[`)
		for i, c := range n.Children {
			if i > 0 {
				w.WriteByte(',')
			}
			if err := writeTree(w, c); err != nil {
				return err
			}
		}
		w.WriteByte(']')
	}
	return w.WriteByte('}')
}

// writeInt appends v into the writer's own free buffer space, avoiding a
// heap-allocated scratch array per call.
func writeInt(w *bufio.Writer, v int64) {
	w.Write(strconv.AppendInt(w.AvailableBuffer(), v, 10))
}

const hexDigits = "0123456789abcdef"

// writeJSONString writes s as a JSON string literal. Like encoding/json it
// escapes <, > and & for safe embedding in HTML, escapes U+2028/U+2029, and
// replaces invalid UTF-8 (possible in Linux file names) with U+FFFD.
func writeJSONString(w *bufio.Writer, s string) {
	w.WriteByte('"')
	start := 0
	for i := 0; i < len(s); {
		b := s[i]
		if b < utf8.RuneSelf {
			if b >= 0x20 && b != '"' && b != '\\' && b != '<' && b != '>' && b != '&' {
				i++
				continue
			}
			w.WriteString(s[start:i])
			switch b {
			case '"', '\\':
				w.WriteByte('\\')
				w.WriteByte(b)
			case '\n':
				w.WriteString(`\n`)
			case '\r':
				w.WriteString(`\r`)
			case '\t':
				w.WriteString(`\t`)
			default: // other control characters and <, >, &
				w.WriteString(`\u00`)
				w.WriteByte(hexDigits[b>>4])
				w.WriteByte(hexDigits[b&0xf])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			w.WriteString(s[start:i])
			w.WriteString(`�`)
			i += size
			start = i
			continue
		}
		if r == ' ' || r == ' ' {
			w.WriteString(s[start:i])
			w.WriteString(`\u202`)
			w.WriteByte(hexDigits[r&0xf])
			i += size
			start = i
			continue
		}
		i += size
	}
	w.WriteString(s[start:])
	w.WriteByte('"')
}

// writeObjectWithTree writes the small metadata object meta (encoded the
// usual way) with an extra field `key` holding the streamed tree.
func writeObjectWithTree(out io.Writer, meta map[string]interface{}, key string, tree *FileNode) error {
	head, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(out, streamBufSize)
	if len(head) > 2 { // non-empty object: drop the closing brace, add a comma
		w.Write(head[:len(head)-1])
		w.WriteByte(',')
	} else {
		w.WriteByte('{')
	}
	w.WriteString(strconv.Quote(key))
	w.WriteByte(':')
	if err := writeTree(w, tree); err != nil {
		return err
	}
	w.WriteString("}\n")
	return w.Flush()
}
