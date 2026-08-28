package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type RobotClient struct {
	harborURL string
	username  string
	password  string
	client    *http.Client
}

func NewRobotClient(config StorageConfig) *RobotClient {
	return &RobotClient{
		harborURL: config.RegistryURL,
		username:  config.Username,
		password:  config.Password,
		client:    &http.Client{},
	}
}

type RobotPermission struct {
	Kind      string        `json:"kind"`
	Namespace string        `json:"namespace"`
	Access    []RobotAccess `json:"access"`
}

type RobotAccess struct {
	Resource string `json:"resource"`
	Action   string `json:"action"`
}

type CreateRobotRequest struct {
	Name        string            `json:"name"`
	Duration    int               `json:"duration"`
	Level       string            `json:"level"`
	Permissions []RobotPermission `json:"permissions"`
}

type RobotResponse struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Token string `json:"token"`
}

func (c *RobotClient) do(req *http.Request) (*http.Response, error) {
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	return c.client.Do(req)
}

func (c *RobotClient) CreateRobot(ctx context.Context, name, project string, durationSeconds int) (*RobotResponse, error) {
	body := CreateRobotRequest{
		Name:     name,
		Duration: durationSeconds,
		Level:    "project",
		Permissions: []RobotPermission{
			{
				Kind:      "project",
				Namespace: project,
				Access: []RobotAccess{
					{Resource: "repository", Action: "push"},
				},
			},
		},
	}
	payload, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/api/v2.0/robots", c.harborURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.do(req)
	if err != nil {
		return nil, fmt.Errorf("harbor robot create: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return nil, fmt.Errorf("harbor robot quota exceeded or name conflict")
	}
	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("harbor robot permission denied")
	}
	if resp.StatusCode != http.StatusCreated {
		var errBody struct {
			Errors []struct {
				Message string `json:"message"`
			} `json:"errors"`
		}
		json.NewDecoder(resp.Body).Decode(&errBody)
		msg := "unknown"
		if len(errBody.Errors) > 0 {
			msg = errBody.Errors[0].Message
		}
		return nil, fmt.Errorf("harbor robot create failed: status=%d msg=%s", resp.StatusCode, msg)
	}

	var robot RobotResponse
	if err := json.NewDecoder(resp.Body).Decode(&robot); err != nil {
		return nil, fmt.Errorf("harbor robot decode: %w", err)
	}
	return &robot, nil
}

func (c *RobotClient) DeleteRobot(ctx context.Context, robotID int) error {
	url := fmt.Sprintf("%s/api/v2.0/robots/%d", c.harborURL, robotID)
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return fmt.Errorf("harbor robot delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("harbor robot delete failed: status=%d", resp.StatusCode)
	}
	return nil
}
