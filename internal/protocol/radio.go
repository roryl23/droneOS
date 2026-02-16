package protocol

import (
	"context"
	"droneOS/internal/utils"

	"github.com/rs/zerolog"
)

func ServeRadio(ctx context.Context, link RadioLink) {
	logger := zerolog.Ctx(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		data, err := link.Receive()
		if err != nil {
			logger.Error().Err(err).Msg("radio receive failed")
			continue
		}
		if len(data) == 0 {
			continue
		}

		msg, err := DecodeMessageBytes(data)
		if err != nil {
			logger.Error().Err(err).Msg("radio decode failed")
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
