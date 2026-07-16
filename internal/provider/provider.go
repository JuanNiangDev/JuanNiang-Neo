package provider

import (
	"context"
	"errors"
	"sync"
	"time"
)

const TIMEOUT = 5 * time.Second

type Provider interface {
	ID() string
	Name() string
	Platform() Platform
	StartAdapter(ctx context.Context) error
	StopAdapter(ctx context.Context) error
	SendMessage(ctx context.Context, msg *Message)
	Register(messageChan chan Message)
}

type ProviderHub struct {
	mu             sync.RWMutex
	MessageChan    chan Message
	ActiveProvider map[string]Provider
}

func NewProviderHub() *ProviderHub {
	return &ProviderHub{
		mu:             sync.RWMutex{},
		MessageChan:    make(chan Message),
		ActiveProvider: make(map[string]Provider),
	}
}

func (r *ProviderHub) StartProvider(ctx context.Context, provider Provider) error {
	ctx, cancel := context.WithTimeout(ctx, TIMEOUT)

	r.mu.Lock()
	defer func() {
		cancel()
		r.mu.Unlock()
	}()

	if _, ok := r.ActiveProvider[provider.ID()]; ok {
		return errors.New("provider already started")
	}

	if err := provider.StartAdapter(ctx); err != nil {
		return err
	}

	provider.Register(r.MessageChan)

	r.ActiveProvider[provider.ID()] = provider

	return nil
}

func (r *ProviderHub) StopProvider(ctx context.Context, providerID string) error {
	ctx, cancel := context.WithTimeout(ctx, TIMEOUT)

	r.mu.Lock()
	defer func() {
		cancel()
		r.mu.Unlock()
	}()

	provider, ok := r.ActiveProvider[providerID]

	if !ok {
		return errors.New("provider not started")
	}

	if err := provider.StopAdapter(ctx); err != nil {
		return err
	}

	delete(r.ActiveProvider, providerID)

	provider = nil

	return nil
}

func (r *ProviderHub) GetProviderStatus(providerID string) bool {
	_, ok := r.ActiveProvider[providerID]

	return ok
}
