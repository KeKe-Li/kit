package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/go-kit/kit/endpoint"
	httptransport "github.com/go-kit/kit/transport/http"
)

//业务逻辑 在go-kit中，我们把服务模型定义为一个接口,接口中的方法代表了服务提供的功能：
//该服务提供两个功能：字符串大写化，计算字符串中字符数
// StringService provides operations on strings.
type StringService interface {
	Uppercase(string) (string, error)
	Count(string) int
}

//接口实现
type stringService struct{}


//处理Uppercase业务
func (stringService) Uppercase(s string) (string, error) {
	if s == "" {
		return "", ErrEmpty
	}
	return strings.ToUpper(s), nil
}
//处理Count业务
func (stringService) Count(s string) int {
	return len(s)
}

func main() {
	svc := stringService{}

	uppercaseHandler := httptransport.NewServer(
		makeUppercaseEndpoint(svc),
		decodeUppercaseRequest,
		encodeResponse,
	)

	countHandler := httptransport.NewServer(
		makeCountEndpoint(svc),
		decodeCountRequest,
		encodeResponse,
	)

	http.Handle("/uppercase", uppercaseHandler)
	http.Handle("/count", countHandler)
	log.Fatal(http.ListenAndServe(":8080", nil))
}

//go-kit中，如果使用go-kit/kit/transport/http，那么还需要把StringService封装为endpoint来供调用。
//抽象 uppercase的RPC调用
func makeUppercaseEndpoint(svc StringService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(uppercaseRequest)
		v, err := svc.Uppercase(req.S)
		if err != nil {
			return uppercaseResponse{v, err.Error()}, nil
		}
		return uppercaseResponse{v, ""}, nil
	}
}

//RPC调用封装成了更加通用的接口，输入参数和输出参数都为interface
//抽象 len的RPC调用
func makeCountEndpoint(svc StringService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(countRequest)
		v := svc.Count(req.S)
		return countResponse{v}, nil
	}
}

//从Request解码输入参数，编码输出到ResponseWriter
//第二步就是调用的是上面生成的endpoint，第一步需要我们传入解码器，用于将Request解码为输入参数，第三部需要我们传入编码器，输出到ResponseWriter。
//Uppercase输入解码器
func decodeUppercaseRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var request uppercaseRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return nil, err
	}
	return request, nil
}

//Count输入解码器
func decodeCountRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var request countRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return nil, err
	}
	return request, nil
}

//由于Uppercase和Count对输出的处理一样，所以可以用一个通用的编码器，将结果写入到ResponseWriter
func encodeResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	return json.NewEncoder(w).Encode(response)
}

//go kit中主要的消息传递方式为RPC，所以接口中的每一个方法都要用 remote procedure call 实现。
//对每一个方法，我们要定义对应的request和response方法，request用来获取入参，response用来传递输出参数。

//定义Uppercase的输入参数的结构
type uppercaseRequest struct {
	S string `json:"s"`
}

//定义Uppercase的输出接口
type uppercaseResponse struct {
	V   string `json:"v"`
	Err string `json:"err,omitempty"` // errors don't define JSON marshaling
}


//定义Count的输入参数结构
type countRequest struct {
	S string `json:"s"`
}

//定义Count的输入结构
type countResponse struct {
	V int `json:"v"`
}

// ErrEmpty is returned when an input string is empty.
var ErrEmpty = errors.New("empty string")
