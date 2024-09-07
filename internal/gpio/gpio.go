package gpio

import (
	"github.com/warthog618/go-gpiocdev"
	"github.com/warthog618/go-gpiocdev/device/rpi"
)

func infoChangeHandler(evt gpiocdev.LineInfoChangeEvent) {
	// handle change in line info
}

func Init() {
	// currently available chips
	cc := gpiocdev.Chips()
	c, _ := gpiocdev.NewChip("gpiochip0")
	// using Raspberry Pi J8 mapping
	l, _ := c.RequestLine(rpi.J8p7)
	// watch line and handle
	inf, _ := c.WatchLineInfo(4, infoChangeHandler)
}
