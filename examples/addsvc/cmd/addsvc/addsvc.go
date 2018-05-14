package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"text/tabwriter"

	"github.com/apache/thrift/lib/go/thrift"
	lightstep "github.com/lightstep/lightstep-tracer-go"
	"github.com/oklog/oklog/pkg/group"
	stdopentracing "github.com/opentracing/opentracing-go"
	zipkin "github.com/openzipkin/zipkin-go"
	zipkinot "github.com/openzipkin/zipkin-go-opentracing"
	zipkinhttp "github.com/openzipkin/zipkin-go/reporter/http"
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"sourcegraph.com/sourcegraph/appdash"
	appdashot "sourcegraph.com/sourcegraph/appdash/opentracing"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
	"github.com/go-kit/kit/metrics/prometheus"
	kitgrpc "github.com/go-kit/kit/transport/grpc"

	addpb "github.com/go-kit/kit/examples/addsvc/pb"
	"github.com/go-kit/kit/examples/addsvc/pkg/addendpoint"
	"github.com/go-kit/kit/examples/addsvc/pkg/addservice"
	"github.com/go-kit/kit/examples/addsvc/pkg/addtransport"
	addthrift "github.com/go-kit/kit/examples/addsvc/thrift/gen-go/addsvc"
)

func main() {
	// Define our flags. Your service probably won't need to bind listeners for
	// *all* supported transports, or support both Zipkin and LightStep, and so
	// on, but we do it here for demonstration purposes.
	fs := flag.NewFlagSet("addsvc", flag.ExitOnError)
	var (
		debugAddr      = fs.String("debug.addr", ":8080", "Debug and metrics listen address")
		httpAddr       = fs.String("http-addr", ":8081", "HTTP listen address")
		grpcAddr       = fs.String("grpc-addr", ":8082", "gRPC listen address")
		thriftAddr     = fs.String("thrift-addr", ":8083", "Thrift listen address")
		jsonRPCAddr    = fs.String("jsonrpc-addr", ":8084", "JSON RPC listen address")
		thriftProtocol = fs.String("thrift-protocol", "binary", "binary, compact, json, simplejson")
		thriftBuffer   = fs.Int("thrift-buffer", 0, "0 for unbuffered")
		thriftFramed   = fs.Bool("thrift-framed", false, "true to enable framing")
		zipkinV2URL    = fs.String("zipkin-url", "", "Enable Zipkin v2 tracing (zipkin-go) using a Reporter URL e.g. http://localhost:9411/api/v2/spans")
		zipkinV1URL    = fs.String("zipkin-v1-url", "", "Enable Zipkin v1 tracing (zipkin-go-opentracing) using a collector URL e.g. http://localhost:9411/api/v1/spans")
		lightstepToken = fs.String("lightstep-token", "", "Enable LightStep tracing via a LightStep access token")
		appdashAddr    = fs.String("appdash-addr", "", "Enable Appdash tracing via an Appdash server host:port")
	)
	fs.Usage = usageFor(fs, os.Args[0]+" [flags]")
	fs.Parse(os.Args[1:])

	// Create a single logger, which we'll use and give to other components.
	//创建一个记录器，我们将使用该记录器并将其提供给其他组件。
	var logger log.Logger
	{
		logger = log.NewLogfmtLogger(os.Stderr)
		logger = log.With(logger, "ts", log.DefaultTimestampUTC)
		logger = log.With(logger, "caller", log.DefaultCaller)
	}

	// Determine which OpenTracing tracer to use. We'll pass the tracer to all the
	// components that use it, as a dependency.
	//确定要使用哪个OpenTracing示踪器。我们会将跟踪器传递给所有使用它的组件，作为依赖关系。
	var tracer stdopentracing.Tracer
	{
		if *zipkinV1URL != "" && *zipkinV2URL == "" {
			logger.Log("tracer", "Zipkin", "type", "OpenTracing", "URL", *zipkinV1URL)
			collector, err := zipkinot.NewHTTPCollector(*zipkinV1URL)
			if err != nil {
				logger.Log("err", err)
				os.Exit(1)
			}
			defer collector.Close()
			var (
				debug       = false
				hostPort    = "localhost:80"
				serviceName = "addsvc"
			)
			recorder := zipkinot.NewRecorder(collector, debug, hostPort, serviceName)
			tracer, err = zipkinot.NewTracer(recorder)
			if err != nil {
				logger.Log("err", err)
				os.Exit(1)
			}
		} else if *lightstepToken != "" {
			logger.Log("tracer", "LightStep") // probably don't want to print out the token :)
			tracer = lightstep.NewTracer(lightstep.Options{
				AccessToken: *lightstepToken,
			})
			defer lightstep.FlushLightStepTracer(tracer)
		} else if *appdashAddr != "" {
			logger.Log("tracer", "Appdash", "addr", *appdashAddr)
			tracer = appdashot.NewTracer(appdash.NewRemoteCollector(*appdashAddr))
		} else {
			tracer = stdopentracing.GlobalTracer() // no-op
		}
	}

	var zipkinTracer *zipkin.Tracer
	{
		var (
			err           error
			hostPort      = "localhost:80"
			serviceName   = "addsvc"
			useNoopTracer = (*zipkinV2URL == "")
			reporter      = zipkinhttp.NewReporter(*zipkinV2URL)
		)
		defer reporter.Close()
		zEP, _ := zipkin.NewEndpoint(serviceName, hostPort)
		zipkinTracer, err = zipkin.NewTracer(
			reporter, zipkin.WithLocalEndpoint(zEP), zipkin.WithNoopTracer(useNoopTracer),
		)
		if err != nil {
			logger.Log("err", err)
			os.Exit(1)
		}
		if !useNoopTracer {
			logger.Log("tracer", "Zipkin", "type", "Native", "URL", *zipkinV2URL)
		}
	}

	// Create the (sparse) metrics we'll use in the service. They, too, are
	// dependencies that we pass to components that use them.
	//创建我们将在服务中使用的健康指标。它们也是我们传递给使用它们的组件的依赖关系
	var ints, chars metrics.Counter
	{
		// Business-level metrics.
		//业务级指标
		ints = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: "example",
			Subsystem: "addsvc",
			Name:      "integers_summed",
			Help:      "Total count of integers summed via the Sum method.",
		}, []string{})
		chars = prometheus.NewCounterFrom(stdprometheus.CounterOpts{
			Namespace: "example",
			Subsystem: "addsvc",
			Name:      "characters_concatenated",
			Help:      "Total count of characters concatenated via the Concat method.",
		}, []string{})
	}
	var duration metrics.Histogram
	{
		// Endpoint-level metrics.
		//端点级指标。
		duration = prometheus.NewSummaryFrom(stdprometheus.SummaryOpts{
			Namespace: "example",
			Subsystem: "addsvc",
			Name:      "request_duration_seconds",
			Help:      "Request duration in seconds.",
		}, []string{"method", "success"})
	}
	http.DefaultServeMux.Handle("/metrics", promhttp.Handler())

	// Build the layers of the service "onion" from the inside out. First, the
	// business logic service; then, the set of endpoints that wrap the service;
	// and finally, a series of concrete transport adapters. The adapters, like
	// the HTTP handler or the gRPC server, are the bridge between Go kit and
	// the interfaces that the transports expect. Note that we're not binding
	// them to ports or anything yet; we'll do that next.
	var (
		service        = addservice.New(logger, ints, chars)
		endpoints      = addendpoint.New(service, logger, duration, tracer, zipkinTracer)
		httpHandler    = addtransport.NewHTTPHandler(endpoints, tracer, zipkinTracer, logger)
		grpcServer     = addtransport.NewGRPCServer(endpoints, tracer, zipkinTracer, logger)
		thriftServer   = addtransport.NewThriftServer(endpoints)
		jsonrpcHandler = addtransport.NewJSONRPCHandler(endpoints, logger)
	)

	// Now we're to the part of the func main where we want to start actually
	// running things, like servers bound to listeners to receive connections.
	//现在我们来看看func的主要部分，我们想要实际开始运行事情，比如绑定到监听器的服务器来接收连接。

	//
	// The method is the same for each component: add a new actor to the group
	// struct, which is a combination of 2 anonymous functions: the first
	// function actually runs the component, and the second function should
	// interrupt the first function and cause it to return. It's in these
	// functions that we actually bind the Go kit server/handler structs to the
	// concrete transports and run them.
	//每个组件的方法都是一样的：向组中添加一个新的actor结构，这是2个匿名函数的组合：第一个函数实际运行组件，第二个函数应该中断第一个函数并使其返回。
	//在这些函数中，我们实际上将Go kit服务器处理程序结构绑定到具体传输并运行它们。

	//
	// Putting each component into its own block is mostly for aesthetics: it
	// clearly demarcates the scope in which each listener/socket may be used.

	//将每个组件放到它自己的块中主要是为了美观：它明确地划分了每个侦听器/套接字可以使用的范围。
	var g group.Group
	{
		// The debug listener mounts the http.DefaultServeMux, and serves up
		// stuff like the Prometheus metrics route, the Go debug and profiling
		// routes, and so on.

		//调试监听器装载http.DefaultServeMux，并提供Prometheus指标路由，Go调试和分析路由等。
		debugListener, err := net.Listen("tcp", *debugAddr)
		if err != nil {
			logger.Log("transport", "debug/HTTP", "during", "Listen", "err", err)
			os.Exit(1)
		}
		g.Add(func() error {
			logger.Log("transport", "debug/HTTP", "addr", *debugAddr)
			return http.Serve(debugListener, http.DefaultServeMux)
		}, func(error) {
			debugListener.Close()
		})
	}
	{
		// The HTTP listener mounts the Go kit HTTP handler we created.
		//HTTP侦听器安装我们创建的Go kit HTTP处理程序。
		httpListener, err := net.Listen("tcp", *httpAddr)
		if err != nil {
			logger.Log("transport", "HTTP", "during", "Listen", "err", err)
			os.Exit(1)
		}
		g.Add(func() error {
			logger.Log("transport", "HTTP", "addr", *httpAddr)
			return http.Serve(httpListener, httpHandler)
		}, func(error) {
			httpListener.Close()
		})
	}
	{
		// The gRPC listener mounts the Go kit gRPC server we created.
		// gRPC监听器安装我们创建的Go kit gRPC服务器。
		grpcListener, err := net.Listen("tcp", *grpcAddr)
		if err != nil {
			logger.Log("transport", "gRPC", "during", "Listen", "err", err)
			os.Exit(1)
		}
		g.Add(func() error {
			logger.Log("transport", "gRPC", "addr", *grpcAddr)
			// we add the Go Kit gRPC Interceptor to our gRPC service as it is used by
			// the here demonstrated zipkin tracing middleware.
			baseServer := grpc.NewServer(grpc.UnaryInterceptor(kitgrpc.Interceptor))
			addpb.RegisterAddServer(baseServer, grpcServer)
			return baseServer.Serve(grpcListener)
		}, func(error) {
			grpcListener.Close()
		})
	}
	{
		// The Thrift socket mounts the Go kit Thrift server we created earlier.
		//Thrift socket安装了我们之前创建的Go kit Thrift服务器
		// There's a lot of boilerplate involved here, related to configuring
		// the protocol and transport; blame Thrift.
		//这里涉及很多文件范例，涉及配置协议和传输
		thriftSocket, err := thrift.NewTServerSocket(*thriftAddr)
		if err != nil {
			logger.Log("transport", "Thrift", "during", "Listen", "err", err)
			os.Exit(1)
		}
		g.Add(func() error {
			logger.Log("transport", "Thrift", "addr", *thriftAddr)
			var protocolFactory thrift.TProtocolFactory
			switch *thriftProtocol {
			case "binary":
				protocolFactory = thrift.NewTBinaryProtocolFactoryDefault()
			case "compact":
				protocolFactory = thrift.NewTCompactProtocolFactory()
			case "json":
				protocolFactory = thrift.NewTJSONProtocolFactory()
			case "simplejson":
				protocolFactory = thrift.NewTSimpleJSONProtocolFactory()
			default:
				return fmt.Errorf("invalid Thrift protocol %q", *thriftProtocol)
			}
			var transportFactory thrift.TTransportFactory
			if *thriftBuffer > 0 {
				transportFactory = thrift.NewTBufferedTransportFactory(*thriftBuffer)
			} else {
				transportFactory = thrift.NewTTransportFactory()
			}
			if *thriftFramed {
				transportFactory = thrift.NewTFramedTransportFactory(transportFactory)
			}
			return thrift.NewTSimpleServer4(
				addthrift.NewAddServiceProcessor(thriftServer),
				thriftSocket,
				transportFactory,
				protocolFactory,
			).Serve()
		}, func(error) {
			thriftSocket.Close()
		})
	}
	{
		httpListener, err := net.Listen("tcp", *jsonRPCAddr)
		if err != nil {
			logger.Log("transport", "JSONRPC over HTTP", "during", "Listen", "err", err)
			os.Exit(1)
		}
		g.Add(func() error {
			logger.Log("transport", "JSONRPC over HTTP", "addr", *jsonRPCAddr)
			return http.Serve(httpListener, jsonrpcHandler)
		}, func(error) {
			httpListener.Close()
		})
	}
	{
		// This function just sits and waits for ctrl-C.
		cancelInterrupt := make(chan struct{})
		g.Add(func() error {
			c := make(chan os.Signal, 1)
			signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
			select {
			case sig := <-c:
				return fmt.Errorf("received signal %s", sig)
			case <-cancelInterrupt:
				return nil
			}
		}, func(error) {
			close(cancelInterrupt)
		})
	}
	logger.Log("exit", g.Run())
}

func usageFor(fs *flag.FlagSet, short string) func() {
	return func() {
		fmt.Fprintf(os.Stderr, "USAGE\n")
		fmt.Fprintf(os.Stderr, "  %s\n", short)
		fmt.Fprintf(os.Stderr, "\n")
		fmt.Fprintf(os.Stderr, "FLAGS\n")
		w := tabwriter.NewWriter(os.Stderr, 0, 2, 2, ' ', 0)
		fs.VisitAll(func(f *flag.Flag) {
			fmt.Fprintf(w, "\t-%s %s\t%s\n", f.Name, f.DefValue, f.Usage)
		})
		w.Flush()
		fmt.Fprintf(os.Stderr, "\n")
	}
}
