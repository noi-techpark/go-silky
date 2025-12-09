// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package apigorowler

import (
	"fmt"
	"log"
	"os"
)

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warning(msg string, args ...any)
	Error(msg string, args ...any)
}

type stdLogger struct {
	logger *log.Logger
}

func NewDefaultLogger() Logger {
	return &stdLogger{
		logger: log.New(os.Stdout, "", log.LstdFlags),
	}
}

func (l *stdLogger) Info(msg string, args ...any) {
	l.logger.Println("[INFO]", fmt.Sprintf(msg, args...)+"\n")
}

func (l *stdLogger) Debug(msg string, args ...any) {
	l.logger.Println("[DEBUG]", fmt.Sprintf(msg, args...)+"\n")
}

func (l *stdLogger) Warning(msg string, args ...any) {
	l.logger.Println("[WARN]", fmt.Sprintf(msg, args...)+"\n")
}

func (l *stdLogger) Error(msg string, args ...any) {
	l.logger.Println("[ERROR]", fmt.Sprintf(msg, args...)+"\n")
}

type noopLogger struct {
	logger *log.Logger
}

func NewNoopLogger() Logger {
	return &noopLogger{
		logger: log.New(os.Stdout, "", log.LstdFlags),
	}
}

func (l *noopLogger) Info(msg string, args ...any) {
}

func (l *noopLogger) Debug(msg string, args ...any) {
}

func (l *noopLogger) Warning(msg string, args ...any) {
}

func (l *noopLogger) Error(msg string, args ...any) {
}
