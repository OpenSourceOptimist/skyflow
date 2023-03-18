package log

import (
	"encoding/json"

	"github.com/OpenSourceOptimist/skyflow/internal/slice"
	"github.com/sirupsen/logrus"
)

func Marshall[T any](t T) string {
	bytes, err := json.Marshal(t)
	if err != nil {
		return err.Error()
	}
	return string(bytes)
}

func Debug(msg string, keyVals ...interface{}) {
	logrus.
		WithFields(fields(keyVals)).
		Debug(msg)
}

func Error(msg string, keyVals ...interface{}) {
	logrus.
		WithFields(fields(keyVals)).
		Error(msg)
}

func Info(msg string, keyVals ...interface{}) {
	logrus.
		WithFields(fields(keyVals)).
		Info(msg)
}

func fields(keyVals []interface{}) logrus.Fields {
	fields := make(map[string]interface{})
	for _, keyVal := range slice.Chunk(keyVals, 2) {
		if len(keyVal) != 2 {
			continue
		}
		key := keyVal[0].(string)
		if val, ok := keyVal[1].(string); ok {
			fields[key] = val
		} else if err, ok := keyVal[1].(error); ok {
			fields[key] = err.Error()
		} else {
			val, err := json.Marshal(keyVal[1])
			if err != nil {
				panic(err.Error())
			}
			fields[key] = string(val)
		}
	}
	return fields
}
