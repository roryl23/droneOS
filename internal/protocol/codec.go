package protocol

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
)

const maxPayloadSize = 64 * 1024

func EncodeMessage(msg Message) ([]byte, error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("encode message: %w", err)
	}
	if len(payload) > maxPayloadSize {
		return nil, fmt.Errorf("message too large: %d", len(payload))
	}

	buf := make([]byte, 4+len(payload))
	binary.BigEndian.PutUint32(buf[:4], uint32(len(payload)))
	copy(buf[4:], payload)
	return buf, nil
}

func DecodeMessage(r io.Reader) (Message, error) {
	lengthBytes := make([]byte, 4)
	if _, err := io.ReadFull(r, lengthBytes); err != nil {
		return Message{}, fmt.Errorf("read length: %w", err)
	}
	length := binary.BigEndian.Uint32(lengthBytes)
	if length == 0 || length > maxPayloadSize {
		return Message{}, fmt.Errorf("invalid length: %d", length)
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return Message{}, fmt.Errorf("read payload: %w", err)
	}

	var msg Message
	if err := json.Unmarshal(payload, &msg); err != nil {
		return Message{}, fmt.Errorf("decode message: %w", err)
	}
	return msg, nil
}

func DecodeMessageBytes(data []byte) (Message, error) {
	return DecodeMessage(bytes.NewReader(data))
}
