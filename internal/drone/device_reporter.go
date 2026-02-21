package drone

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"

	"droneOS/internal/config"
	"droneOS/internal/protocol"

	"github.com/rs/zerolog/log"
)

const deviceReportInterval = 10 * time.Second

func StartDeviceReporter(
	ctx context.Context,
	settings *config.Config,
	wifiConnected *atomic.Bool,
) {
	if settings == nil || wifiConnected == nil {
		return
	}

	addr := fmt.Sprintf("%s:%d", settings.Base.Host, settings.Base.Port)
	transport := &protocol.WiFiTransport{
		Addr:    addr,
		Timeout: 3 * time.Second,
	}

	ticker := time.NewTicker(deviceReportInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			report, errs := CollectDeviceState(ctx, settings)
			if len(errs) > 0 {
				log.Debug().Strs("errors", report.Errors).
					Msg("device state collection errors")
			}

			if !wifiConnected.Load() {
				continue
			}

			payload, err := json.Marshal(report)
			if err != nil {
				log.Warn().Err(err).
					Msg("failed to encode device state report")
				continue
			}

			_, err = transport.Send(ctx, protocol.Message{
				ID:   settings.Drone.ID,
				Cmd:  "device_state",
				Data: string(payload),
			})
			if err != nil {
				log.Debug().Err(err).
					Msg("device state report failed")
			}
		}
	}()
}
