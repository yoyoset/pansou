package service

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"pansou/model"
)

type checkRoundTripperFunc func(*http.Request) (*http.Response, error)

func (roundTrip checkRoundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func TestCheck115ShareState(test *testing.T) {
	testCases := []struct {
		name        string
		response    string
		wantState   string
		wantSummary string
	}{
		{
			name:        "expired share with files and metadata",
			response:    `{"state":true,"errno":0,"data":{"share_state":7,"shareinfo":{"forbid_reason":"链接已过期","snap_id":"snapshot","share_title":"shared files"},"count":1,"list":[{"fid":"file"}]}}`,
			wantState:   checkStateBad,
			wantSummary: "链接已过期",
		},
		{
			name:        "expired share with list and no reason",
			response:    `{"state":true,"errno":0,"data":{"share_state":7,"list":[{"fid":"file"}]}}`,
			wantState:   checkStateBad,
			wantSummary: "链接状态异常(share_state=7)",
		},
		{
			name:        "expired share with count and no reason",
			response:    `{"state":true,"errno":0,"data":{"share_state":7,"count":1}}`,
			wantState:   checkStateBad,
			wantSummary: "链接状态异常(share_state=7)",
		},
		{
			name:        "expired share with snapshot and no reason",
			response:    `{"state":true,"errno":0,"data":{"share_state":7,"shareinfo":{"snap_id":"snapshot"}}}`,
			wantState:   checkStateBad,
			wantSummary: "链接状态异常(share_state=7)",
		},
		{
			name:        "expired share with title and no reason",
			response:    `{"state":true,"errno":0,"data":{"share_state":7,"shareinfo":{"share_title":"shared files"}}}`,
			wantState:   checkStateBad,
			wantSummary: "链接状态异常(share_state=7)",
		},
		{
			name:        "nested expired share state with files",
			response:    `{"state":true,"errno":0,"data":{"share_state":0,"shareinfo":{"share_state":7,"forbid_reason":"链接已过期"},"count":1}}`,
			wantState:   checkStateBad,
			wantSummary: "链接已过期",
		},
		{
			name:        "top level expired state overrides nested active state",
			response:    `{"state":true,"errno":0,"data":{"share_state":7,"shareinfo":{"share_state":1},"count":1}}`,
			wantState:   checkStateBad,
			wantSummary: "链接状态异常(share_state=7)",
		},
		{
			name:        "forbidden share with active state and files",
			response:    `{"state":true,"errno":0,"data":{"share_state":1,"shareinfo":{"forbid_reason":"  分享已取消  "},"count":1}}`,
			wantState:   checkStateBad,
			wantSummary: "分享已取消",
		},
		{
			name:        "forbidden share without state and with files",
			response:    `{"state":true,"errno":0,"data":{"shareinfo":{"forbid_reason":"链接已过期"},"count":1}}`,
			wantState:   checkStateBad,
			wantSummary: "链接已过期",
		},
		{
			name:        "password error with files",
			response:    `{"state":true,"errno":0,"data":{"share_state":7,"shareinfo":{"forbid_reason":"提取码错误"},"count":1}}`,
			wantState:   checkStateLocked,
			wantSummary: "提取码错误",
		},
		{
			name:        "active share without files",
			response:    `{"state":true,"errno":0,"data":{"share_state":1}}`,
			wantState:   checkStateOK,
			wantSummary: "链接有效",
		},
		{
			name:        "nested active share without files",
			response:    `{"state":true,"errno":0,"data":{"shareinfo":{"share_state":1}}}`,
			wantState:   checkStateOK,
			wantSummary: "链接有效",
		},
		{
			name:        "list without share state",
			response:    `{"state":true,"errno":0,"data":{"list":[{"fid":"file"}]}}`,
			wantState:   checkStateOK,
			wantSummary: "链接有效",
		},
		{
			name:        "count with zero share state",
			response:    `{"state":true,"errno":0,"data":{"share_state":0,"count":1}}`,
			wantState:   checkStateOK,
			wantSummary: "链接有效",
		},
		{
			name:        "snapshot without share state",
			response:    `{"state":true,"errno":0,"data":{"shareinfo":{"snap_id":"snapshot"}}}`,
			wantState:   checkStateOK,
			wantSummary: "链接有效",
		},
		{
			name:        "title without share state",
			response:    `{"state":true,"errno":0,"data":{"shareinfo":{"share_title":"shared files"}}}`,
			wantState:   checkStateOK,
			wantSummary: "链接有效",
		},
		{
			name:        "whitespace reason with files",
			response:    `{"state":true,"errno":0,"data":{"shareinfo":{"forbid_reason":"  "},"count":1}}`,
			wantState:   checkStateOK,
			wantSummary: "链接有效",
		},
		{
			name:        "empty share response",
			response:    `{"state":true,"errno":0,"data":{}}`,
			wantState:   checkStateBad,
			wantSummary: "链接状态异常(share_state=0)",
		},
		{
			name:        "API password error",
			response:    `{"state":false,"errno":1,"error":"提取码错误"}`,
			wantState:   checkStateLocked,
			wantSummary: "提取码错误",
		},
		{
			name:        "API expired error",
			response:    `{"state":false,"errno":1,"error":"链接已过期"}`,
			wantState:   checkStateBad,
			wantSummary: "链接已过期",
		},
	}

	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			client := &http.Client{Transport: checkRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
				if request.Method != http.MethodGet || request.URL.Host != "115cdn.com" || request.URL.Path != "/webapi/share/snap" {
					test.Fatalf("unexpected request: %s %s", request.Method, request.URL)
				}
				if request.URL.Query().Get("share_code") != "swff16b3ngy" || request.URL.Query().Get("receive_code") != "CNYY" {
					test.Fatalf("unexpected share parameters: %s", request.URL.RawQuery)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(testCase.response)),
				}, nil
			})}
			service := &CheckService{}
			item := model.CheckItem{
				DiskType: "115",
				URL:      "https://115.com/s/swff16b3ngy?password=CNYY",
			}

			result, err := service.check115(item, item.URL, client)
			if err != nil {
				test.Fatalf("check115 returned an error: %v", err)
			}
			if result.State != testCase.wantState || result.Summary != testCase.wantSummary {
				test.Fatalf("expected state %q and summary %q, got %#v", testCase.wantState, testCase.wantSummary, result)
			}
		})
	}
}

