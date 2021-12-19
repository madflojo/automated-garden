package server

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/calvinmclean/automated-garden/garden-app/pkg"
	"github.com/calvinmclean/automated-garden/garden-app/pkg/action"
	"github.com/rs/xid"
)

func TestPlantRequest(t *testing.T) {
	pp := uint(0)
	now := time.Now()
	tests := []struct {
		name string
		pr   *PlantRequest
		err  string
	}{
		{
			"EmptyRequest",
			nil,
			"missing required Plant fields",
		},
		{
			"EmptyPlantError",
			&PlantRequest{},
			"missing required Plant fields",
		},
		{
			"EmptyPlantPositionError",
			&PlantRequest{
				Plant: &pkg.Plant{
					Name: "plant",
				},
			},
			"missing required plant_position field",
		},
		{
			"EmptyWaterScheduleError",
			&PlantRequest{
				Plant: &pkg.Plant{
					Name:          "plant",
					PlantPosition: &pp,
				},
			},
			"missing required water_schedule field",
		},
		{
			"EmptyWaterScheduleIntervalError",
			&PlantRequest{
				Plant: &pkg.Plant{
					Name:          "plant",
					PlantPosition: &pp,
					WaterSchedule: &pkg.WaterSchedule{
						Duration: "1000ms",
					},
				},
			},
			"missing required water_schedule.interval field",
		},
		{
			"EmptyWaterScheduleDurationError",
			&PlantRequest{
				Plant: &pkg.Plant{
					Name:          "plant",
					PlantPosition: &pp,
					WaterSchedule: &pkg.WaterSchedule{
						Interval: "24h",
					},
				},
			},
			"missing required water_schedule.duration field",
		},
		{
			"EmptyWaterScheduleStartTimeError",
			&PlantRequest{
				Plant: &pkg.Plant{
					Name:          "plant",
					PlantPosition: &pp,
					WaterSchedule: &pkg.WaterSchedule{
						Interval: "24h",
						Duration: "1000ms",
					},
				},
			},
			"missing required water_schedule.start_time field",
		},
		{
			"InvalidDurationStringError",
			&PlantRequest{
				Plant: &pkg.Plant{
					Name:          "plant",
					PlantPosition: &pp,
					WaterSchedule: &pkg.WaterSchedule{
						Interval:  "24h",
						Duration:  "NOT A DURATION",
						StartTime: &now,
					},
				},
			},
			"invalid duration format for water_schedule.duration: NOT A DURATION",
		},
		{
			"EmptyNameError",
			&PlantRequest{
				Plant: &pkg.Plant{
					PlantPosition: &pp,
					WaterSchedule: &pkg.WaterSchedule{
						Interval:  "24h",
						Duration:  "1000ms",
						StartTime: &now,
					},
				},
			},
			"missing required name field",
		},
	}

	t.Run("Successful", func(t *testing.T) {
		pr := &PlantRequest{
			Plant: &pkg.Plant{
				Name:          "plant",
				PlantPosition: &pp,
				WaterSchedule: &pkg.WaterSchedule{
					Duration:  "1000ms",
					Interval:  "24h",
					StartTime: &now,
				},
			},
		}
		r := httptest.NewRequest("", "/", nil)
		err := pr.Bind(r)
		if err != nil {
			t.Errorf("Unexpected error reading PlantRequest JSON: %v", err)
		}
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("", "/", nil)
			err := tt.pr.Bind(r)
			if err == nil {
				t.Error("Expected error reading PlantRequest JSON, but none occurred")
				return
			}
			if err.Error() != tt.err {
				t.Errorf("Unexpected error string: %v", err)
			}
		})
	}
}

func TestUpdatePlantRequest(t *testing.T) {
	pp := uint(0)
	now := time.Now()
	past := now.Add(-1 * time.Hour)
	tests := []struct {
		name string
		pr   *UpdatePlantRequest
		err  string
	}{
		{
			"EmptyRequest",
			nil,
			"missing required Plant fields",
		},
		{
			"EmptyPlantError",
			&UpdatePlantRequest{},
			"missing required Plant fields",
		},
		{
			"ManualSpecificationOfIDError",
			&UpdatePlantRequest{
				Plant: &pkg.Plant{ID: xid.New()},
			},
			"updating ID is not allowed",
		},
		{
			"InvalidWaterScheduleDurationError",
			&UpdatePlantRequest{
				Plant: &pkg.Plant{
					WaterSchedule: &pkg.WaterSchedule{
						Duration: "NOT A DURATION",
					},
				},
			},
			"invalid duration format for water_schedule.duration: NOT A DURATION",
		},
		{
			"StartTimeInPastError",
			&UpdatePlantRequest{
				Plant: &pkg.Plant{
					WaterSchedule: &pkg.WaterSchedule{
						StartTime: &past,
					},
				},
			},
			"unable to set water_schedule.start_time to time in the past",
		},
		{
			"EndDateError",
			&UpdatePlantRequest{
				Plant: &pkg.Plant{
					EndDate: &now,
				},
			},
			"to end-date a Plant, please use the DELETE endpoint",
		},
	}

	t.Run("Successful", func(t *testing.T) {
		pr := &UpdatePlantRequest{
			Plant: &pkg.Plant{
				Name:          "plant",
				PlantPosition: &pp,
			},
		}
		r := httptest.NewRequest("", "/", nil)
		err := pr.Bind(r)
		if err != nil {
			t.Errorf("Unexpected error reading PlantRequest JSON: %v", err)
		}
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("", "/", nil)
			err := tt.pr.Bind(r)
			if err == nil {
				t.Error("Expected error reading PlantRequest JSON, but none occurred")
				return
			}
			if err.Error() != tt.err {
				t.Errorf("Unexpected error string: %v", err)
			}
		})
	}
}

func TestPlantActionRequest(t *testing.T) {
	tests := []struct {
		name string
		ar   *PlantActionRequest
		err  string
	}{
		{
			"EmptyRequestError",
			nil,
			"missing required action fields",
		},
		{
			"EmptyActionError",
			&PlantActionRequest{},
			"missing required action fields",
		},
		{
			"EmptyPlantActionError",
			&PlantActionRequest{
				PlantAction: &action.PlantAction{},
			},
			"missing required action fields",
		},
	}

	t.Run("Successful", func(t *testing.T) {
		ar := &PlantActionRequest{
			PlantAction: &action.PlantAction{
				Water: &action.WaterAction{},
			},
		}
		r := httptest.NewRequest("", "/", nil)
		err := ar.Bind(r)
		if err != nil {
			t.Errorf("Unexpected error reading PlantActionRequest JSON: %v", err)
		}
	})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("", "/", nil)
			err := tt.ar.Bind(r)
			if err == nil {
				t.Error("Expected error reading PlantActionRequest JSON, but none occurred")
				return
			}
			if err.Error() != tt.err {
				t.Errorf("Unexpected error string: %v", err)
			}
		})
	}
}
