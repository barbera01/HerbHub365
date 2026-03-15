package sensors

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

const i2cSlave = 0x0703

type I2CDevice struct {
	file *os.File
}

func OpenI2CDevice(devicePath string, address uint16) (*I2CDevice, error) {
	file, err := os.OpenFile(devicePath, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	if err := unix.IoctlSetInt(int(file.Fd()), i2cSlave, int(address)); err != nil {
		file.Close()
		return nil, fmt.Errorf("set i2c address 0x%X: %w", address, err)
	}
	return &I2CDevice{file: file}, nil
}

func (d *I2CDevice) Close() error {
	if d == nil || d.file == nil {
		return nil
	}
	return d.file.Close()
}

func (d *I2CDevice) Write(data []byte) error {
	_, err := d.file.Write(data)
	return err
}

func (d *I2CDevice) Read(length int) ([]byte, error) {
	buffer := make([]byte, length)
	_, err := d.file.Read(buffer)
	if err != nil {
		return nil, err
	}
	return buffer, nil
}

func (d *I2CDevice) WriteRegister(register byte, data []byte) error {
	payload := append([]byte{register}, data...)
	_, err := d.file.Write(payload)
	return err
}

func (d *I2CDevice) ReadRegister(register byte, length int) ([]byte, error) {
	if _, err := d.file.Write([]byte{register}); err != nil {
		return nil, err
	}
	return d.Read(length)
}
