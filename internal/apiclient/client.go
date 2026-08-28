package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/rs/zerolog/log"
)

func New(addr string) *Client { return &Client{addr: addr} }

type Client struct{ addr string }

func (c *Client) BaseURL() *url.URL { return &url.URL{Scheme: "http", Host: c.addr, Path: "/api/v1"} }

func (c *Client) GetDepartments(ctx context.Context) ([]model.Department, error) {
	u := c.BaseURL().JoinPath("departments")

	rawJSON, err := c.request(ctx, u)
	if err != nil {
		return nil, err
	}

	var data []model.Department
	if err := json.Unmarshal(rawJSON, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return data, nil
}

func (c *Client) GetGroup(ctx context.Context, name string) (*model.Group, error) {
	u := c.BaseURL().JoinPath("groups/" + name)

	rawJSON, err := c.request(ctx, u)
	if err != nil {
		return nil, err
	}

	var data model.Group
	if err := json.Unmarshal(rawJSON, &data); err != nil {
		return nil, err
	}

	return &data, nil

}
func (c *Client) GetGroups(ctx context.Context, departmentName string) ([]model.Group, error) {
	u := c.BaseURL().JoinPath("groups")
	q := u.Query()
	q.Set("department", departmentName)
	u.RawQuery = q.Encode()

	rawJSON, err := c.request(ctx, u)
	if err != nil {
		return nil, err
	}

	var data []model.Group
	if err := json.Unmarshal(rawJSON, &data); err != nil {
		log.Error().Err(err).Str("response", string(rawJSON)).Send()
		return nil, fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	return data, nil
}

func (c *Client) GetTeacher(ctx context.Context, nameOrID string) (*model.Teacher, error) {
	u := c.BaseURL().JoinPath("teachers/" + nameOrID)

	rawJSON, err := c.request(ctx, u)
	if err != nil {
		return nil, err
	}

	var data model.Teacher
	if err := json.Unmarshal(rawJSON, &data); err != nil {
		return nil, err
	}

	return &data, nil
}
func (c *Client) SearchTeachers(ctx context.Context, query string) ([]model.Teacher, error) {
	u := c.BaseURL().JoinPath("teachers", "search")
	q := u.Query()
	q.Set("q", query)
	u.RawQuery = q.Encode()

	rawJSON, err := c.request(ctx, u)
	if err != nil {
		return nil, err
	}

	var data []model.Teacher
	if err := json.Unmarshal(rawJSON, &data); err != nil {
		return nil, err
	}

	return data, nil
}
func (c *Client) GetTeachers(ctx context.Context) ([]model.Teacher, error) {
	u := c.BaseURL().JoinPath("teachers")

	rawJSON, err := c.request(ctx, u)
	if err != nil {
		return nil, err
	}

	var data []model.Teacher
	if err := json.Unmarshal(rawJSON, &data); err != nil {
		return nil, err
	}

	return data, nil
}

type GetScheduleParams struct {
	// Group name
	Group string
	// Teacher name or college internal id
	Teacher string
}

func (c *Client) GetSchedule(
	ctx context.Context, params *GetScheduleParams,
) (schedule *model.ScheduleData, err error) {
	u := c.BaseURL().JoinPath("schedule")
	q := u.Query()
	if params.Group != "" {
		q.Set("group", params.Group)
	} else {
		q.Set("teacher", params.Teacher)
	}
	u.RawQuery = q.Encode()

	rawJSON, err := c.request(ctx, u)
	if err != nil {
		return nil, err
	}

	var respData model.ScheduleData
	if err := json.Unmarshal(rawJSON, &respData); err != nil {
		return nil, err
	}

	return &respData, nil
}

var (
	ErrNotFound            = errors.New("not found")
	ErrInternalServerError = errors.New("internal server error")
	ErrBadRequest          = errors.New("bad request")
	ErrServiceUnavailable  = errors.New("service unavailable")
)

func (c *Client) request(ctx context.Context, url *url.URL) ([]byte, error) {
	log.Trace().Str("url", url.String()).Msg("Requesting API...")

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := new(http.Client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Warn().Str("status", resp.Status).Msg("Non-200 response status")

		rawJSON, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read body: %w", err)
		}

		var data model.ErrorResponse
		if err := json.Unmarshal(rawJSON, &data); err != nil {
			return nil, err
		}

		switch resp.StatusCode {
		case http.StatusBadRequest:
			return nil, fmt.Errorf("%w: %s", ErrBadRequest, data.Error)
		case http.StatusNotFound:
			return nil, fmt.Errorf("%w: %s", ErrNotFound, data.Error)
		case http.StatusInternalServerError:
			return nil, fmt.Errorf("%w: %s", ErrInternalServerError, data.Error)
		case http.StatusServiceUnavailable:
			return nil, fmt.Errorf("%w: %s", ErrServiceUnavailable, data.Error)
		default:
			return nil, fmt.Errorf("unknown API error: %s", data.Error)
		}
	}

	rawJSON, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}
	log.Trace().Msg("Got response successfully")

	return rawJSON, nil
}
