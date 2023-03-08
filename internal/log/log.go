package log

import (
	"encoding/json"
)

func Marshall[T any](t T) string {
	bytes, err := json.Marshal(t)
	if err != nil {
		return err.Error()
	}
	return string(bytes)
}
