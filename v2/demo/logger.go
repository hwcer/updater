package demo

import (
	"fmt"
	"github.com/hwcer/logger"
	"github.com/hwcer/updater"
)

func init() {
	updater.Logger = &log{}
}

type log struct {
}

func (l *log) Fatal(format any, args ...any) {
	logger.Fatal(format, args...)
}
func (l *log) Panic(format any, args ...any) {
	logger.Panic(format, args...)
}
func (l *log) Error(format any, args ...any) {
	fmt.Printf(logger.Sprintf(format, args...) + "\n")
}
func (l *log) Alert(format any, args ...any) {
	fmt.Printf(logger.Sprintf(format, args...) + "\n")
}
func (l *log) Debug(format any, args ...any) {
	fmt.Printf(logger.Sprintf(format, args...) + "\n")
}
func (l *log) Trace(format any, args ...any) {
	fmt.Printf(logger.Sprintf(format, args...) + "\n")
}
