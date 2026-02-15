package protocol

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

type Transport interface {
	Send(ctx context.Context, msg Message) (Message, error)
}

type WiFiTransport struct {
	Addr    string
	Timeout time.Duration
}

func (t *WiFiTransport) Send(ctx context.Context, msg Message) (Message, error) {
	if t.Addr == "" {
		return Message{}, errors.New("wifi address not set")
	}
	deadline := time.Now().Add(t.Timeout)
	conn, err := net.DialTimeout("tcp", t.Addr, t.Timeout)
	if err != nil {
		return Message{}, fmt.Errorf("wifi dial: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(deadline)

	data, err := EncodeMessage(msg)
	if err != nil {
		return Message{}, err
	}
	if _, err := conn.Write(data); err != nil {
		return Message{}, fmt.Errorf("wifi write: %w", err)
	}

	resp, err := DecodeMessage(conn)
	if err != nil {
		return Message{}, fmt.Errorf("wifi read: %w", err)
	}
	return resp, nil
}

type RadioLink interface {
	Send([]byte) error
	Receive() ([]byte, error)
}

type RadioTransport struct {
	Link          RadioLink
	Timeout       time.Duration
	RetryInterval time.Duration
}

func (t *RadioTransport) Send(ctx context.Context, msg Message) (Message, error) {
	if t.Link == nil {
		return Message{}, errors.New("radio link not set")
	}
	payload, err := EncodeMessage(msg)
	if err != nil {
		return Message{}, err
	}
	if err := t.Link.Send(payload); err != nil {
		return Message{}, fmt.Errorf("radio send: %w", err)
	}

	deadline := time.Now().Add(t.Timeout)
	retry := t.RetryInterval
	if retry <= 0 {
		retry = 50 * time.Millisecond
	}

	for {
		select {
		case <-ctx.Done():
			return Message{}, ctx.Err()
		default:
		}

		data, err := t.Link.Receive()
		if err != nil {
			if time.Now().After(deadline) {
				return Message{}, fmt.Errorf("radio receive: %w", err)
			}
			time.Sleep(retry)
			continue
		}
		if len(data) == 0 {
			if time.Now().After(deadline) {
				return Message{}, errors.New("radio receive timeout")
			}
			time.Sleep(retry)
			continue
		}

		resp, err := DecodeMessageBytes(data)
		if err != nil {
			return Message{}, fmt.Errorf("radio decode: %w", err)
		}
		return resp, nil
	}
}

type AutoTransport struct {
	WiFi       Transport
	Radio      Transport
	PreferWiFi bool
}

func (t *AutoTransport) Send(ctx context.Context, msg Message) (Message, error) {
	if t.PreferWiFi && t.WiFi != nil {
		resp, err := t.WiFi.Send(ctx, msg)
		if err == nil {
			return resp, nil
		}
	}
	if t.Radio != nil {
		resp, err := t.Radio.Send(ctx, msg)
		if err == nil {
			return resp, nil
		}
	}
	if !t.PreferWiFi && t.WiFi != nil {
		return t.WiFi.Send(ctx, msg)
	}
	return Message{}, errors.New("no transport available")
}

func NewAutoTransport(addr string, link RadioLink, preferWiFi bool) *AutoTransport {
	wifi := &WiFiTransport{Addr: addr, Timeout: 500 * time.Millisecond}
	radio := &RadioTransport{Link: link, Timeout: 2 * time.Second}
	return &AutoTransport{
		WiFi:       wifi,
		Radio:      radio,
		PreferWiFi: preferWiFi,
	}
}
