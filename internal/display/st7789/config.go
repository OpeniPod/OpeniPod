package st7789

// Pin identifies a GPIO character-device chip and line offset.
type Pin struct {
	Chip   string
	Offset int
}

// LinuxConfig selects the SPI device, GPIO lines and panel settings.
type LinuxConfig struct {
	Bus        int
	ChipSelect int
	SpeedHz    int64
	Mode       int
	DC         Pin
	Reset      Pin
	ChunkSize  int
	Panel      Config
}
