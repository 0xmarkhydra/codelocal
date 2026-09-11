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

type ImageSpec struct {
	URI string `json:"uri"`
}

type PlatformSpec struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type NetworkRule struct {
	Action string `json:"action"`
	Target string `json:"target"`
}

type NetworkPolicy struct {
	DefaultAction string        `json:"defaultAction,omitempty"`
	Egress        []NetworkRule `json:"egress,omitempty"`
}

type SandboxStatus struct {
	State   string `json:"state"`
	Reason  string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
}

type SandboxInfo struct {
	ID         string            `json:"id"`
	Image      *ImageSpec        `json:"image,omitempty"`
	SnapshotID string            `json:"snapshotId,omitempty"`
	Platform   *PlatformSpec     `json:"platform,omitempty"`
	Entrypoint []string          `json:"entrypoint"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Status     SandboxStatus     `json:"status"`
	CreatedAt  time.Time         `json:"createdAt"`
	ExpiresAt  *time.Time        `json:"expiresAt"`
}

type CreateSandboxRequest struct {
	Image            *ImageSpec        `json:"image,omitempty"`
	SnapshotID       string            `json:"snapshotId,omitempty"`
	Entrypoint       []string          `json:"entrypoint,omitempty"`
	Platform         *PlatformSpec     `json:"platform,omitempty"`
	SecureAccess     *bool             `json:"secureAccess,omitempty"`
	Timeout          *int              `json:"timeout,omitempty"`
	ResourceLimits   map[string]string `json:"resourceLimits"`
	ResourceRequests map[string]string `json:"resourceRequests,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
	NetworkPolicy    *NetworkPolicy    `json:"networkPolicy,omitempty"`
	Extensions       map[string]any    `json:"extensions,omitempty"`
}

type ListOptions struct {
	States   []string
	Metadata map[string]string
	Page     int
	PageSize int
}

type Pagination struct {
	Page         int  `json:"page"`
	PageSize     int  `json:"pageSize"`
	TotalItems   int  `json:"totalItems"`
	TotalPages   int  `json:"totalPages"`
	HasNextPage bool `json:"hasNextPage"`
}

type ListSandboxesResponse struct {
	Items      []SandboxInfo `json:"items"`
	Pagination *Pagination   `json:"pagination,omitempty"`
}

type RenewExpirationRequest struct {
	ExpiresAt time.Time `json:"expiresAt"`
}

type RenewExpirationResponse struct {
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

func (c *Client) CreateSandbox(ctx context.Context, request CreateSandboxRequest) (*SandboxInfo, error) {
	if len(request.ResourceLimits) == 0 {
		return nil, fmt.Errorf("opensandbox resource limits required")
	}
	var output SandboxInfo
	if err := c.doJSON(ctx, http.MethodPost, "/sandboxes", request, &output); err != nil {
		return nil, err
	}
	return &output, nil
}

func (c *Client) GetSandbox(ctx context.Context, sandboxID string) (*SandboxInfo, error) {
	if strings.TrimSpace(sandboxID) == "" {
		return nil, fmt.Errorf("opensandbox sandbox ID required")
	}
	var output SandboxInfo
	path := "/sandboxes/" + url.PathEscape(sandboxID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &output); err != nil {
		return nil, err
	}
	return &output, nil
}

func (c *Client) ListSandboxes(ctx context.Context, options ListOptions) (*ListSandboxesResponse, error) {
	query := url.Values{}
	for _, state := range options.States {
		if state = strings.TrimSpace(state); state != "" {
			query.Add("state", state)
		}
	}
	if len(options.Metadata) > 0 {
		metadata := url.Values{}
		for key, value := range options.Metadata {
			metadata.Set(key, value)
		}
		query.Set("metadata", metadata.Encode())
	}
	if options.Page > 0 {
		query.Set("page", strconv.Itoa(options.Page))
	}
	if options.PageSize > 0 {
		query.Set("pageSize", strconv.Itoa(options.PageSize))
	}
	path := "/sandboxes"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	var output ListSandboxesResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &output); err != nil {
		return nil, err
	}
	return &output, nil
}

func (c *Client) PauseSandbox(ctx context.Context, sandboxID string) error {
	return c.lifecycleAction(ctx, sandboxID, "pause")
}

func (c *Client) ResumeSandbox(ctx context.Context, sandboxID string) error {
	return c.lifecycleAction(ctx, sandboxID, "resume")
}

func (c *Client) lifecycleAction(ctx context.Context, sandboxID, action string) error {
	if strings.TrimSpace(sandboxID) == "" {
		return fmt.Errorf("opensandbox sandbox ID required")
	}
	path := "/sandboxes/" + url.PathEscape(sandboxID) + "/" + action
	return c.doJSON(ctx, http.MethodPost, path, nil, nil)
}

func (c *Client) DeleteSandbox(ctx context.Context, sandboxID string) error {
	if strings.TrimSpace(sandboxID) == "" {
		return fmt.Errorf("opensandbox sandbox ID required")
	}
	return c.doJSON(ctx, http.MethodDelete, "/sandboxes/"+url.PathEscape(sandboxID), nil, nil)
}

func (c *Client) RenewExpiration(ctx context.Context, sandboxID string, expiresAt time.Time) (*RenewExpirationResponse, error) {
	if strings.TrimSpace(sandboxID) == "" {
		return nil, fmt.Errorf("opensandbox sandbox ID required")
	}
	request := RenewExpirationRequest{ExpiresAt: expiresAt.UTC()}
	var output RenewExpirationResponse
	path := "/sandboxes/" + url.PathEscape(sandboxID) + "/renew-expiration"
	if err := c.doJSON(ctx, http.MethodPost, path, request, &output); err != nil {
		return nil, err
	}
	return &output, nil
}
