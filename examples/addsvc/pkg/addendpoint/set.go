package addendpoint

import (
	"context"
	"time"

	"golang.org/x/time/rate"

	stdopentracing "github.com/opentracing/opentracing-go"
	stdzipkin "github.com/openzipkin/zipkin-go"
	"github.com/sony/gobreaker"

	"github.com/go-kit/kit/circuitbreaker"
	"github.com/go-kit/kit/endpoint"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/ratelimit"
	"github.com/go-kit/kit/tracing/opentracing"
	"github.com/go-kit/kit/tracing/zipkin"

	"github.com/go-kit/kit/examples/addsvc/pkg/addservice"
)

// Set collects all of the endpoints that compose an add service. It's meant to
// be used as a helper struct, to collect all of the endpoints into a single
// parameter.
//集合收集组成添加服务的所有端点。它意味着被用作辅助结构，将所有端点收集到一个参数中。
type Set struct {
	SumEndpoint    endpoint.Endpoint
	ConcatEndpoint endpoint.Endpoint
}

// New returns a Set that wraps the provided server, and wires in all of the
// expected endpoint middlewares via the various parameters.
//New返回一个包装提供的服务器的Set，并通过各种参数连接所有预期的端点中间件。
func New(svc addservice.Service, logger log.Logger, duration metrics.Histogram, otTracer stdopentracing.Tracer, zipkinTracer *stdzipkin.Tracer) Set {
	var sumEndpoint endpoint.Endpoint
	{
		sumEndpoint = MakeSumEndpoint(svc)
		sumEndpoint = ratelimit.NewErroringLimiter(rate.NewLimiter(rate.Every(time.Second), 10))(sumEndpoint)
		sumEndpoint = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{}))(sumEndpoint)
		sumEndpoint = opentracing.TraceServer(otTracer, "Sum")(sumEndpoint)
		sumEndpoint = zipkin.TraceEndpoint(zipkinTracer, "Sum")(sumEndpoint)
		sumEndpoint = LoggingMiddleware(log.With(logger, "method", "Sum"))(sumEndpoint)
		sumEndpoint = InstrumentingMiddleware(duration.With("method", "Sum"))(sumEndpoint)
	}
	var concatEndpoint endpoint.Endpoint
	{
		concatEndpoint = MakeConcatEndpoint(svc)
		concatEndpoint = ratelimit.NewErroringLimiter(rate.NewLimiter(rate.Every(time.Second), 100))(concatEndpoint)
		concatEndpoint = circuitbreaker.Gobreaker(gobreaker.NewCircuitBreaker(gobreaker.Settings{}))(concatEndpoint)
		concatEndpoint = opentracing.TraceServer(otTracer, "Concat")(concatEndpoint)
		concatEndpoint = zipkin.TraceEndpoint(zipkinTracer, "Concat")(concatEndpoint)
		concatEndpoint = LoggingMiddleware(log.With(logger, "method", "Concat"))(concatEndpoint)
		concatEndpoint = InstrumentingMiddleware(duration.With("method", "Concat"))(concatEndpoint)
	}
	return Set{
		SumEndpoint:    sumEndpoint,
		ConcatEndpoint: concatEndpoint,
	}
}

// Sum implements the service interface, so Set may be used as a service.
//Sum实现了服务接口，所以Set可以用作服务。
// This is primarily useful in the context of a client library.
//这在客户端库的上下文中非常有用。
func (s Set) Sum(ctx context.Context, a, b int) (int, error) {
	resp, err := s.SumEndpoint(ctx, SumRequest{A: a, B: b})
	if err != nil {
		return 0, err
	}
	response := resp.(SumResponse)
	return response.V, response.Err
}

// Concat implements the service interface, so Set may be used as a
// service. This is primarily useful in the context of a client library.
//Concat实现了服务接口，所以Set可以用作服务。这在客户端库的上下文中非常有用
func (s Set) Concat(ctx context.Context, a, b string) (string, error) {
	resp, err := s.ConcatEndpoint(ctx, ConcatRequest{A: a, B: b})
	if err != nil {
		return "", err
	}
	response := resp.(ConcatResponse)
	return response.V, response.Err
}

// MakeSumEndpoint constructs a Sum endpoint wrapping the service.
// MakeSumEndpoint构造一个包装服务的Sum端点。
func MakeSumEndpoint(s addservice.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(SumRequest)
		v, err := s.Sum(ctx, req.A, req.B)
		return SumResponse{V: v, Err: err}, nil
	}
}

// MakeConcatEndpoint constructs a Concat endpoint wrapping the service.
// MakeConcatEndpoint构造一个包装服务的Concat端点
func MakeConcatEndpoint(s addservice.Service) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (response interface{}, err error) {
		req := request.(ConcatRequest)
		v, err := s.Concat(ctx, req.A, req.B)
		return ConcatResponse{V: v, Err: err}, nil
	}
}

// Failer is an interface that should be implemented by response types.
// Failer是一个应该由响应类型实现的接口。
// Response encoders can check if responses are Failer, and if so if they've
// failed, and if so encode them using a separate write path based on the error.
//响应编码器可以检查响应是否为Failer，如果是这样，如果它们失败了，并且如果是这样，则使用基于错误的单独写入路径对它们进行编码。
type Failer interface {
	Failed() error
}

// SumRequest collects the request parameters for the Sum method.
//SumRequest收集Sum方法的请求参数。
type SumRequest struct {
	A, B int
}

// SumResponse collects the response values for the Sum method.
type SumResponse struct {
	V   int   `json:"v"`
	Err error `json:"-"` // should be intercepted by Failed/errorEncoder
}

// Failed implements Failer.
func (r SumResponse) Failed() error { return r.Err }

// ConcatRequest collects the request parameters for the Concat method.
type ConcatRequest struct {
	A, B string
}

// ConcatResponse collects the response values for the Concat method.
type ConcatResponse struct {
	V   string `json:"v"`
	Err error  `json:"-"`
}

// Failed implements Failer.
func (r ConcatResponse) Failed() error { return r.Err }