func TestCheckBaiduSharePageRecheck(test *testing.T) {
	// 夹具来自 2026-10-23 对真实百度接口的观测：无提取码的链接 share/list 一律回 9019，
	// 失效分享页 200 + 标题「链接不存在」或 404，公开分享页 200 + "errortype":-1，需提取码的分享 302 到 share/init。
	const deadPage = `<html><head><title>百度网盘-链接不存在</title></head><div class="share-error">"errortype":1</div>`
	const publicPage = `<html><head><title>我的三体_免费高速下载|百度网盘-分享无限制</title></head>"errortype":-1`
	const notFoundPage = `<html><head><title>页面不存在</title></head>啊哦，你所访问的页面不存在了`

	testCases := []struct {
		name        string
		url         string
		verify      string
		list        string
		pageStatus  int
		pageBody    string
		wantState   string
		wantSummary string
	}{
		{"no password, cancelled share", "https://pan.baidu.com/s/1abc", "", `{"errno":9019,"errmsg":"need verify"}`, http.StatusOK, deadPage, checkStateBad, "链接失效"},
		{"no password, page 404", "https://pan.baidu.com/s/1abc", "", `{"errno":9019,"errmsg":"need verify"}`, http.StatusNotFound, notFoundPage, checkStateBad, "链接失效"},
		{"no password, public share", "https://pan.baidu.com/s/1abc", "", `{"errno":9019,"errmsg":"need verify"}`, http.StatusOK, publicPage, checkStateOK, "链接有效"},
		{"no password, share needs password", "https://pan.baidu.com/s/1abc", "", `{"errno":9019,"errmsg":"need verify"}`, http.StatusFound, "", checkStateLocked, "需要提取码"},
		{"no password, unknown page keeps uncertain", "https://pan.baidu.com/s/1abc", "", `{"errno":9019,"errmsg":"need verify"}`, http.StatusOK, `<title>百度网盘</title>`, checkStateUncertain, "need verify"},
		{"password rejected, share cancelled", "https://pan.baidu.com/s/1abc?pwd=7z55", `{"errno":-9}`, "", http.StatusOK, deadPage, checkStateBad, "链接失效"},
		{"password rejected, share exists", "https://pan.baidu.com/s/1abc?pwd=h75X", `{"errno":-9}`, "", http.StatusFound, "", checkStateLocked, "提取码错误"},
		{"password accepted, list ok skips page", "https://pan.baidu.com/s/1abc?pwd=8888", `{"errno":0,"randsk":"x"}`, `{"errno":0,"list":[{"fs_id":1}]}`, 0, "", checkStateOK, "链接有效"},
	}

	for _, testCase := range testCases {
		test.Run(testCase.name, func(test *testing.T) {
			pageRequested := false
			client := &http.Client{Transport: checkRoundTripperFunc(func(request *http.Request) (*http.Response, error) {
				respond := func(status int, body string, header http.Header) (*http.Response, error) {
					if header == nil {
						header = http.Header{}
					}
					return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
				}
				switch request.URL.Path {
				case "/share/verify":
					return respond(http.StatusOK, testCase.verify, nil)
				case "/share/list":
					return respond(http.StatusOK, testCase.list, nil)
				case "/s/1abc":
					pageRequested = true
					if testCase.pageStatus == http.StatusFound {
						return respond(http.StatusFound, "", http.Header{"Location": []string{"https://pan.baidu.com/share/init?surl=abc"}})
					}
					return respond(testCase.pageStatus, testCase.pageBody, nil)
				}
				test.Fatalf("unexpected request: %s %s", request.Method, request.URL)
				return nil, nil
			})}
			service := &CheckService{}
			item := model.CheckItem{DiskType: "baidu", URL: testCase.url}

			result, err := service.checkBaidu(item, item.URL, client)
			if err != nil {
				test.Fatalf("checkBaidu returned an error: %v", err)
			}
			if result.State != testCase.wantState || result.Summary != testCase.wantSummary {
				test.Fatalf("expected state %q and summary %q, got %#v", testCase.wantState, testCase.wantSummary, result)
			}
			if testCase.pageStatus == 0 && pageRequested {
				test.Fatalf("share page should not be requested when share/list is conclusive")
			}
		})
	}
}
