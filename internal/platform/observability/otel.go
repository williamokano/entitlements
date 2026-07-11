package observability

import (
	"context"
	"errors"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Options configures OpenTelemetry setup.
type Options struct {
	ServiceName string
	// OTLPEndpoint is the OTLP/HTTP collector URL. When empty, no exporter is
	// installed: spans and metrics are created but dropped (no-op default).
	OTLPEndpoint string
}

// SetupOTel installs global tracer and meter providers and a W3C trace-context
// propagator. It returns a shutdown function that flushes and stops both
// providers. With no OTLP endpoint configured, telemetry is a no-op.
func SetupOTel(ctx context.Context, opts Options) (shutdown func(context.Context) error, err error) {
	res := resource.NewSchemaless(attribute.String("service.name", opts.ServiceName))

	var shutdowns []func(context.Context) error

	traceOpts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if opts.OTLPEndpoint != "" {
		exp, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(opts.OTLPEndpoint))
		if err != nil {
			return nil, err
		}
		traceOpts = append(traceOpts, sdktrace.WithBatcher(exp))
	}
	tp := sdktrace.NewTracerProvider(traceOpts...)
	otel.SetTracerProvider(tp)
	shutdowns = append(shutdowns, tp.Shutdown)

	meterOpts := []sdkmetric.Option{sdkmetric.WithResource(res)}
	if opts.OTLPEndpoint != "" {
		mexp, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(opts.OTLPEndpoint))
		if err != nil {
			return nil, err
		}
		meterOpts = append(meterOpts, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(mexp)))
	}
	mp := sdkmetric.NewMeterProvider(meterOpts...)
	otel.SetMeterProvider(mp)
	shutdowns = append(shutdowns, mp.Shutdown)

	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{}))

	return func(ctx context.Context) error {
		var err error
		for _, s := range shutdowns {
			err = errors.Join(err, s(ctx))
		}
		return err
	}, nil
}
