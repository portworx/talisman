package k8sutils

import (
	"encoding/json"
	"fmt"

	"github.com/sirupsen/logrus"
)

// DebugDumpObjectJS dumps object on DEBUG
func DebugDumpObjectJS(obj interface{}, args ...interface{}) {
	if !logrus.IsLevelEnabled(logrus.DebugLevel) {
		return
	}
	dumpObjectJS(logrus.Debug, obj, args...)
}

// TraceDumpObjectJS dumps object on TRACE
func TraceDumpObjectJS(obj interface{}, args ...interface{}) {
	if !logrus.IsLevelEnabled(logrus.TraceLevel) {
		return
	}
	dumpObjectJS(logrus.Trace, obj, args...)
}

// dumpObjectJS dumps object in JSON if given debug-level is on
func dumpObjectJS(lvlFunc func(args ...interface{}), obj interface{}, args ...interface{}) {
	var (
		js  = []byte("<nil>")
		msg = "..."
		ok  bool
		err error
	)

	if obj != nil {
		if js, err = json.Marshal(obj); err != nil {
			logrus.Errorf("log(error marshaling object %T): %s", obj, err)
			return
		}
	}

	switch len(args) {
	case 0:
		msg = fmt.Sprintf("%T =", obj)
	case 1:
		if msg, ok = args[0].(string); !ok {
			logrus.Errorf("log(invalid argument type %T)", msg)
			return
		}
	default:
		msg = fmt.Sprintf(args[0].(string), args[1:]...)
	}
	lvlFunc("|/_  (cont below)")
	_, _ = fmt.Fprintln(logrus.StandardLogger().Out, ">>", msg, string(js))
}
