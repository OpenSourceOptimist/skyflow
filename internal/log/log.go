package log

import "github.com/sirupsen/logrus"

func Info(msg string, keyvals ...interface{}) {
	entry(keyvals).Info(msg)
}

func Error(msg string, keyvals ...interface{}) {
	entry(keyvals).Error(msg)
}

func Debug(msg string, keyvals ...interface{}) {
	entry(keyvals).Debug(msg)
}

func entry(keyvals []interface{}) *logrus.Entry {
	logrus.SetFormatter(&logrus.JSONFormatter{})
	return logrus.WithFields(parse(keyvals))
}

func parse(keyvals ...interface{}) logrus.Fields {
	res := make(map[string]interface{})
	for i, _ := range keyvals {
		if i%2 == 1 {
			continue
		}
		key, ok := keyvals[i].(string)
		if !ok {
			continue
		}
		if len(keyvals) < i+1 {
			break
		}
		res[key] = keyvals[i+1]
	}
	return res
}
