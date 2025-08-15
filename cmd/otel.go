package main

import (
	"context"
	"errors"

	"github.com/go-logr/logr"
	"go.opentelemetry.io/contrib/bridges/otellogr"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
)

type shutdownFunc func(context.Context) error

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
// nolint: nonamedreturns
func setupOTelSDK(ctx context.Context) (shutdown shutdownFunc, err error) {
	var shutdownFuncs []shutdownFunc

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	if !otelSdkEnabled() {
		return shutdown, nil
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	resourceWAttributes, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("mdai-logstream", "hub"),
	))
	if err != nil {
		panic(err)
	}

	operatorResource, err := resource.Merge(
		resource.Default(),
		resourceWAttributes,
	)
	if err != nil {
		panic(err)
	}

	// Set up logger provider.
	loggerProvider, err := newLoggerProvider(ctx, operatorResource)
	if err != nil {
		handleErr(err)
		return shutdown, err
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	return shutdown, err
}

func newLoggerProvider(ctx context.Context, res *resource.Resource) (*log.LoggerProvider, error) {
	logExporter, err := otlploghttp.New(ctx)
	if err != nil {
		return nil, err
	}

	loggerProvider := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(logExporter)),
		log.WithResource(res),
	)
	return loggerProvider, nil
}

type LogrLogHandler struct {
	logger logr.Logger
}

func (errorHandler LogrLogHandler) Handle(err error) {
	errorHandler.logger.Error(err, "OTEL SDK error")
}

func attachOtelLogger(logger logr.Logger) logr.Logger {
	// Bootstrap OTEL's own error handling with this logger, so we're not attempting to log OTEL's errors to itself
	otel.SetErrorHandler(&LogrLogHandler{logger})
	mainSink := logger.GetSink()
	var otelSink logr.LogSink = otellogr.NewLogSink("github.com/decisiveai/mdai-operator")
	otelifiedLogger := logger.WithSink(otelMirrorSink{mainSink, otelSink})
	return otelifiedLogger
}

type otelMirrorSink struct {
	mainSink logr.LogSink
	otelSink logr.LogSink
}

func (o otelMirrorSink) Init(info logr.RuntimeInfo) {
	o.mainSink.Init(info)
	if o.otelSink != nil {
		o.otelSink.Init(info)
	}
}

func (o otelMirrorSink) Enabled(level int) bool {
	return o.mainSink.Enabled(level)
}

func (o otelMirrorSink) Info(level int, msg string, keysAndValues ...any) {
	o.mainSink.Info(level, msg, keysAndValues...)
	if o.otelSink != nil {
		o.otelSink.Info(level, msg, keysAndValues...)
	}
}

func (o otelMirrorSink) Error(err error, msg string, keysAndValues ...any) {
	o.mainSink.Error(err, msg, keysAndValues...)
	if o.otelSink != nil {
		o.otelSink.Error(err, msg, keysAndValues...)
	}
}

func (o otelMirrorSink) WithValues(kv ...any) logr.LogSink {
	var newOtel logr.LogSink
	if o.otelSink != nil {
		newOtel = o.otelSink.WithValues(kv...)
	}
	return otelMirrorSink{
		mainSink: o.mainSink.WithValues(kv...),
		otelSink: newOtel,
	}
}

func (o otelMirrorSink) WithName(name string) logr.LogSink {
	var newOtel logr.LogSink
	if o.otelSink != nil {
		newOtel = o.otelSink.WithName(name)
	}
	return otelMirrorSink{
		mainSink: o.mainSink.WithName(name),
		otelSink: newOtel,
	}
}
