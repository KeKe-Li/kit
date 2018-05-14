package main

import (
	"errors"
	"strings"
)

// StringService provides operations on strings.
//服务起始于业务逻辑.在Go kit 中,我们让一个接口作为一个服务
type StringService interface {
	Uppercase(string) (string, error)
	Count(string) int
}

//接口实现
type stringService struct{}

func (stringService) Uppercase(s string) (string, error) {
	if s == "" {
		return "", ErrEmpty
	}
	return strings.ToUpper(s), nil
}

func (stringService) Count(s string) int {
	return len(s)
}

// ErrEmpty is returned when an input string is empty.
var ErrEmpty = errors.New("empty string")
