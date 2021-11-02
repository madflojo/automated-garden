package server

import (
	"errors"
	"net/http"
	"regexp"

	"github.com/calvinmclean/automated-garden/garden-app/pkg"
	"github.com/rs/xid"
)

// PlantRequest wraps a Plant into a request so we can handle Bind/Render in this package
type PlantRequest struct {
	*pkg.Plant
}

// Bind is used to make this struct compatible with the go-chi webserver for reading incoming
// JSON requests
func (p *PlantRequest) Bind(r *http.Request) error {
	if p == nil || p.Plant == nil {
		return errors.New("missing required Plant fields")
	}

	if p.WateringStrategy == (pkg.WateringStrategy{}) {
		return errors.New("missing required watering_strategy field")
	}
	if p.WateringStrategy.Interval == "" {
		return errors.New("missing required watering_strategy.interval field")
	}
	if p.WateringStrategy.WateringAmount == 0 {
		return errors.New("missing required watering_strategy.watering_amount field")
	}
	if p.Name == "" {
		return errors.New("missing required name field")
	}
	if p.GardenID != xid.NilID() {
		return errors.New("manual specification of garden ID is not allowed")
	}

	return nil
}

// PlantActionRequest wraps a AggregateAction into a request so we can handle Bind/Render in this package
type PlantActionRequest struct {
	*pkg.PlantAction
}

// Bind is used to make this struct compatible with our REST API implemented with go-chi.
// It will verify that the request is valid
func (action *PlantActionRequest) Bind(r *http.Request) error {
	// a.AggregateAction is nil if no AggregateAction fields are sent in the request. Return an
	// error to avoid a nil pointer dereference.
	if action == nil || action.PlantAction == nil || (action.Water == nil) {
		return errors.New("missing required action fields")
	}
	return nil
}

// GardenRequest wraps a Garden into a request so we can handle Bind/Render in this package
type GardenRequest struct {
	*pkg.Garden
}

// Bind is used to make this struct compatible with the go-chi webserver for reading incoming
// JSON requests
func (g *GardenRequest) Bind(r *http.Request) error {
	if g == nil || g.Garden == nil {
		return errors.New("missing required Garden fields")
	}
	if g.Name == "" {
		return errors.New("missing required name field")
	}
	illegalRegexp := regexp.MustCompile(`[\$\#\*\>\+\/]`)
	if illegalRegexp.MatchString(g.Name) {
		return errors.New("one or more invalid characters in Garden name")
	}
	if len(g.Plants) > 0 {
		return errors.New("cannot add or modify Plants with this request")
	}
	return nil
}
