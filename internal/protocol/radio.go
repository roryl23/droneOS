package protocol

import (
	"context"
	"droneOS/internal/utils"
	"time"

	"github.com/rs/zerolog"
)

func ServeRadio(ctx context.Context, link RadioLink) {
	logger := zerolog.Ctx(ctx)
	consecutiveErrors := 0
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		data, err := link.Receive()
		if err != nil {
			consecutiveErrors++
			// Only log every 100th error to avoid log spam
			if consecutiveErrors%100 == 1 {
				logger.Warn().Err(err).Int("count", consecutiveErrors).Msg("radio receive errors")
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}
		if len(data) == 0 {
			// No data available - sleep longer to reduce USB polling pressure
			time.Sleep(50 * time.Millisecond)
			continue
		}

		// Reset error counter on successful receive
		consecutiveErrors = 0

		msg, err := DecodeMessageBytes(data)
		if err != nil {
			logger.Debug().Err(err).Msg("radio decode failed")
			continue
		}

		output, err := utils.CallFunctionByName(ctx, FuncMap, msg.Cmd, msg)
		if err != nil {
			logger.Error().Err(err).Msg("radio handler failed")
			continue
		}
		response, ok := output[0].Interface().(Message)
		if !ok {
			logger.Error().Msg("unexpected response type")
			continue
		}
		payload, err := EncodeMessage(response)
		if err != nil {
			logger.Error().Err(err).Msg("radio encode failed")
			continue
		}
		if err := link.Send(payload); err != nil {
			logger.Error().Err(err).Msg("radio send failed")
		}
	}
}
