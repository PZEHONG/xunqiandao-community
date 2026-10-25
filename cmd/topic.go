package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
)

type PushRequest struct {
	Title   string `json:"title"`   // 消息标题，必填
	Desp    string `json:"desp"`    // 消息内容，选填
	Short   string `json:"short"`   // 消息卡片内容，选填
	NoIP    string `json:"noip"`    // 是否隐藏调用 IP，选填
	Channel string `json:"channel"` // 动态指定消息通道，选填
	OpenID  string `json:"openid"`  // 消息抄送的 openid，选填
}

type PushResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		PushID  string `json:"pushid"`
		ReadKey string `json:"readkey"`
	} `json:"data"`
}

func sendPush(apiKey string, reqData PushRequest) (*PushResponse, error) {
	url := fmt.Sprintf("https://sctapi.ftqq.com/%s.send", apiKey)

	// 将请求数据序列化为 JSON
	jsonData, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request data: %w", err)
	}

	// 创建 HTTP 请求
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json;charset=utf-8")

	// 发送 HTTP 请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应数据
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// 解析响应 JSON 数据
	var pushResp PushResponse
	if err := json.Unmarshal(body, &pushResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &pushResp, nil
}

func PushMsgToWechat(title, desp, channel, secret_key string) {

	reqData := PushRequest{
		Title:   title,
		Desp:    desp,
		Channel: channel,
	}

	apiKey := secret_key // 替换为实际的 API Key

	resp, err := sendPush(apiKey, reqData)
	if err != nil {
		fmt.Printf("Error sending push: %v\n", err)
		return
	}

	fmt.Printf("Push Response: %+v\n", resp)
}

var secret_key string
