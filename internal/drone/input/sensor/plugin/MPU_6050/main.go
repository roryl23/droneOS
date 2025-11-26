package MPU_6050

import (
	"context"
	"droneOS/internal/config"
	"droneOS/internal/drone/input/sensor"
	"time"

	"github.com/rs/zerolog/log"
	"gobot.io/x/gobot/drivers/i2c"
	"gobot.io/x/gobot/platforms/raspi"
)

func Main(
	ctx context.Context,
	s *config.Device,
	eventCh *chan sensor.Event,
) {
	rpi := raspi.NewAdaptor()
	log.Info().Interface("rpi", rpi)

	// default bus/address
	d := i2c.NewMPU6050Driver(rpi)

	// optional bus/address
	//d := i2c.NewMPU6050Driver(adaptor,
	//	i2c.WithBus(0),
	//	i2c.WithAddress(0x34))
	log.Info().Interface("driver", d)

	for {
		time.Sleep(500 * time.Millisecond)
	}
}
