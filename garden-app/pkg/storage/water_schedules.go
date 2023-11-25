package storage

import (
	"fmt"

	"github.com/calvinmclean/automated-garden/garden-app/pkg"
)

// GetZonesUsingWaterSchedule will find all Zones that use this WaterSchedule and return the Zones along with the Gardens they belong to
func (c *Client) GetZonesUsingWaterSchedule(id string) ([]*pkg.ZoneAndGarden, error) {
	gardens, err := c.Gardens.GetAll(FilterEndDated[*pkg.Garden](false))
	if err != nil {
		return nil, fmt.Errorf("unable to get all Gardens: %w", err)
	}

	results := []*pkg.ZoneAndGarden{}
	for _, g := range gardens {
		zones, err := c.Zones.GetAll(func(z *pkg.Zone) bool {
			return z.GardenID == g.ID && !z.EndDated()
		})
		if err != nil {
			return nil, fmt.Errorf("unable to get all Zones for Garden %q: %w", g.ID, err)
		}

		for _, z := range zones {
			for _, wsID := range z.WaterScheduleIDs {
				if wsID.String() == id {
					results = append(results, &pkg.ZoneAndGarden{Zone: z, Garden: g})
				}
			}
		}
	}

	return results, nil
}
