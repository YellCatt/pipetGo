package testcase

import (
	"encoding/json"
	"fmt"

	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"

	"pipetGo/internal/httpclient"
	"pipetGo/internal/logger"
	"pipetGo/internal/psv"
)

func LoadFromPSV(filePath string) error {
	testCases, err := psv.ParseFile(filePath)
	if err != nil {
		return err
	}

	for _, tc := range testCases {
		if tc.Skip {
			RegisterSkippedTest(tc.Name, tc.SkipReason)
			continue
		}

		testCase := tc
		RegisterTest(testCase.Name, func() error {
			return runPSVTestCase(&testCase)
		})
	}

	return nil
}

func runPSVTestCase(tc *psv.TestCase) error {
	logger.Info("正在执行 PSV 测试", zap.String("name", tc.Name), zap.String("endpoint", tc.Endpoint))

	req := httpclient.Client.R()

	for key, value := range tc.Headers {
		req.SetHeader(key, value)
	}

	if tc.Body != "" {
		req.SetBody(tc.Body)
	}

	var resp *resty.Response
	var err error

	switch tc.Method {
	case "GET":
		resp, err = req.Get(tc.Endpoint)
	case "POST":
		resp, err = req.Post(tc.Endpoint)
	case "PUT":
		resp, err = req.Put(tc.Endpoint)
	case "DELETE":
		resp, err = req.Delete(tc.Endpoint)
	case "PATCH":
		resp, err = req.Patch(tc.Endpoint)
	case "HEAD":
		resp, err = req.Head(tc.Endpoint)
	default:
		return ErrInvalidMethod(tc.Method)
	}

	if err != nil {
		return err
	}

	logger.Debug("响应",
		zap.Int("status", resp.StatusCode()),
		zap.String("body", string(resp.Body())))

	if tc.ExpectedCode > 0 && resp.StatusCode() != tc.ExpectedCode {
		return ErrUnexpectedStatus(tc.ExpectedCode, resp.StatusCode())
	}

	return nil
}

func ErrInvalidMethod(method string) error {
	return &TestError{Message: "不支持的 HTTP 方法: " + method}
}

func ErrUnexpectedStatus(expected, actual int) error {
	return &TestError{Message: fmt.Sprintf("期望状态码 %d，实际 %d", expected, actual)}
}

type TestError struct {
	Message string
}

func (e *TestError) Error() string {
	return e.Message
}

func PrettyPrintBody(body interface{}) string {
	jsonBytes, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return ""
	}
	return string(jsonBytes)
}
