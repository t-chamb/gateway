// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"go.githedgehog.com/gateway/pkg/agent"
	"go.githedgehog.com/gateway/pkg/version"
	"k8s.io/klog/v2"
	kctrl "sigs.k8s.io/controller-runtime"
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

	slog.SetDefault(slog.New(tint.NewHandler(logW, &tint.Options{
		Level:      logLevel,
		TimeFormat: time.StampMilli,
		NoColor:    !isatty.IsTerminal(logW.Fd()),
	})))

	kubeHandler := tint.NewHandler(logW, &tint.Options{
		Level:      slog.LevelInfo,
		TimeFormat: time.StampMilli,
		NoColor:    !isatty.IsTerminal(logW.Fd()),
	})
	kctrl.SetLogger(logr.FromSlogHandler(kubeHandler))
	klog.SetSlogLogger(slog.New(kubeHandler))

	args := []any{
		"version", version.Version,
	}

	if len(os.Args) != 1 {
		return fmt.Errorf("usage: %s", os.Args[0]) //nolint:goerr113
	}

	slog.Info("Hedgehog Gateway Agent", args...)

	return agent.New().Run(ctx) //nolint:wrapcheck
}
