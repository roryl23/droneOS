package radio

import (
	log "github.com/sirupsen/logrus"
	"tinygo.org/x/drivers/lora"
)

func Init() {
	sxdriver := lora.Config{
		Freq:           0,
		Cr:             0,
		Sf:             0,
		Bw:             0,
		Ldr:            0,
		Preamble:       0,
		SyncWord:       0,
		HeaderType:     0,
		Crc:            0,
		Iq:             0,
		LoraTxPowerDBm: 0,
	}
	log.Info("sx1262 init", sxdriver)
}
