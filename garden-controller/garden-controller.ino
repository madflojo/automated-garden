#include <ArduinoJson.h>
#include <PubSubClient.h>
#include <stdio.h>
#include "driver/gpio.h"

/* include other files for this program */
#include "valve.h"
#include "wifi.h"
#include "config.h"

#define JSON_CAPACITY JSON_OBJECT_SIZE(3) + 40
#define QUEUE_SIZE 10

#define NUM_VALVES 3

#define INTERVAL 86400000 // 24 hours
#define DEFAULT_WATER_TIME 15000

#define DEBOUNCE_DELAY 50

#define MQTT_RETRY_DELAY 5000
#define MQTT_CLIENT_NAME "Garden"

typedef struct WateringEvent {
    int valve_id;
    unsigned long watering_time;
};

Valve valves[NUM_VALVES] = {
    Valve(0, VALVE_1_PIN, PUMP_PIN),
    Valve(1, VALVE_2_PIN, PUMP_PIN),
    Valve(2, VALVE_3_PIN, PUMP_PIN)
};
bool skipValve[NUM_VALVES] = { false, false, false };

/* button variables */
unsigned long lastDebounceTime = 0;
gpio_num_t buttons[NUM_VALVES] = { BUTTON_1_PIN, BUTTON_2_PIN, BUTTON_3_PIN };
int buttonStates[NUM_VALVES] = { LOW, LOW, LOW };
int lastButtonStates[NUM_VALVES] = { LOW, LOW, LOW };

/* stop button variables */
unsigned long lastStopDebounceTime = 0;
int stopButtonState = LOW;
int lastStopButtonState;

/* watering cycle variables */
unsigned long previousMillis = -INTERVAL;
int watering = -1;

/* MQTT variables */
unsigned long lastConnectAttempt = 0;
PubSubClient client(wifiClient);
const char* waterCommandTopic = "garden/command/water";
const char* stopCommandTopic = "garden/command/stop";
const char* stopAllCommandTopic = "garden/command/stop_all";
const char* skipCommandTopic = "garden/command/skip";
const char* waterDataTopic = "garden/data/water";

/* FreeRTOS Queue and Task handlers */
QueueHandle_t publisherQueue;
QueueHandle_t wateringQueue;
TaskHandle_t mqttConnectTaskHandle;
TaskHandle_t mqttLoopTaskHandle;
TaskHandle_t publisherTaskHandle;
TaskHandle_t waterPlantTaskHandle;
TaskHandle_t waterIntervalTaskHandle;
TaskHandle_t readButtonsTaskHandle;

void setup() {
    // Prepare pins
    for (int i = 0; i < NUM_VALVES; i++) {
        gpio_reset_pin(buttons[i]);
        gpio_set_direction(buttons[i], GPIO_MODE_INPUT);
    }

    // Connect to WiFi and MQTT
    setup_wifi();
    client.setServer(MQTT_ADDRESS, MQTT_PORT);
    client.setCallback(processIncomingMessage);

    // Initialize Queues
    publisherQueue = xQueueCreate(QUEUE_SIZE, sizeof(WateringEvent));
    if (publisherQueue == NULL) {
        printf("error creating the publisherQueue\n");
    }
    wateringQueue = xQueueCreate(QUEUE_SIZE, sizeof(WateringEvent));
    if (wateringQueue == NULL) {
        printf("error creating the wateringQueue\n");
    }

    // Start all tasks (currently using equal priorities)
    xTaskCreate(mqttConnectTask, "MQTTConnectTask", 2048, NULL, 1, &mqttConnectTaskHandle);
    xTaskCreate(mqttLoopTask, "MQTTLoopTask", 2048, NULL, 1, &mqttLoopTaskHandle);
    xTaskCreate(publisherTask, "PublisherTask", 2048, NULL, 1, &publisherTaskHandle);
    xTaskCreate(waterPlantTask, "WaterPlantTask", 2048, NULL, 1, &waterPlantTaskHandle);
    xTaskCreate(waterIntervalTask, "WaterIntervalTask", 2048, NULL, 1, &waterIntervalTaskHandle);
    xTaskCreate(readButtonsTask, "ReadButtonsTask", 2048, NULL, 1, &readButtonsTaskHandle);

    // I tested the stack sizes above by enabling this task which will print
    // the number of words remaining when that task's stack reached its highest
    // xTaskCreate(getStackSizesTask, "getStackSizesTask", 4096, NULL, 1, NULL);
}

void loop() {}

/*
  waterIntervalTask will queue up each plant to be watered fro the configured
  default time. Then it will wait during the configured interval and then loop
*/
void waterIntervalTask(void* parameters) {
    while (true) {
        // Every 24 hours, start watering plant 1
        unsigned long currentMillis = millis();
        if (currentMillis - previousMillis >= INTERVAL) {
            previousMillis = currentMillis;
            for (int i = 0; i < NUM_VALVES; i++) {
                waterPlant(i, DEFAULT_WATER_TIME);
            }
        }
        vTaskDelay(INTERVAL / portTICK_PERIOD_MS);
    }
    vTaskDelete(NULL);
}

