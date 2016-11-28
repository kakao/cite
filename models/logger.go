package models

import (
	"github.com/fluent/fluent-logger-golang/fluent"
	gologging "github.com/op/go-logging"
	"os"
	"sync"
)

var (
	fluentClientOnce sync.Once
	fluentClientInst *fluent.Fluent
)

const (
	LOG_FORMAT = `%{color}%{level:.4s} %{shortfile}%{color:reset} | %{message}`
)

func init() {
	gologging.SetFormatter(gologging.MustStringFormatter(LOG_FORMAT))

	stdoutLogger := gologging.MustGetLogger("stdout")
	loggerBackend := gologging.NewLogBackend(os.Stdout, "", 0)
	loggerBackendLeveled := gologging.AddModuleLevel(loggerBackend)
	stdoutLogger.SetBackend(loggerBackendLeveled)
}

type fluentLogger struct {
	client *fluent.Fluent
	tag    string
	args   map[string]interface{}
}

func NewFluentLogger(tag string, args map[string]interface{}) *gologging.Logger {
	fluentClientOnce.Do(func() {
		var err error
		fluentClientInst, err = fluent.New(fluent.Config{
			FluentHost:   Conf.Aggregator.Host,
			AsyncConnect: false,
		})
		if err != nil {
			panic(err)
		}
	})

	fluentLoggerInst := gologging.MustGetLogger("fluentd")
	fluentLoggerBackend := &fluentLogger{
		client: fluentClientInst,
		tag:    tag,
		args:   args,
	}
	fluentLoggerBackendLeveled := gologging.AddModuleLevel(fluentLoggerBackend)
	fluentLoggerInst.SetBackend(fluentLoggerBackendLeveled)

	return fluentLoggerInst
}

func (b *fluentLogger) Log(level gologging.Level, calldepth int, rec *gologging.Record) error {
	b.args["level"] = rec.Level.String()
	b.args["msg"] = rec.Message()
	return b.client.Post(b.tag, b.args)
}
