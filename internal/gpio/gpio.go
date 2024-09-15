package gpio

import (
	"github.com/warthog618/go-gpiocdev"
)

func Init() []string {
	// currently available chips
	chips := gpiocdev.Chips()

	return chips
}
