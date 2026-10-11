package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// jsonObject is a JSON object that remembers its key order and keeps each
// value as the exact bytes it was decoded from. Rewriting a settings.json
// through it changes only the keys that were set; everything else keeps
// its position and content.
type jsonObject struct {
	keys   []string
	values map[string]json.RawMessage
}

// errNotObject reports JSON input that is valid but is not an object.
var errNotObject = errors.New("not a JSON object")

// UnmarshalJSON decodes a JSON object, recording keys in file order. A
// repeated key keeps its first position and its last value, matching
// encoding/json's last-wins rule for maps.
func (o *jsonObject) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return errNotObject
	}

	o.keys = nil
	o.values = make(map[string]json.RawMessage)
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := tok.(string)
		if !ok {
			return fmt.Errorf("object key is %T, want string", tok)
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return err
		}
		o.set(key, value)
	}
	if _, err := dec.Token(); err != nil { // closing '}'
		return err
	}
	return nil
}

// get returns the raw value for key and whether the key is present.
func (o *jsonObject) get(key string) (json.RawMessage, bool) {
	v, ok := o.values[key]
	return v, ok
}

// set stores value under key. An existing key keeps its position; a new
// key goes at the end.
func (o *jsonObject) set(key string, value json.RawMessage) {
	if o.values == nil {
		o.values = make(map[string]json.RawMessage)
	}
	if _, ok := o.values[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

// encode returns the object as compact JSON in key order. Keys are encoded
// without HTML escaping and values are copied byte for byte, so the result
// carries no escaping the input did not have.
func (o *jsonObject) encode() (json.RawMessage, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, key := range o.keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		encodedKey, err := marshalUnescaped(key)
		if err != nil {
			return nil, err
		}
		buf.Write(encodedKey)
		buf.WriteByte(':')
		buf.Write(o.values[key])
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// marshalUnescaped is json.Marshal without HTML escaping of '<', '>' and
// '&'. json.RawMessage values inside v are copied without escaping too.
func marshalUnescaped(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// jsonKind returns the first byte of raw, a valid JSON value, after leading
// whitespace: '{' for an object, '[' for an array, '"' for a string, and so
// on. It returns 0 for empty input.
func jsonKind(raw json.RawMessage) byte {
	trimmed := bytes.TrimLeft(raw, " \t\r\n")
	if len(trimmed) == 0 {
		return 0
	}
	return trimmed[0]
}
