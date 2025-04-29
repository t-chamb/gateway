// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"go.githedgehog.com/gateway/pkg/agent"
	"go.githedgehog.com/gateway/pkg/version"
)

func main() {
	if err := Run(context.Background()); err != nil {
		// TODO what if slog isn't initialized yet?
		slog.Error(err.Error())
		os.Exit(1)
	}
}

func Run(ctx context.Context) error {
	logLevel := slog.LevelDebug

	logW := os.Stderr
	logger := slog.New(tint.NewHandler(logW, &tint.Options{
		Level:      logLevel,
		TimeFormat: time.TimeOnly,
		NoColor:    !isatty.IsTerminal(logW.Fd()),
	}))
	slog.SetDefault(logger)

	args := []any{
		"version", version.Version,
	}

	if len(os.Args) != 1 {
		return fmt.Errorf("usage: %s", os.Args[0]) //nolint:goerr113
	}

	slog.Info("Hedgehog Gateway Agent", args...)

	return agent.Run(ctx) //nolint:wrapcheck
}
