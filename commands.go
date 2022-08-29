// This package provides an implementation to read gas concentration measurements from a Sensiron SGP30 sensor.
package sensironsgp30

import (
	"context"
	"time"

	coreio "github.com/go-sensors/core/io"
	"github.com/go-sensors/core/units"
	"github.com/pkg/errors"
	"github.com/sigurn/crc8"
)

var (
	checksumTable = crc8.MakeTable(crc8.Params{
		Poly:   0x31,
		Init:   0xFF,
		RefIn:  false,
		RefOut: false,
		XorOut: 0x00,
		Check:  0x00,
		Name:   "CRC-8/Sensiron",
	})
)

func initAirQuality(ctx context.Context, port coreio.Port) error {
	_, err := port.Write([]byte{0x20, 0x03})
	if err != nil {
		return err
	}

	select {
	case <-ctx.Done():
	case <-time.After(setValueTimeout):
	}

	return nil
}

func setHumidity(ctx context.Context, port coreio.Port, absoluteHumidity units.MassConcentration) error {
	fixedPointValue := uint16(absoluteHumidity.GramsPerCubicMeter() * 256)
	humidityData := []byte{byte(fixedPointValue >> 8), byte(fixedPointValue)}
	humidityCRC := crc8.Checksum(humidityData, checksumTable)

	_, err := port.Write([]byte{0x20, 0x61, humidityData[0], humidityData[1], humidityCRC})
	if err != nil {
		return err
	}

	select {
	case <-ctx.Done():
	case <-time.After(setValueTimeout):
	}
	return nil
}

type airQuality struct {
	CO2eq units.Concentration
	TVOC  units.Concentration
}

func measureAirQuality(ctx context.Context, port coreio.Port) (*airQuality, error) {
	_, err := port.Write([]byte{0x20, 0x08})
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, nil
	case <-time.After(readValueTimeout):
	}

	data, err := readWords(port, 2)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read air quality")
	}

	reading := &airQuality{
		CO2eq: units.Concentration(data[0]) * units.PartPerMillion,
		TVOC:  units.Concentration(data[1]) * units.PartPerBillion,
	}
	return reading, nil
}

func readWords(port coreio.Port, words int) ([]uint16, error) {
	const (
		wordLength = 2
		crcLength  = 1
	)

	buf := make([]byte, words*(wordLength+crcLength))
	_, err := port.Read(buf)
	if err != nil {
		return nil, err
	}

	data := []uint16{}
	for idx := 0; idx < len(buf); idx += wordLength + crcLength {
		wordBytes := buf[idx : idx+2]
		expectedCrc := buf[idx+2]
		actualCrc := crc8.Checksum(wordBytes, checksumTable)
		if actualCrc != expectedCrc {
			return nil, errors.Errorf("failed to validate crc for %v (expected %v but got %v)", wordBytes, expectedCrc, actualCrc)
		}

		word := uint16(wordBytes[0])<<8 | uint16(wordBytes[1])
		data = append(data, word)
	}
	return data, nil
}
