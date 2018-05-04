package main

import (
	"time"

	"github.com/go-kit/kit/log"
)


//loggingMiddleware 这个方法的缺点在于无论是uppercase还是count参数都是uppercase，通用的就没有办法针对每个功能进行定制日志。
//由于StringService是一个接口，所以我们只需要定义一个新的类型包装之前的StringService，实现StringService的接口，在实现过程中加入log。

type loggingMiddleware struct {
	logger log.Logger
	next   StringService
}

func (mw loggingMiddleware) Uppercase(s string) (output string, err error) {
	defer func(begin time.Time) {
		_ = mw.logger.Log(
			"method", "uppercase",
			"input", s,
			"output", output,
			"err", err,
			"took", time.Since(begin),
		)
	}(time.Now())

	output, err = mw.next.Uppercase(s)
	return
}

func (mw loggingMiddleware) Count(s string) (n int) {
	defer func(begin time.Time) {
		_ = mw.logger.Log(
			"method", "count",
			"input", s,
			"n", n,
			"took", time.Since(begin),
		)
	}(time.Now())

	n = mw.next.Count(s)
	return
}
