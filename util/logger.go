package util

import (
	"fmt"
	"log"
	"os"
)

type Logger interface {
	Debug(v ...interface{})
	Debugf(format string, v ...interface{})
	Info(v ...interface{})
	Infof(format string, v ...interface{})
	Warn(v ...interface{})
	Warnf(format string, v ...interface{})
	Error(v ...interface{})
	Errorf(format string, v ...interface{})
	Fatal(v ...interface{})
	Fatalf(format string, v ...interface{})
}

type StdLogger interface {
	Fatal(v ...interface{})
	Fatalf(format string, v ...interface{})
	Print(v ...interface{})
	Println(v ...interface{})
	Printf(format string, v ...interface{})
}

type Level uint8

const (
	LogLevel_DEBUG Level = iota
	LogLevel_INFO
	LogLevel_WARN
	LogLevel_ERROR
	LogLevel_FATAL
)

var (
	_ Logger    = &StdLogAdapter{}
	_ StdLogger = &StdLogAdapter{}
)

type StdLogAdapter struct {
	Level Level
	out   *log.Logger
}

func NewLogger(level Level) Logger {
	return &StdLogAdapter{out: log.New(os.Stdout, "", 0), Level: level}
}

func (s *StdLogAdapter) Debug(v ...interface{}) {
	if s.Level <= LogLevel_DEBUG {
		s.Println(append([]interface{}{"[DEBUG]"}, v...)...)
	}
}

func (s *StdLogAdapter) Debugf(format string, v ...interface{}) {
	if s.Level <= LogLevel_DEBUG {
		s.Printf(fmt.Sprintf("[DEBUG] %s", format), v...)
	}
}

func (s *StdLogAdapter) Info(v ...interface{}) {
	if s.Level <= LogLevel_INFO {
		s.Println(append([]interface{}{"[INFO]"}, v...)...)
	}
}

func (s *StdLogAdapter) Infof(format string, v ...interface{}) {
	if s.Level <= LogLevel_INFO {
		s.Printf(fmt.Sprintf("[INFO] %s", format), v...)
	}
}

func (s *StdLogAdapter) Warn(v ...interface{}) {
	if s.Level <= LogLevel_WARN {
		s.Println(append([]interface{}{"[WARN]"}, v...)...)
	}
}

func (s *StdLogAdapter) Warnf(format string, v ...interface{}) {
	if s.Level <= LogLevel_WARN {
		s.Printf(fmt.Sprintf("[WARN] %s", format), v...)
	}
}

func (s *StdLogAdapter) Error(v ...interface{}) {
	if s.Level <= LogLevel_ERROR {
		s.Println(append([]interface{}{"[ERROR]"}, v...)...)
	}
}

func (s *StdLogAdapter) Errorf(format string, v ...interface{}) {
	if s.Level <= LogLevel_ERROR {
		s.Printf(fmt.Sprintf("[ERROR] %s", format), v...)
	}
}

func (s *StdLogAdapter) Fatal(v ...interface{}) {
	s.Println(append([]interface{}{"[FATAL]"}, v...)...)
	os.Exit(1)
}

func (s *StdLogAdapter) Fatalf(format string, v ...interface{}) {
	s.Printf(fmt.Sprintf("[FATAL] %s", format), v...)
	os.Exit(1)
}

func (s *StdLogAdapter) Print(v ...interface{}) {
	s.out.Print(v...)
}

func (s *StdLogAdapter) Println(v ...interface{}) {
	s.out.Println(v...)
}

func (s *StdLogAdapter) Printf(format string, v ...interface{}) {
	s.out.Printf(format, v...)
}
