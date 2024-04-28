package storage

import (
	"fmt"

	"github.com/calvinmclean/automated-garden/garden-app/pkg"
	"github.com/calvinmclean/automated-garden/garden-app/pkg/weather"

	"github.com/calvinmclean/babyapi"
	"github.com/calvinmclean/babyapi/storage/kv"
	"github.com/madflojo/hord"
	"github.com/madflojo/hord/drivers/hashmap"
	"github.com/madflojo/hord/drivers/redis"
	"github.com/mitchellh/mapstructure"
)

// Config is used to identify and configure a storage client
type Config struct {
	Driver  string                 `mapstructure:"driver"`
	Options map[string]interface{} `mapstructure:"options"`
}

type Client struct {
	Gardens              babyapi.Storage[*pkg.Garden]
	Zones                babyapi.Storage[*pkg.Zone]
	WaterSchedules       babyapi.Storage[*pkg.WaterSchedule]
	WeatherClientConfigs babyapi.Storage[*weather.Config]
}

func NewClient(config Config) (*Client, error) {
	db, err := newHordDB(config)
	if err != nil {
		return nil, fmt.Errorf("error creating base client: %w", err)
	}

	return &Client{
		Gardens:              kv.NewClient[*pkg.Garden](db, "Garden"),
		Zones:                kv.NewClient[*pkg.Zone](db, "Zone"),
		WaterSchedules:       kv.NewClient[*pkg.WaterSchedule](db, "WaterSchedule"),
		WeatherClientConfigs: kv.NewClient[*weather.Config](db, "WeatherClient"),
	}, nil
}

// newHordDB will create a new DB connection for one of the supported hord backends:
//   - hashmap
//   - redis
func newHordDB(config Config) (hord.Database, error) {
	switch config.Driver {
	case "hashmap":
		var cfg hashmap.Config
		err := mapstructure.Decode(config.Options, &cfg)
		if err != nil {
			return nil, fmt.Errorf("error decoding config: %w", err)
		}
		return kv.NewFileDB(cfg)
	case "redis":
		var cfg redis.Config
		err := mapstructure.Decode(config.Options, &cfg)
		if err != nil {
			return nil, fmt.Errorf("error decoding config: %w", err)
		}
		return kv.NewRedisDB(cfg)
	default:
		return nil, fmt.Errorf("invalid KV driver: %q", config.Driver)
	}
}
