// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"google.golang.org/grpc/grpclog"
)

const (
	grpcInfoLog int = iota
	grpcWarningLog
	grpcErrorLog
	grpcFatalLog
)

type grpcSlog struct {
	l     *slog.Logger
	level slog.Level
}

func NewGRPCLogger(l *slog.Logger, level slog.Level) grpclog.LoggerV2 {
	return &grpcSlog{l: l, level: level}
}

var _ grpclog.LoggerV2 = (*grpcSlog)(nil)

func (g *grpcSlog) log(level slog.Level, msg string) {
	ctx := context.Background()

	if level < g.level || !g.l.Enabled(ctx, level) {
		return
	}

	var pc uintptr
	// TODO do we actually need this? straight from the slog package
	// if !internal.IgnorePC {
	// 	var pcs [1]uintptr
	// 	// skip [runtime.Callers, this function, this function's caller]
	// 	runtime.Callers(3, pcs[:])
	// 	pc = pcs[0]
	// }
	r := slog.NewRecord(time.Now(), level, msg, pc)
	_ = g.l.Handler().Handle(ctx, r)
}

func (g *grpcSlog) Error(args ...any) {
	g.log(slog.LevelError, fmt.Sprint(args...))
}

func (g *grpcSlog) Fatal(args ...any) {
	g.log(slog.LevelError, fmt.Sprint(args...))
	os.Exit(1)
}

func (g *grpcSlog) Info(args ...any) {
	g.log(slog.LevelInfo, fmt.Sprint(args...))
}

func (g *grpcSlog) Warning(args ...any) {
	g.log(slog.LevelWarn, fmt.Sprint(args...))
}

func (g *grpcSlog) Errorf(format string, args ...any) {
	g.log(slog.LevelError, fmt.Sprintf(format, args...))
}

func (g *grpcSlog) Fatalf(format string, args ...any) {
	g.log(slog.LevelError, fmt.Sprintf(format, args...))
	os.Exit(1)
}

func (g *grpcSlog) Infof(format string, args ...any) {
	g.log(slog.LevelInfo, fmt.Sprintf(format, args...))
}

func (g *grpcSlog) Warningf(format string, args ...any) {
	g.log(slog.LevelWarn, fmt.Sprintf(format, args...))
}

func (g *grpcSlog) Errorln(args ...any) {
	g.log(slog.LevelError, fmt.Sprint(args...))
}

func (g *grpcSlog) Fatalln(args ...any) {
	g.log(slog.LevelError, fmt.Sprint(args...))
	os.Exit(1)
}

func (g *grpcSlog) Infoln(args ...any) {
	g.log(slog.LevelInfo, fmt.Sprint(args...))
}

func (g *grpcSlog) Warningln(args ...any) {
	g.log(slog.LevelWarn, fmt.Sprint(args...))
}

func (g *grpcSlog) V(l int) bool {
	level := slog.LevelInfo
	switch l {
	case grpcInfoLog:
		level = slog.LevelInfo
	case grpcWarningLog:
		level = slog.LevelWarn
	case grpcErrorLog, grpcFatalLog:
		level = slog.LevelError
	}

	return level >= g.level && g.l.Enabled(context.Background(), level)
}
