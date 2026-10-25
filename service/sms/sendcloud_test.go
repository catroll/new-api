package sms

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/setting/system_setting"
)

// The documented Python example (smsUser=testuser, templateId=1,
// phone=18888888888, vars={}) rendered with
// SMS_KEY=ABCDEFGHIJKLMNOPQRSTUVWXYZ. If this breaks, every request is signed
// wrongly and SendCloud answers 422.
func TestSignParamsMatchesDocumentedExample(t *testing.T) {
	params := map[string]string{
		"smsUser":    "testuser",
		"templateId": "1",
		"phone":      "18888888888",
		"vars":       "{}",
	}
	const want = "473959199cbb4bcac74310296ba880b1394ff73fd0953f3661c5d5eb999d43d3"
	if got := signParams(params, "ABCDEFGHIJKLMNOPQRSTUVWXYZ"); got != want {
		t.Fatalf("signParams() = %s, want %s", got, want)
	}
	if got := signParams(map[string]string{"smsUser": "testuser", "signature": "x"}, "k"); got == signParams(map[string]string{"smsUser": "x", "signature": "x"}, "k") {
		t.Fatal("signature must not depend on the signature field")
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name     string
		envelope sendCloudResponse
		want     error
	}{
		{"ok", sendCloudResponse{Result: true, StatusCode: 200}, nil},
		{"partial with success", partialSuccess(), nil},
		{"invalid phone", sendCloudResponse{StatusCode: 412}, ErrInvalidPhone},
		{"signature", sendCloudResponse{StatusCode: 422}, ErrNotConfigured},
		{"credentials", sendCloudResponse{StatusCode: 4722}, ErrNotConfigured},
		{"ip allowlist", sendCloudResponse{StatusCode: 601}, ErrNotConfigured},
		{"clock", sendCloudResponse{StatusCode: 461}, errClockSkew},
		{"throttled", sendCloudResponse{StatusCode: 496}, ErrThrottled},
		{"quota", sendCloudResponse{StatusCode: 499}, ErrQuota},
		{"unknown", sendCloudResponse{StatusCode: 502}, ErrProvider},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classify(tc.envelope); !errors.Is(got, tc.want) {
				t.Fatalf("classify() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSendCodeUsesSendCodeEndpointAndSignature(t *testing.T) {
	var gotPath string
	var gotForm map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		gotForm = map[string]string{}
		for key := range r.PostForm {
			gotForm[key] = r.PostForm.Get(key)
		}
		_, _ = w.Write([]byte(`{"result":true,"statusCode":200,"message":"请求成功","info":{"successCount":1,"smsIds":["abc$13800138000"]}}`))
	}))
	defer server.Close()

	provider := newSendCloud(Config{
		API:     system_setting.SMSAPISendCode,
		APIBase: server.URL + "/smsapi",
		SMSUser: "u",
		SMSKey:  "k",
		SignID:  10876,
		MsgType: system_setting.SMSMsgTypeDomestic,
	}, server.Client())

	result, err := provider.SendCode(context.Background(), Message{Phone: "+8613800138000", Code: "123456"})
	if err != nil {
		t.Fatalf("SendCode() error = %v", err)
	}
	if gotPath != "/smsapi/sendCode" {
		t.Fatalf("path = %s, want /smsapi/sendCode", gotPath)
	}
	if gotForm["phone"] != "13800138000" {
		t.Fatalf("phone = %q, want domestic digits", gotForm["phone"])
	}
	if gotForm["signId"] != "10876" || gotForm["code"] != "123456" {
		t.Fatalf("unexpected form: %v", gotForm)
	}
	if gotForm["signature"] == "" || gotForm["timestamp"] == "" {
		t.Fatalf("missing signature/timestamp: %v", gotForm)
	}
	signature := gotForm["signature"]
	delete(gotForm, "signature")
	if want := signParams(gotForm, "k"); signature != want {
		t.Fatalf("signature = %s, want %s", signature, want)
	}
	if len(result.SMSIDs) != 1 || result.SMSIDs[0] != "abc$13800138000" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestSendCodeRetriesOnceAfterClockSkew(t *testing.T) {
	sends := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/smsapi/timestamp/get" {
			_, _ = w.Write([]byte(`{"result":true,"info":{"timestamp":` + strconv.FormatInt(time.Now().UnixMilli(), 10) + `}}`))
			return
		}
		sends++
		if sends == 1 {
			_, _ = w.Write([]byte(`{"result":false,"statusCode":461,"message":"时间戳无效"}`))
			return
		}
		_, _ = w.Write([]byte(`{"result":true,"statusCode":200,"info":{"successCount":1,"smsIds":["ok"]}}`))
	}))
	defer server.Close()

	provider := newSendCloud(Config{
		API:      system_setting.SMSAPISendCode,
		APIBase:  server.URL + "/smsapi",
		SMSUser:  "u",
		SMSKey:   "k",
		SignName: "签名",
	}, server.Client())

	result, err := provider.SendCode(context.Background(), Message{Phone: "+8613800138000", Code: "123456"})
	if err != nil {
		t.Fatalf("SendCode() error = %v", err)
	}
	if sends != 2 {
		t.Fatalf("sends = %d, want 2 (one retry)", sends)
	}
	if len(result.SMSIDs) != 1 || result.SMSIDs[0] != "ok" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func partialSuccess() sendCloudResponse {
	response := sendCloudResponse{Result: true, StatusCode: 311}
	response.Info.SuccessCount = 1
	return response
}
