package sensironsgp30_test

import (
	"testing"

	"github.com/go-sensors/core/i2c"
	"github.com/go-sensors/sensironsgp30"
	"github.com/stretchr/testify/assert"
)

func Test_GetDefaultI2CPortConfig_returns_expected_configuration(t *testing.T) {
	// Arrange
	expected := &i2c.I2CPortConfig{
		Address: 0x58,
	}

	// Act
	actual := sensironsgp30.GetDefaultI2CPortConfig()

	// Assert
	assert.NotNil(t, actual)
	assert.EqualValues(t, expected, actual)
}
