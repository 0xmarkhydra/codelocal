package opensandbox

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type SnapshotStatus struct {
	State   string `json:"state"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type SnapshotInfo struct {
	ID        string         `json:"id"`
	SandboxID string         `json:"sandboxId"`
	Name      string         `json:"name,omitempty"`
	Status    SnapshotStatus `json:"status"`
	CreatedAt time.Time      `json:"createdAt"`
}

type CreateSnapshotRequest struct {
	Name string `json:"name,omitempty"`
}

type SnapshotListOptions struct {
	SandboxID string
	Name      string
	States    []string
	Page      int
	PageSize  int
}

type ListSnapshotsResponse struct {
	Items      []SnapshotInfo `json:"items"`
	Pagination *Pagination    `json:"pagination,omitempty"`
}

func (c *Client) CreateSnapshot(ctx context.Context, sandboxID, name string) (*SnapshotInfo, error) {
	if strings.TrimSpace(sandboxID) == "" {
		return nil, fmt.Errorf("opensandbox sandbox ID required")
	}
	var output SnapshotInfo
	path := "/sandboxes/" + url.PathEscape(sandboxID) + "/snapshots"
	if err := c.doJSON(ctx, http.MethodPost, path, CreateSnapshotRequest{Name: strings.TrimSpace(name)}, &output); err != nil {
		return nil, err
	}
	return &output, nil
}

func (c *Client) GetSnapshot(ctx context.Context, snapshotID string) (*SnapshotInfo, error) {
	if strings.TrimSpace(snapshotID) == "" {
		return nil, fmt.Errorf("opensandbox snapshot ID required")
	}
	var output SnapshotInfo
	if err := c.doJSON(ctx, http.MethodGet, "/snapshots/"+url.PathEscape(snapshotID), nil, &output); err != nil {
		return nil, err
	}
	return &output, nil
}

func (c *Client) ListSnapshots(ctx context.Context, options SnapshotListOptions) (*ListSnapshotsResponse, error) {
	query := url.Values{}
	if value := strings.TrimSpace(options.SandboxID); value != "" {
		query.Set("sandboxId", value)
	}
	if value := strings.TrimSpace(options.Name); value != "" {
		query.Set("name", value)
	}
	for _, state := range options.States {
		if state = strings.TrimSpace(state); state != "" {
			query.Add("state", state)
		}
	}
	if options.Page > 0 {
		query.Set("page", strconv.Itoa(options.Page))
	}
	if options.PageSize > 0 {
		query.Set("pageSize", strconv.Itoa(options.PageSize))
	}
	path := "/snapshots"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var output ListSnapshotsResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &output); err != nil {
		return nil, err
	}
	return &output, nil
}

func (c *Client) DeleteSnapshot(ctx context.Context, snapshotID string) error {
	if strings.TrimSpace(snapshotID) == "" {
		return fmt.Errorf("opensandbox snapshot ID required")
	}
	return c.doJSON(ctx, http.MethodDelete, "/snapshots/"+url.PathEscape(snapshotID), nil, nil)
}