/*
  waterPlantTask will wait for WateringEvents on a queue and will then open the
  valve for an amount of time. The delay before closing the valve is done with
  xTaskNotifyWait, allowing it to be interrupted with xTaskNotify. After the
  valve is closed, the WateringEvent is pushed to the queue fro publisherTask
  which will record the WateringEvent in InfluxDB via MQTT and Telegraf
*/
void waterPlantTask(void* parameters) {
    WateringEvent we;
    while (true) {
        if (xQueueReceive(wateringQueue, &we, 0)) {
            // First clear the notifications to prevent a bug that would cause
            // watering to be skipped if I run xTaskNotify when not waiting
            ulTaskNotifyTake(NULL, 0);

            if (we.watering_time == 0) {
                we.watering_time = DEFAULT_WATER_TIME;
            }

            // Only water if this valve isn't setup to be skipped
            if (!skipValve[we.valve_id]) {
                unsigned long start = millis();
                valves[we.valve_id].on();
                // Delay for specified watering time with option to interrupt
                xTaskNotifyWait(0x00, ULONG_MAX, NULL, we.watering_time / portTICK_PERIOD_MS);
                unsigned long stop = millis();
                valves[we.valve_id].off();
                we.watering_time = stop - start;
                xQueueSend(publisherQueue, &we, portMAX_DELAY);
            } else {
                printf("skipping watering for valve %d\n", we.valve_id);
                skipValve[we.valve_id] = false;
            }
        }
        vTaskDelay(5 / portTICK_PERIOD_MS);
    }
    vTaskDelete(NULL);
}

/*
  readButtonsTask will check if any buttons are being pressed
*/
void readButtonsTask(void* parameters) {
    while (true) {
        // Check if any valves need to be stopped and check all buttons
        for (int i = 0; i < NUM_VALVES; i++) {
            readButton(i);
        }
        readStopButton();
        vTaskDelay(5 / portTICK_PERIOD_MS);
    }
    vTaskDelete(NULL);
}

/*
  publisherTask reads from a queue and publish WateringEvents as an InfluxDB
  line protocol message to MQTT
*/
void publisherTask(void* parameters) {
    WateringEvent we;
    while (true) {
        if (xQueueReceive(publisherQueue, &we, portMAX_DELAY)) {
            char message[50];
            sprintf(message, "water,plant=%d millis=%lu", we.valve_id, we.watering_time);
            if (client.connected()) {
                printf("publishing to MQTT:\n\ttopic=%s\n\tmessage=%s\n", waterDataTopic, message);
                client.publish(waterDataTopic, message);
            } else {
                printf("unable to publish: not connected to MQTT broker\n");
            }
        }
        vTaskDelay(5 / portTICK_PERIOD_MS);
    }
    vTaskDelete(NULL);
}

/*
  mqttConnectTask will periodically attempt to reconnect to MQTT if needed
*/
void mqttConnectTask(void* parameters) {
    while (true) {
        // Connect to MQTT server if not connected already
        if (!client.connected()) {
            printf("attempting MQTT connection...");
            if (client.connect(MQTT_CLIENT_NAME)) {
                printf("connected\n");
                client.subscribe(waterCommandTopic);
                client.subscribe(stopCommandTopic);
                client.subscribe(stopAllCommandTopic);
                client.subscribe(skipCommandTopic);
            } else {
                printf("failed, rc=%zu\n", client.state());
            }
        }
        vTaskDelay(5000 / portTICK_PERIOD_MS);
    }
    vTaskDelete(NULL);
}

/*
  mqttLoopTask will run the MQTT client loop to listen on subscribed topics
*/
void mqttLoopTask(void* parameters) {
    while (true) {
        // Run MQTT loop to process incoming messages if connected
        if (client.connected()) {
            client.loop();
        }
        vTaskDelay(5 / portTICK_PERIOD_MS);
    }
    vTaskDelete(NULL);
}

/*
  getStackSizesTask is a tool used for debugging and testing that will give me
  information about the remaining words in each task's stack at its highest
*/
void getStackSizesTask(void* parameters) {
    while (true) {
        printf("mqttConnectTask stack high water mark: %d\n", uxTaskGetStackHighWaterMark(mqttConnectTaskHandle));
        printf("mqttLoopTask stack high water mark: %d\n", uxTaskGetStackHighWaterMark(mqttLoopTaskHandle));
        printf("publisherTask stack high water mark: %d\n", uxTaskGetStackHighWaterMark(publisherTaskHandle));
        printf("waterPlantTask stack high water mark: %d\n", uxTaskGetStackHighWaterMark(waterPlantTaskHandle));
        printf("waterIntervalTask stack high water mark: %d\n", uxTaskGetStackHighWaterMark(waterIntervalTaskHandle));
        printf("readButtonsTask stack high water mark: %d\n", uxTaskGetStackHighWaterMark(readButtonsTaskHandle));
        vTaskDelay(10000 / portTICK_PERIOD_MS);
    }
}

