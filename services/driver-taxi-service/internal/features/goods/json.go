package goods

import (
	"encoding/json"
	"io"
)

func newJSONDecoder(reader io.Reader) *json.Decoder { return json.NewDecoder(reader) }
