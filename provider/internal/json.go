package internal

import (
	"encoding/json"
	"fmt"
	"io"
)

func MustMarshal(v any) string {
	data, _ := json.Marshal(v)
	return string(data)
}

func DecodeJSON(r io.Reader, v any) error {
	return json.NewDecoder(r).Decode(v)
}

func DecodeJSONString(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}

func DecodeJSONLimited(r io.Reader, v any, maxBytes int) error {
	if maxBytes <= 0 {
		return DecodeJSON(r, v)
	}
	data, err := io.ReadAll(io.LimitReader(r, int64(maxBytes)+1))
	if err != nil {
		return err
	}
	if len(data) > maxBytes {
		return fmt.Errorf("response exceeded %d bytes", maxBytes)
	}
	return json.Unmarshal(data, v)
}
