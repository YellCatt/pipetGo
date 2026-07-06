package tests

import (
	"errors"

	"github.com/go-resty/resty/v2"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"pipet/internal/httpclient"
	"pipet/internal/logger"
	"pipet/internal/testcase"
)

func init() {
	testcase.RegisterTest("TestGetUsers", testGetUsers)
	testcase.RegisterTest("TestCreateUser", testCreateUser)
	testcase.RegisterSkippedTest("TestDeleteUser", "Not implemented yet")
}

func testGetUsers() error {
	resp, err := httpclient.Client.R().
		SetHeader("Content-Type", "application/json").
		Get("/users")

	if err != nil {
		return err
	}

	logger.Debug("Response",
		zap.Int("status", resp.StatusCode()),
		zap.String("body", string(resp.Body())))

	assert.Equal(200, resp.StatusCode())
	return nil
}

func testCreateUser() error {
	resp, err := httpclient.Client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]interface{}{
			"name":  "Test User",
			"email": "test@example.com",
		}).
		Post("/users")

	if err != nil {
		return err
	}

	logger.Debug("Response",
		zap.Int("status", resp.StatusCode()),
		zap.String("body", string(resp.Body())))

	if resp.StatusCode() != 201 {
		return errors.New("expected status code 201")
	}
	return nil
}

func testDeleteUser() error {
	resp, err := httpclient.Client.R().
		SetHeader("Content-Type", "application/json").
		Delete("/users/1")

	if err != nil {
		return err
	}

	assert.Equal(204, resp.StatusCode())
	return nil
}

func NewTestClient() *resty.Client {
	return httpclient.Client
}
