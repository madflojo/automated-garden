package controller

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"time"

	"golang.org/x/term"
)

const (
	configTemplate = `#ifndef config_h
#define config_h

#define TOPIC_PREFIX "{{ .TopicPrefix }}"

#define QUEUE_SIZE 10

#define ENABLE_WIFI
#ifdef ENABLE_WIFI
#define MQTT_ADDRESS "{{ .MQTTConfig.Broker }}"
#define MQTT_PORT {{ .MQTTConfig.Port }}
#define MQTT_CLIENT_NAME TOPIC_PREFIX
#define MQTT_WATER_TOPIC TOPIC_PREFIX"/command/water"
#define MQTT_STOP_TOPIC TOPIC_PREFIX"/command/stop"
#define MQTT_STOP_ALL_TOPIC TOPIC_PREFIX"/command/stop_all"
#define MQTT_LIGHT_TOPIC TOPIC_PREFIX"/command/light"
#define MQTT_LIGHT_DATA_TOPIC TOPIC_PREFIX"/data/light"
#define MQTT_WATER_DATA_TOPIC TOPIC_PREFIX"/data/water"

{{ if .PublishHealth -}}
#define ENABLE_MQTT_HEALTH
#ifdef ENABLE_MQTT_HEALTH
#define MQTT_HEALTH_DATA_TOPIC TOPIC_PREFIX"/data/health"
#define HEALTH_PUBLISH_INTERVAL {{ .HealthInterval }}
#endif
{{- end }}

#define ENABLE_MQTT_LOGGING
#ifdef ENABLE_MQTT_LOGGING
#define MQTT_LOGGING_TOPIC TOPIC_PREFIX"/data/logs"
#endif

#define JSON_CAPACITY 48
#endif

#define NUM_ZONES {{ .NumZones }}
#define ZONES { {{ range $p := .Zones }}{ {{ $p.PumpPin }}, {{ $p.ValvePin }}, {{ $p.ButtonPin }}, {{ $p.MoistureSensorPin }} }{{ end }} }
#define DEFAULT_WATER_TIME {{ .DefaultWaterTime }}

#define LIGHT_PIN {{ .LightPin }}

{{ if .EnableButtons -}}
#define ENABLE_BUTTONS
#ifdef ENABLE_BUTTONS
#define STOP_BUTTON_PIN {{ .StopButtonPin }}
#endif
{{- end }}

{{ if .EnableMoistureSensor -}}
#ifdef ENABLE_MOISTURE_SENSORS AND ENABLE_WIFI
#define MQTT_MOISTURE_DATA_TOPIC TOPIC_PREFIX"/data/moisture"
#define MOISTURE_SENSOR_AIR_VALUE 3415
#define MOISTURE_SENSOR_WATER_VALUE 1362
#define MOISTURE_SENSOR_INTERVAL {{ milliseconds .MoistureInterval }}
#endif
{{ end }}
#endif
`
	wifiConfigTemplate = `#ifndef wifi_config_h
#define wifi_config_h

#define SSID "{{ .SSID }}"
#define PASSWORD "{{ .Password }}"

#endif
`
)

type WifiConfig struct {
	SSID     string `mapstructure:"ssid"`
	Password string `mapstructure:"password"`
}

type ZoneConfig struct {
	PumpPin           string `mapstructure:"pump_pin"`
	ValvePin          string `mapstructure:"valve_pin"`
	ButtonPin         string `mapstructure:"button_pin"`
	MoistureSensorPin string `mapstructure:"moisture_sensor_pin"`
}

func GenerateConfig(config Config) {
	mainConfig, err := generateMainConfig(config)
	if err != nil {
		fmt.Printf("error generating 'config.h': %v", err)
	}
	fmt.Println(mainConfig)
	fmt.Println("====")
	wifiConfig, err := generateWiFiConfig(config)
	if err != nil {
		fmt.Printf("error generating 'wifi_config.h': %v", err)
	}
	fmt.Println(wifiConfig)
}

func generateMainConfig(config Config) (string, error) {
	milliseconds := func(interval time.Duration) string {
		return fmt.Sprintf("%d", interval.Milliseconds())
	}
	t := template.Must(template.
		New("config.h").
		Funcs(template.FuncMap{"milliseconds": milliseconds}).
		Parse(configTemplate))

	var result bytes.Buffer
	data := config
	err := t.Execute(&result, data)
	if err != nil {
		return "", err
	}
	return result.String(), nil
}

func generateWiFiConfig(config Config) (string, error) {
	if config.WifiConfig.Password == "" {
		fmt.Print("WiFi password: ")
		password, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			return "", nil
		}

		config.WifiConfig.Password = string(password)
	}

	t := template.Must(template.New("wifi_config.h").Parse(wifiConfigTemplate))
	var result bytes.Buffer
	err := t.Execute(&result, config.WifiConfig)
	if err != nil {
		return "", err
	}
	return result.String(), nil
}
