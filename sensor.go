package sensironsgp30

import (
	"context"
	"time"

	"github.com/go-sensors/core/gas"
	coreio "github.com/go-sensors/core/io"
	"github.com/go-sensors/core/units"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

const (
	TotalVolatileOrganicCompounds string = "TVOC"
	CarbonDioxideEquivalent       string = "CO2eq"
)

// Sensor represents a configured Sensiron SGP30 gas sensor
type Sensor struct {
	gases            chan *gas.Concentration
	portFactory      coreio.PortFactory
	reconnectTimeout time.Duration
	errorHandlerFunc ShouldTerminate
	commands         chan interface{}
}

// Option is a configured option that may be applied to a Sensor
type Option struct {
	apply func(*Sensor)
}

// NewSensor creates a Sensor with optional configuration
func NewSensor(portFactory coreio.PortFactory, options ...*Option) *Sensor {
	gases := make(chan *gas.Concentration)
	commands := make(chan interface{})
	s := &Sensor{
		gases:            gases,
		portFactory:      portFactory,
		reconnectTimeout: DefaultReconnectTimeout,
		errorHandlerFunc: nil,
		commands:         commands,
	}
	for _, o := range options {
		o.apply(s)
	}
	return s
}

// WithReconnectTimeout specifies the duration to wait before reconnecting after a recoverable error
func WithReconnectTimeout(timeout time.Duration) *Option {
	return &Option{
		apply: func(s *Sensor) {
			s.reconnectTimeout = timeout
		},
	}
}

// ReconnectTimeout is the duration to wait before reconnecting after a recoverable error
func (s *Sensor) ReconnectTimeout() time.Duration {
	return s.reconnectTimeout
}

// ShouldTerminate is a function that returns a result indicating whether the Sensor should terminate after a recoverable error
type ShouldTerminate func(error) bool

// WithRecoverableErrorHandler registers a function that will be called when a recoverable error occurs
func WithRecoverableErrorHandler(f ShouldTerminate) *Option {
	return &Option{
		apply: func(s *Sensor) {
			s.errorHandlerFunc = f
		},
	}
}

// RecoverableErrorHandler a function that will be called when a recoverable error occurs
func (s *Sensor) RecoverableErrorHandler() ShouldTerminate {
	return s.errorHandlerFunc
}

const (
	setValueTimeout           time.Duration = 10 * time.Millisecond
	readValueTimeout          time.Duration = 12 * time.Millisecond
	measureAirQualityInterval time.Duration = 1 * time.Second
)

// Run begins reading from the sensor and blocks until either an error occurs or the context is completed
func (s *Sensor) Run(ctx context.Context) error {
	defer close(s.gases)
	defer close(s.commands)
	for {
		port, err := s.portFactory.Open()
		if err != nil {
			return errors.Wrap(err, "failed to open port")
		}

		group, innerCtx := errgroup.WithContext(ctx)
		group.Go(func() error {
			<-innerCtx.Done()
			return port.Close()
		})
		group.Go(func() error {
			err = initAirQuality(innerCtx, port)
			if err != nil {
				return errors.Wrap(err, "failed to initialize sensor")
			}

			group.Go(handleCommands(innerCtx, s.commands, s.gases, port))
			group.Go(requestAirQualityRepeatedly(innerCtx, s.commands))
			return nil
		})

		err = group.Wait()
		if s.errorHandlerFunc != nil {
			if s.errorHandlerFunc(err) {
				return err
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(s.reconnectTimeout):
		}
	}
}

// Concentrations returns a channel of concentration readings as they become available from the sensor
func (s *Sensor) Concentrations() <-chan *gas.Concentration {
	return s.gases
}

// ConcentrationSpecs returns a collection of specified measurement ranges supported by the sensor
func (*Sensor) ConcentrationSpecs() []*gas.ConcentrationSpec {
	return []*gas.ConcentrationSpec{
		{
			Gas:              TotalVolatileOrganicCompounds,
			Resolution:       1 * units.PartPerBillion,
			MinConcentration: 0 * units.PartPerBillion,
			MaxConcentration: 2008 * units.PartPerBillion,
		},
		{
			Gas:              TotalVolatileOrganicCompounds,
			Resolution:       6 * units.PartPerBillion,
			MinConcentration: 2009 * units.PartPerBillion,
			MaxConcentration: 11110 * units.PartPerBillion,
		},
		{
			Gas:              TotalVolatileOrganicCompounds,
			Resolution:       32 * units.PartPerBillion,
			MinConcentration: 11111 * units.PartPerBillion,
			MaxConcentration: 60000 * units.PartPerBillion,
		},
		{
			Gas:              CarbonDioxideEquivalent,
			Resolution:       1 * units.PartPerMillion,
			MinConcentration: 400 * units.PartPerMillion,
			MaxConcentration: 1479 * units.PartPerMillion,
		},
		{
			Gas:              CarbonDioxideEquivalent,
			Resolution:       3 * units.PartPerMillion,
			MinConcentration: 1480 * units.PartPerMillion,
			MaxConcentration: 5144 * units.PartPerMillion,
		},
		{
			Gas:              CarbonDioxideEquivalent,
			Resolution:       9 * units.PartPerMillion,
			MinConcentration: 5145 * units.PartPerMillion,
			MaxConcentration: 17597 * units.PartPerMillion,
		},
		{
			Gas:              CarbonDioxideEquivalent,
			Resolution:       31 * units.PartPerMillion,
			MinConcentration: 17598 * units.PartPerMillion,
			MaxConcentration: 60000 * units.PartPerMillion,
		},
	}
}

func (s *Sensor) HandleRelativeHumidity(ctx context.Context, relativeHumidity *units.RelativeHumidity) error {
	select {
	case <-ctx.Done():
	case s.commands <- relativeHumidity:
	}
	return nil
}

type requestAirQuality struct{}

func requestAirQualityRepeatedly(
	ctx context.Context,
	commands chan interface{}) func() error {
	request := &requestAirQuality{}
	return func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(measureAirQualityInterval):
				select {
				case <-ctx.Done():
					return nil
				case commands <- request:
				}
			}
		}
	}
}

func handleCommands(
	ctx context.Context,
	commands chan interface{},
	gases chan *gas.Concentration,
	port coreio.Port) func() error {
	return func() error {
		for {
			select {
			case <-ctx.Done():
				return nil
			case c := <-commands:
				switch command := c.(type) {
				case *units.RelativeHumidity:
					err := setHumidity(ctx, port, command.AbsoluteHumidity())
					if err != nil {
						return errors.Wrap(err, "failed to set humidity")
					}
				case *requestAirQuality:
					readings, err := measureAirQuality(ctx, port)
					if err != nil {
						return errors.Wrap(err, "failed to measure air quality")
					}

					tvoc := &gas.Concentration{
						Gas:    TotalVolatileOrganicCompounds,
						Amount: readings.TVOC,
					}

					select {
					case <-ctx.Done():
						return nil
					case gases <- tvoc:
					}

					co2eq := &gas.Concentration{
						Gas:    CarbonDioxideEquivalent,
						Amount: readings.CO2eq,
					}

					select {
					case <-ctx.Done():
						return nil
					case gases <- co2eq:
					}
				}
			}
		}
	}
}
