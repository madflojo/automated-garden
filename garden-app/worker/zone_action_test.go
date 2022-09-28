package worker

import (
	"errors"
	"testing"

	"github.com/calvinmclean/automated-garden/garden-app/pkg"
	"github.com/calvinmclean/automated-garden/garden-app/pkg/action"
	"github.com/calvinmclean/automated-garden/garden-app/pkg/influxdb"
	"github.com/calvinmclean/automated-garden/garden-app/pkg/mqtt"
	"github.com/calvinmclean/automated-garden/garden-app/pkg/weather"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestZoneAction(t *testing.T) {
	garden := &pkg.Garden{
		Name:        "garden",
		TopicPrefix: "garden",
	}

	tests := []struct {
		name      string
		action    *action.ZoneAction
		setupMock func(*mqtt.MockClient, *influxdb.MockClient)
		assert    func(error, *testing.T)
	}{
		{
			"SuccessfulEmptyZoneAction",
			&action.ZoneAction{},
			func(mqttClient *mqtt.MockClient, influxdbClient *influxdb.MockClient) {},
			func(err error, t *testing.T) {
				if err != nil {
					t.Errorf("Unexpected error occurred when executing ZoneAction: %v", err)
				}
			},
		},
		{
			"SuccessfulZoneActionWithWaterAction",
			&action.ZoneAction{
				Water: &action.WaterAction{
					Duration: 1000,
				},
			},
			func(mqttClient *mqtt.MockClient, influxdbClient *influxdb.MockClient) {
				mqttClient.On("WaterTopic", "garden").Return("garden/action/water", nil)
				mqttClient.On("Publish", "garden/action/water", mock.Anything).Return(nil)
			},
			func(err error, t *testing.T) {
				if err != nil {
					t.Errorf("Unexpected error occurred when executing ZoneAction: %v", err)
				}
			},
		},
		{
			"FailedZoneActionWithWaterAction",
			&action.ZoneAction{
				Water: &action.WaterAction{
					Duration: 1000,
				},
			},
			func(mqttClient *mqtt.MockClient, influxdbClient *influxdb.MockClient) {
				mqttClient.On("WaterTopic", "garden").Return("", errors.New("template error"))
			},
			func(err error, t *testing.T) {
				if err == nil {
					t.Error("Expected error, but nil was returned")
				}
				if err.Error() != "unable to execute WaterAction: unable to fill MQTT topic template: template error" {
					t.Errorf("Unexpected error string: %v", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zone := &pkg.Zone{
				Position:      uintPointer(0),
				WaterSchedule: &pkg.WaterSchedule{},
			}
			mqttClient := new(mqtt.MockClient)
			influxdbClient := new(influxdb.MockClient)
			tt.setupMock(mqttClient, influxdbClient)

			err := NewWorker(nil, influxdbClient, mqttClient, nil, logrus.New()).ExecuteZoneAction(garden, zone, tt.action)
			tt.assert(err, t)
			mqttClient.AssertExpectations(t)
			influxdbClient.AssertExpectations(t)
		})
	}
}

func TestWaterActionExecute(t *testing.T) {
	garden := &pkg.Garden{
		Name:        "garden",
		TopicPrefix: "garden",
	}
	action := &action.WaterAction{
		Duration: 1000,
	}
	fakeWeatherClient, err := weather.NewClient(weather.Config{
		Type: "fake",
		Options: map[string]interface{}{
			"rain_mm":       25.4,
			"rain_interval": "24h",
		},
	})
	assert.NoError(t, err)

	tests := []struct {
		name          string
		zone          *pkg.Zone
		setupMock     func(*mqtt.MockClient, *influxdb.MockClient)
		weatherClient weather.Client
		assert        func(error, *testing.T)
	}{
		{
			"Successful",
			&pkg.Zone{
				Position:      uintPointer(0),
				WaterSchedule: &pkg.WaterSchedule{},
			},
			func(mqttClient *mqtt.MockClient, influxdbClient *influxdb.MockClient) {
				mqttClient.On("WaterTopic", "garden").Return("garden/action/water", nil)
				mqttClient.On("Publish", "garden/action/water", mock.Anything).Return(nil)
			},
			nil,
			func(err error, t *testing.T) {
				if err != nil {
					t.Errorf("Unexpected error occurred when executing WaterAction: %v", err)
				}
			},
		},
		{
			"TopicTemplateError",
			&pkg.Zone{
				Position:      uintPointer(0),
				WaterSchedule: &pkg.WaterSchedule{},
			},
			func(mqttClient *mqtt.MockClient, influxdbClient *influxdb.MockClient) {
				mqttClient.On("WaterTopic", "garden").Return("", errors.New("template error"))
			},
			nil,
			func(err error, t *testing.T) {
				if err == nil {
					t.Error("Expected error, but nil was returned")
				}
				if err.Error() != "unable to fill MQTT topic template: template error" {
					t.Errorf("Unexpected error string: %v", err)
				}
			},
		},
		{
			"SuccessWhenMoistureLessThanMinimum",
			&pkg.Zone{
				Position: uintPointer(0),
				WaterSchedule: &pkg.WaterSchedule{
					MinimumMoisture: 50,
				},
			},
			func(mqttClient *mqtt.MockClient, influxdbClient *influxdb.MockClient) {
				mqttClient.On("WaterTopic", "garden").Return("garden/action/water", nil)
				mqttClient.On("Publish", "garden/action/water", mock.Anything).Return(nil)
				influxdbClient.On("GetMoisture", mock.Anything, uint(0), garden.Name).Return(float64(0), nil)
				influxdbClient.On("Close")
			},
			nil,
			func(err error, t *testing.T) {
				if err != nil {
					t.Errorf("Unexpected error occurred when executing WaterAction: %v", err)
				}
			},
		},
		{
			"SuccessWhenMoistureGreaterThanMinimum",
			&pkg.Zone{
				Position: uintPointer(0),
				WaterSchedule: &pkg.WaterSchedule{
					MinimumMoisture: 50,
				},
			},
			func(mqttClient *mqtt.MockClient, influxdbClient *influxdb.MockClient) {
				influxdbClient.On("GetMoisture", mock.Anything, uint(0), garden.Name).Return(float64(51), nil)
				influxdbClient.On("Close")
				// No MQTT calls made
			},
			nil,
			func(err error, t *testing.T) {
				assert.NoError(t, err)
			},
		},
		{
			"ErrorInfluxDBClient",
			&pkg.Zone{
				Position: uintPointer(0),
				WaterSchedule: &pkg.WaterSchedule{
					MinimumMoisture: 50,
				},
			},
			func(mqttClient *mqtt.MockClient, influxdbClient *influxdb.MockClient) {
				influxdbClient.On("GetMoisture", mock.Anything, uint(0), garden.Name).Return(float64(0), errors.New("influxdb error"))
				influxdbClient.On("Close")
			},
			nil,
			func(err error, t *testing.T) {
				if err == nil {
					t.Error("Expected error, but nil was returned")
				}
				if err.Error() != "error getting Zone's moisture data: influxdb error" {
					t.Errorf("Unexpected error string: %v", err)
				}
			},
		},
		{
			"SuccessfulRainDelay",
			&pkg.Zone{
				Position: uintPointer(0),
				WaterSchedule: &pkg.WaterSchedule{
					Interval: "24h",
					WeatherControl: &weather.Control{
						Rain: &weather.RainControl{
							Threshold: 25.4,
						},
					},
				},
			},
			func(mqttClient *mqtt.MockClient, influxdbClient *influxdb.MockClient) {
				// No MQTT calls made
			},
			fakeWeatherClient,
			func(err error, t *testing.T) {
				assert.NoError(t, err)
			},
		},
		{
			"SuccessfulNoRainDelay",
			&pkg.Zone{
				Position: uintPointer(0),
				WaterSchedule: &pkg.WaterSchedule{
					Interval: "24h",
					WeatherControl: &weather.Control{
						Rain: &weather.RainControl{
							Threshold: 30,
						},
					},
				},
			},
			func(mqttClient *mqtt.MockClient, influxdbClient *influxdb.MockClient) {
				mqttClient.On("WaterTopic", "garden").Return("garden/action/water", nil)
				mqttClient.On("Publish", "garden/action/water", mock.Anything).Return(nil)
			},
			fakeWeatherClient,
			func(err error, t *testing.T) {
				assert.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mqttClient := new(mqtt.MockClient)
			influxdbClient := new(influxdb.MockClient)
			tt.setupMock(mqttClient, influxdbClient)

			err := NewWorker(nil, influxdbClient, mqttClient, tt.weatherClient, logrus.New()).ExecuteWaterAction(garden, tt.zone, action)
			tt.assert(err, t)
			mqttClient.AssertExpectations(t)
			influxdbClient.AssertExpectations(t)
		})
	}
}

func uintPointer(n int) *uint {
	uintn := uint(n)
	return &uintn
}
