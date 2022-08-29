package sensironsgp30_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-sensors/core/gas"
	"github.com/go-sensors/core/io/mocks"
	"github.com/go-sensors/core/units"
	"github.com/go-sensors/sensironsgp30"
	"github.com/golang/mock/gomock"
	"github.com/sigurn/crc8"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/errgroup"
)

func Test_NewSensor_returns_a_configured_sensor(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	portFactory := mocks.NewMockPortFactory(ctrl)

	// Act
	sensor := sensironsgp30.NewSensor(portFactory)

	// Assert
	assert.NotNil(t, sensor)
	assert.Equal(t, sensironsgp30.DefaultReconnectTimeout, sensor.ReconnectTimeout())
	assert.Nil(t, sensor.RecoverableErrorHandler())
}

func Test_NewSensor_with_options_returns_a_configured_sensor(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	portFactory := mocks.NewMockPortFactory(ctrl)
	expectedReconnectTimeout := sensironsgp30.DefaultReconnectTimeout * 10

	// Act
	sensor := sensironsgp30.NewSensor(portFactory,
		sensironsgp30.WithReconnectTimeout(expectedReconnectTimeout),
		sensironsgp30.WithRecoverableErrorHandler(func(err error) bool { return true }))

	// Assert
	assert.NotNil(t, sensor)
	assert.Equal(t, expectedReconnectTimeout, sensor.ReconnectTimeout())
	assert.NotNil(t, sensor.RecoverableErrorHandler())
	assert.True(t, sensor.RecoverableErrorHandler()(nil))
}

func Test_ConcentrationSpecs_returns_supported_concentrations(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	portFactory := mocks.NewMockPortFactory(ctrl)
	sensor := sensironsgp30.NewSensor(portFactory)
	expected := []*gas.ConcentrationSpec{
		{
			Gas:              sensironsgp30.TotalVolatileOrganicCompounds,
			Resolution:       1 * units.PartPerBillion,
			MinConcentration: 0 * units.PartPerBillion,
			MaxConcentration: 2008 * units.PartPerBillion,
		},
		{
			Gas:              sensironsgp30.TotalVolatileOrganicCompounds,
			Resolution:       6 * units.PartPerBillion,
			MinConcentration: 2009 * units.PartPerBillion,
			MaxConcentration: 11110 * units.PartPerBillion,
		},
		{
			Gas:              sensironsgp30.TotalVolatileOrganicCompounds,
			Resolution:       32 * units.PartPerBillion,
			MinConcentration: 11111 * units.PartPerBillion,
			MaxConcentration: 60000 * units.PartPerBillion,
		},
		{
			Gas:              sensironsgp30.CarbonDioxideEquivalent,
			Resolution:       1 * units.PartPerMillion,
			MinConcentration: 400 * units.PartPerMillion,
			MaxConcentration: 1479 * units.PartPerMillion,
		},
		{
			Gas:              sensironsgp30.CarbonDioxideEquivalent,
			Resolution:       3 * units.PartPerMillion,
			MinConcentration: 1480 * units.PartPerMillion,
			MaxConcentration: 5144 * units.PartPerMillion,
		},
		{
			Gas:              sensironsgp30.CarbonDioxideEquivalent,
			Resolution:       9 * units.PartPerMillion,
			MinConcentration: 5145 * units.PartPerMillion,
			MaxConcentration: 17597 * units.PartPerMillion,
		},
		{
			Gas:              sensironsgp30.CarbonDioxideEquivalent,
			Resolution:       31 * units.PartPerMillion,
			MinConcentration: 17598 * units.PartPerMillion,
			MaxConcentration: 60000 * units.PartPerMillion,
		},
	}

	// Act
	actual := sensor.ConcentrationSpecs()

	// Assert
	assert.NotNil(t, actual)
	assert.EqualValues(t, expected, actual)
}

func Test_Run_fails_when_opening_port(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	portFactory := mocks.NewMockPortFactory(ctrl)
	portFactory.EXPECT().
		Open().
		Return(nil, errors.New("boom"))
	sensor := sensironsgp30.NewSensor(portFactory)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	group, ctx := errgroup.WithContext(ctx)

	// Act
	group.Go(func() error {
		return sensor.Run(ctx)
	})
	err := group.Wait()

	// Assert
	assert.ErrorContains(t, err, "failed to open port")
}

