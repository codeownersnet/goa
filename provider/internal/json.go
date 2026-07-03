package internal

import (
	"encoding/json"
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
