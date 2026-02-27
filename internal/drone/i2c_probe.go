package drone

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

const i2cSlave = 0x0703

func readI2CRegister(bus int, addr int, reg byte) (byte, error) {
	path := fmt.Sprintf("/dev/i2c-%d", bus)
	handle, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return 0, fmt.Errorf("open i2c bus %s: %w", path, err)
	}
	defer handle.Close()

	if err := unix.IoctlSetInt(int(handle.Fd()), i2cSlave, addr); err != nil {
		return 0, fmt.Errorf("select i2c addr 0x%02X: %w", addr, err)
	}
	if _, err := handle.Write([]byte{reg}); err != nil {
		return 0, fmt.Errorf("write register 0x%02X: %w", reg, err)
	}
	buf := []byte{0}
	if _, err := handle.Read(buf); err != nil {
		return 0, fmt.Errorf("read register 0x%02X: %w", reg, err)
	}
	return buf[0], nil
}