func Test_Run_fails_to_initialize_sensor(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	portFactory := mocks.NewMockPortFactory(ctrl)

	port := mocks.NewMockPort(ctrl)
	portFactory.EXPECT().
		Open().
		Return(port, nil)

	port.EXPECT().
		Write([]byte{0x20, 0x03}).
		Return(0, errors.New("boom"))
	port.EXPECT().
		Close().
		Return(nil)

	sensor := sensironsgp30.NewSensor(portFactory,
		sensironsgp30.WithRecoverableErrorHandler(func(err error) bool { return true }))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	group, ctx := errgroup.WithContext(ctx)

	// Act
	group.Go(func() error {
		return sensor.Run(ctx)
	})
	err := group.Wait()

	// Assert
	assert.ErrorContains(t, err, "failed to initialize sensor")
}

func Test_handleCommand_fails_to_request_air_quality(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	portFactory := mocks.NewMockPortFactory(ctrl)

	port := mocks.NewMockPort(ctrl)
	portFactory.EXPECT().
		Open().
		Return(port, nil)

	port.EXPECT().
		Write([]byte{0x20, 0x03}).
		Return(0, nil)
	port.EXPECT().
		Write([]byte{0x20, 0x08}).
		Return(0, errors.New("boom"))
	port.EXPECT().
		Close().
		Return(nil)

	sensor := sensironsgp30.NewSensor(portFactory,
		sensironsgp30.WithRecoverableErrorHandler(func(err error) bool { return true }))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	group, ctx := errgroup.WithContext(ctx)

	// Act
	group.Go(func() error {
		return sensor.Run(ctx)
	})
	err := group.Wait()

	// Assert
	assert.ErrorContains(t, err, "failed to measure air quality")
}

func Test_handleCommand_fails_to_read_air_quality(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	portFactory := mocks.NewMockPortFactory(ctrl)

	port := mocks.NewMockPort(ctrl)
	portFactory.EXPECT().
		Open().
		Return(port, nil)

	port.EXPECT().
		Write([]byte{0x20, 0x03}).
		Return(0, nil)
	port.EXPECT().
		Write([]byte{0x20, 0x08}).
		Return(0, nil)
	port.EXPECT().
		Read(gomock.Any()).
		Return(0, errors.New("boom"))
	port.EXPECT().
		Close().
		Return(nil)

	sensor := sensironsgp30.NewSensor(portFactory,
		sensironsgp30.WithRecoverableErrorHandler(func(err error) bool { return true }))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	group, ctx := errgroup.WithContext(ctx)

	// Act
	group.Go(func() error {
		return sensor.Run(ctx)
	})
	err := group.Wait()

	// Assert
	assert.ErrorContains(t, err, "failed to read air quality")
}

func Test_handleCommand_handles_bad_CRC_while_reading_air_quality(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	portFactory := mocks.NewMockPortFactory(ctrl)

	port := mocks.NewMockPort(ctrl)
	portFactory.EXPECT().
		Open().
		Return(port, nil)

	port.EXPECT().
		Write([]byte{0x20, 0x03}).
		Return(0, nil)
	port.EXPECT().
		Write([]byte{0x20, 0x08}).
		Return(0, nil)
	port.EXPECT().
		Read(gomock.Any()).
		DoAndReturn(func(buf []byte) (int, error) {
			buf[0] = 0x01 // CO2eq MSB
			buf[1] = 0x02 // CO2eq LSB
			buf[2] = 0x00 // CO2eq CRC
			buf[3] = 0x03 // TVOC MSB
			buf[4] = 0x04 // TVOC LSB
			buf[5] = 0x00 // TVOC CRC

			return len(buf), nil
		})
	port.EXPECT().
		Close().
		Return(nil)

	sensor := sensironsgp30.NewSensor(portFactory,
		sensironsgp30.WithRecoverableErrorHandler(func(err error) bool { return true }))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	group, ctx := errgroup.WithContext(ctx)

	// Act
	group.Go(func() error {
		return sensor.Run(ctx)
	})
	err := group.Wait()

	// Assert
	assert.ErrorContains(t, err, "failed to validate crc")
}

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

