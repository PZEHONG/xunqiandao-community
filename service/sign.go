// service/sign.go
package service

import (
	"context"
	"errors"
	"io"
)

var ErrSignInput = errors.New("签到参数错误")
var ErrSignAccount = errors.New("请先连接习讯云账号")
var ErrSignCancelled = errors.New("每日任务已暂停或修改，本次跳过")

const MaxSignPhotoSize int64 = 10 << 20
const MaxSignPhotos = 3

type SignRemark struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type SignOptions struct {
	BeforeSubmit        func() error `json:"-"`
	OnSubmitting        func()       `json:"-"`
	ExpectedSchoolID    string       `json:"-"`
	ExpectedRemarkLabel string       `json:"-"`
	Remark              string       `json:"remark"`
	Comment             string       `json:"comment"`
	Photos              []string     `json:"photos"`
}

// SignService 定义执行签到操作的接口
type SignService interface {
	SignIn(account, address, addressName, latitude, longitude string) (string, error)
	SignInWithOptions(context.Context, string, string, string, string, string, SignOptions) (string, error)
	Remarks(context.Context, string) ([]SignRemark, error)
	UploadPhoto(context.Context, string, string, io.Reader) (string, error)
}
