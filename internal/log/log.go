package log

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"

	"github.com/OpenSourceOptimist/skyflow/internal/event"
	"github.com/OpenSourceOptimist/skyflow/internal/messages"
	"github.com/OpenSourceOptimist/skyflow/internal/slice"
	"github.com/sirupsen/logrus"
)

type Logger struct {
	Session          string
	sessionIncrement int64
}

func (l *Logger) Debug(msg string, keyVals ...interface{}) {
	if caller, ok := callerFile(); ok {
		keyVals = append(keyVals, "caller", caller)
	}
	logrus.
		WithFields(l.fields(keyVals)).
		Debug(msg)
}

func (l *Logger) Error(msg string, keyVals ...interface{}) {
	if caller, ok := callerFile(); ok {
		keyVals = append(keyVals, "caller", caller)
	}
	logrus.
		WithFields(l.fields(keyVals)).
		Error(msg)
}

func (l *Logger) Info(msg string, keyVals ...interface{}) {
	if caller, ok := callerFile(); ok {
		keyVals = append(keyVals, "caller", caller)
	}
	logrus.
		WithFields(l.fields(keyVals)).
		Info(msg)
}

func callerFile() (string, bool) {
	_, filename, linenumber, ok := runtime.Caller(2)
	if !ok {
		return "", false
	}
	fileNameWithoutPath, ok := slice.End(strings.Split(filename, "/"))
	if !ok {
		return "", false
	}
	return fmt.Sprintf("%s:%d", fileNameWithoutPath, linenumber), true
}

func (l *Logger) IncrementSession() {
	l.sessionIncrement++
}

func (l *Logger) fields(keyVals []interface{}) logrus.Fields {
	fields := make(map[string]interface{})
	if l.Session != "" {
		fields["session"] = fmt.Sprintf("%s-%d", l.Session, l.sessionIncrement)
	}
	for _, keyVal := range slice.Chunk(keyVals, 2) {
		if len(keyVal) != 2 {
			continue
		}
		key, ok := keyVal[0].(string)
		if !ok {
			l.Error("debug error, key not string", "key", keyVal[0], "value", keyVal[1])
			continue
		}
		if val, ok := keyVal[1].(string); ok {
			fields[key] = val
		} else if err, ok := keyVal[1].(error); ok {
			fields[key] = err.Error()
		} else if id, ok := keyVal[1].(event.ID); ok {
			fields[key] = slice.Prefix(id, 5)
		} else if sub, ok := keyVal[1].(messages.SubscriptionID); ok {
			fields[key] = slice.Prefix(sub, 5)
		} else {
			fields[key] = marshall(keyVal[1])
		}
	}
	return fields
}

func marshall[T any](t T) string {
	bytes, err := json.Marshal(t)
	if err != nil {
		return err.Error()
	}
	return string(bytes)
}