/*
  stopWatering will interrupt the WaterPlantTask. If another plant is in the queue,
  it will begin watering
*/
void stopWatering() {
    xTaskNotify(waterPlantTaskHandle, 0, eNoAction);
}

/*
  stopAllWatering will interrupt the WaterPlantTask and clear the remaining queue
*/
void stopAllWatering() {
    xQueueReset(wateringQueue);
    xTaskNotify(waterPlantTaskHandle, 0, eNoAction);
}

/*
  readButton takes an ID that represents the array index for the valve and button arrays
  and checks if the button is pressed. If the button is pressed, the following is done:
    - stop watering all plants
    - disable watering cycle
    - turn on the valve corresponding to this button
*/
void readButton(int valveID) {
    // Exit if valveID is out of bounds
    if (valveID >= NUM_VALVES || valveID < 0) {
        return;
    }
    int reading = gpio_get_level(buttons[valveID]);
    // If the switch changed, due to noise or pressing, reset debounce timer
    if (reading != lastButtonStates[valveID]) {
        lastDebounceTime = millis();
    }

    // Current reading has been the same longer than our delay, so now we can do something
    if ((millis() - lastDebounceTime) > DEBOUNCE_DELAY) {
        // If the button state has changed
        if (reading != buttonStates[valveID]) {
            buttonStates[valveID] = reading;

            // If our button state is HIGH, stop watering others and water this plant
            if (buttonStates[valveID] == HIGH) {
                if (reading == HIGH) {
                    printf("button pressed: %d\n", valveID);
                    waterPlant(valveID, DEFAULT_WATER_TIME);
                }
            }
        }
    }
    lastButtonStates[valveID] = reading;
}

/*
  readStopButton is similar to the readButton function, but had to be separated because this
  button does not correspond to a Valve and could not be included in the array of buttons.
*/
void readStopButton() {
    int reading = gpio_get_level(STOP_BUTTON_PIN);
    // If the switch changed, due to noise or pressing, reset debounce timer
    if (reading != lastStopButtonState) {
        lastStopDebounceTime = millis();
    }

    // Current reading has been the same longer than our delay, so now we can do something
    if ((millis() - lastStopDebounceTime) > DEBOUNCE_DELAY) {
        // If the button state has changed
        if (reading != stopButtonState) {
            stopButtonState = reading;

            // If our button state is HIGH, do some things
            if (stopButtonState == HIGH) {
                if (reading == HIGH) {
                    printf("stop button pressed\n");
                    stopWatering();
                }
            }
        }
    }
    lastStopButtonState = reading;
}

/*
  processIncomingMessage is a callback function for the MQTT client that will
  react to incoming messages. Currently, the topics are:
    - waterCommandTopic: accepts a WateringEvent JSON to water a plant for
                         specified time
    - stopCommandTopic: ignores message and stops the currently-watering plant
    - stopAllCommandTopic: ignores message, stops the currently-watering plant,
                           and clears the wateringQueue
    - skipCommandTopic:
*/
void processIncomingMessage(char* topic, byte* message, unsigned int length) {
    printf("message received:\n\ttopic=%s\n\tmessage=%s\n", topic, (char*)message);

    StaticJsonDocument<JSON_CAPACITY> doc;
    DeserializationError err = deserializeJson(doc, message);
    if (err) {
        printf("deserialize failed: %s\n", err.c_str());
    }

    WateringEvent we = {
        doc["valve_id"],
        doc["water_time"]
    };

    if (strcmp(topic, waterCommandTopic) == 0) {
        printf("received command to water plant %d for %lu\n", we.valve_id, we.watering_time);
        waterPlant(we.valve_id, we.watering_time);
    } else if (strcmp(topic, stopCommandTopic) == 0) {
        printf("received command to stop watering\n");
        stopWatering();
    } else if (strcmp(topic, stopAllCommandTopic) == 0) {
        printf("received command to stop ALL watering\n");
        stopAllWatering();
    } else if (strcmp(topic, skipCommandTopic) == 0) {
        printf("received command to skip next watering for plant %d\n", we.valve_id);
        skipValve[we.valve_id] = true;
    }
}

/*
  waterPlant pushes a WateringEvent to the queue in order to water a single
  plant. First it will make sure the ID is not out of bounds
*/
void waterPlant(int id, long time) {
    // Exit if valveID is out of bounds
    if (id >= NUM_VALVES || id < 0) {
        printf("valve ID %d is out of range, aborting request\n", id);
        return;
    }
    printf("pushing WateringEvent to queue: id=%d, time=%lu\n", id, time);
    WateringEvent we = { id, time };
    xQueueSend(wateringQueue, &we, portMAX_DELAY);
}