func Test_handleCommand_returns_expected_measurement(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	portFactory := mocks.NewMockPortFactory(ctrl)

	port := mocks.NewMockPort(ctrl)
	portFactory.EXPECT().
		Open().
		Return(port, nil)

	port.EXPECT().
		Write([]byte{0x20, 0x03}).
		Return(0, nil)
	port.EXPECT().
		Write([]byte{0x20, 0x08}).
		Return(0, nil)

	expectedTotalVolatileOrganicCompounds := gas.Concentration{
		Gas:    sensironsgp30.TotalVolatileOrganicCompounds,
		Amount: 50000 * units.PartPerBillion,
	}
	expectedCarbonDioxideEquivalent := gas.Concentration{
		Gas:    sensironsgp30.CarbonDioxideEquivalent,
		Amount: 60000 * units.PartPerMillion,
	}
	port.EXPECT().
		Read(gomock.Any()).
		DoAndReturn(func(buf []byte) (int, error) {
			co2eq := uint16(expectedCarbonDioxideEquivalent.Amount.PartsPerMillion())
			buf[0] = byte((co2eq >> 8) & 0xFF)              // CO2eq MSB
			buf[1] = byte(co2eq & 0xFF)                     // CO2eq LSB
			buf[2] = crc8.Checksum(buf[0:2], checksumTable) // CO2eq CRC

			tvoc := uint16(expectedTotalVolatileOrganicCompounds.Amount.PartsPerBillion())
			buf[3] = byte((tvoc >> 8) & 0xFF)               // TVOC MSB
			buf[4] = byte(tvoc & 0xFF)                      // TVOC LSB
			buf[5] = crc8.Checksum(buf[3:5], checksumTable) // TVOC CRC

			return len(buf), nil
		})
	port.EXPECT().
		Close().
		Return(nil)

	sensor := sensironsgp30.NewSensor(portFactory,
		sensironsgp30.WithRecoverableErrorHandler(func(err error) bool { return true }))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	group, ctx := errgroup.WithContext(ctx)

	// Act
	group.Go(func() error {
		return sensor.Run(ctx)
	})
	group.Go(func() error {
		select {
		case actualTotalVolatileOrganicCompounds, ok := <-sensor.Concentrations():
			assert.True(t, ok)
			assert.NotNil(t, actualTotalVolatileOrganicCompounds)
			assert.Equal(t, expectedTotalVolatileOrganicCompounds, *actualTotalVolatileOrganicCompounds)
		case <-time.After(3 * time.Second):
			assert.Fail(t, "failed to receive Total VOC in expected amount of time")
		}

		select {
		case actualCarbonDioxideEquivalent, ok := <-sensor.Concentrations():
			assert.True(t, ok)
			assert.NotNil(t, actualCarbonDioxideEquivalent)
			assert.Equal(t, expectedCarbonDioxideEquivalent, *actualCarbonDioxideEquivalent)
		case <-time.After(3 * time.Second):
			assert.Fail(t, "failed to receive CO2 equivalent in expected amount of time")
		}

		cancel()
		return nil
	})
	err := group.Wait()

	// Assert
	assert.Nil(t, err)
}

func Test_Run_attempts_to_recover_from_failure(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)

	port := mocks.NewMockPort(ctrl)
	port.EXPECT().
		Write(gomock.Any()).
		Return(0, errors.New("boom")).
		AnyTimes()
	port.EXPECT().
		Close().
		Times(1)

	portFactory := mocks.NewMockPortFactory(ctrl)
	portFactory.EXPECT().
		Open().
		Return(port, nil)

	sensor := sensironsgp30.NewSensor(portFactory,
		sensironsgp30.WithRecoverableErrorHandler(func(err error) bool { return false }))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	group, ctx := errgroup.WithContext(ctx)

	// Act
	group.Go(func() error {
		return sensor.Run(ctx)
	})
	err := group.Wait()

	// Assert
	assert.Nil(t, err)
}

func Test_handleCommand_fails_to_set_humidity(t *testing.T) {
	// Arrange
	ctrl := gomock.NewController(t)
	portFactory := mocks.NewMockPortFactory(ctrl)

	port := mocks.NewMockPort(ctrl)
	portFactory.EXPECT().
		Open().
		Return(port, nil)

	port.EXPECT().
		Write([]byte{0x20, 0x03}).
		Return(0, nil)
	expectedRelativeHumidity := units.RelativeHumidity{
		Temperature: 25 * units.DegreeCelsius,
		Percentage:  0.5,
	}
	fixedPointValue := uint16(expectedRelativeHumidity.AbsoluteHumidity().GramsPerCubicMeter() * 256)
	humidityData := []byte{byte(fixedPointValue >> 8), byte(fixedPointValue)}
	humidityCRC := crc8.Checksum(humidityData, checksumTable)
	port.EXPECT().
		Write([]byte{0x20, 0x61, humidityData[0], humidityData[1], humidityCRC}).
		Return(0, errors.New("boom"))
	port.EXPECT().
		Close().
		Return(nil)

	sensor := sensironsgp30.NewSensor(portFactory,
		sensironsgp30.WithRecoverableErrorHandler(func(err error) bool { return true }))

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	group, ctx := errgroup.WithContext(ctx)

	// Act
	group.Go(func() error {
		return sensor.HandleRelativeHumidity(ctx, &expectedRelativeHumidity)
	})
	group.Go(func() error {
		return sensor.Run(ctx)
	})
	err := group.Wait()

	// Assert
	assert.ErrorContains(t, err, "failed to set humidity")
}
