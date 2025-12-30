// Copyright (c) 2025 Darren Soothill (darren [at] soothill [dot] com)

package pipeline

import (
	"fmt"
	"log"
	"log/syslog"
	"sync"
)

var (
	syslogOnce   sync.Once
	syslogWriter *syslog.Writer
)

// LogInfof records informational events and mirrors them to syslog to make startup/shutdown visible.
func LogInfof(format string, args ...interface{}) {
	logInfof(format, args...)
}

// LogFailuref records an error and mirrors it to syslog so host tooling can alert on failures.
func LogFailuref(format string, args ...interface{}) {
	logFailuref(format, args...)
}

func logInfof(format string, args ...interface{}) {
	log.Printf(format, args...)
	if writer := getSyslogWriter(); writer != nil {
		_ = writer.Info(fmt.Sprintf(format, args...))
	}
}

func logFailuref(format string, args ...interface{}) {
	log.Printf(format, args...)
	if writer := getSyslogWriter(); writer != nil {
		_ = writer.Err(fmt.Sprintf(format, args...))
	}
}

func getSyslogWriter() *syslog.Writer {
	syslogOnce.Do(func() {
		writer, err := syslog.New(syslog.LOG_DAEMON|syslog.LOG_ERR, "icnt-tick-data")
		if err != nil {
			log.Printf("syslog setup failed: %v", err)
			return
		}
		syslogWriter = writer
	})
	return syslogWriter
}
