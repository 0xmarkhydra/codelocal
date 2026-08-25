package social

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrUnsupportedPlatform = errors.New("unsupported social platform or URL")

type Service struct {
	providers []Provider
}

func New() *Service {
	return NewWithProviders(NewXPublicProvider(nil, ""))
}

func NewWithProviders(providers ...Provider) *Service {
	filtered := make([]Provider, 0, len(providers))
	for _, provider := range providers {
		if provider != nil {
			filtered = append(filtered, provider)
		}
	}
	return &Service{providers: filtered}
}

func (s *Service) Execute(ctx context.Context, request Request) (Result, error) {
	action := strings.ToLower(strings.TrimSpace(request.Action))
	if action != "read" {
		return Result{}, fmt.Errorf("unsupported social action %q", request.Action)
	}
	targetURL := strings.TrimSpace(request.URL)
	if targetURL == "" {
		return Result{}, errors.New("social read requires url")
	}
	for _, provider := range s.providers {
		if !provider.Supports(targetURL) {
			continue
		}
		post, err := provider.ReadPublicPost(ctx, targetURL)
		if err != nil {
			return Result{}, fmt.Errorf("%s: %w", provider.Name(), err)
		}
		return Result{Action: action, Platform: post.Platform, Provider: provider.Name(), Post: post}, nil
	}
	return Result{}, ErrUnsupportedPlatform
}
