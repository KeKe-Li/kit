package main

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-kit/kit/endpoint"
)
//Go-kit 大部分功能都会通过一个叫endpoint提供。

//一个endpoint代表一个RPC。也就是说，我们接口中的一个方法。
//我们将写一个适配器来将我们每一个服务的方法转换为一个endpoint.每一个适配器都会引用StringService类型的参数，并且返回与方法对应的endpoint.

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

func makeCountEndpoint(svc StringService) endpoint.Endpoint {
	return func(ctx context.Context, request interface{}) (interface{}, error) {
		req := request.(countRequest)
		v := svc.Count(req.S)
		return countResponse{v}, nil
	}
}

func decodeUppercaseRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var request uppercaseRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return nil, err
	}
	return request, nil
}

func decodeCountRequest(_ context.Context, r *http.Request) (interface{}, error) {
	var request countRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		return nil, err
	}
	return request, nil
}

func encodeResponse(_ context.Context, w http.ResponseWriter, response interface{}) error {
	return json.NewEncoder(w).Encode(response)
}

//go kit中主要的消息传递方式为RPC，所以接口中的每一个方法都要用 remote procedure call 实现。
//对每一个方法，我们要定义对应的request和response方法，request用来获取入参，response用来传递输出参数。
type uppercaseRequest struct {
	S string `json:"s"`
}

type uppercaseResponse struct {
	V   string `json:"v"`
	Err string `json:"err,omitempty"`
}

type countRequest struct {
	S string `json:"s"`
}

type countResponse struct {
	V int `json:"v"`
}
