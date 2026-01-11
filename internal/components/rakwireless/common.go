package rakwireless

// SensorFrame represents the CBOR sensor frame structure from firmware
type SensorFrame struct {
	ID     int    `cbor:"id"`
	Type   int    `cbor:"type"`
	Fmt    string `cbor:"fmt"`
	Sensor string `cbor:"sensor"`
}
