void setupMQTT() {
    // Connect to MQTT
    client.setServer(MQTT_ADDRESS, MQTT_PORT);
    client.setCallback(processIncomingMessage);

    // Initialize publisher Queue
    waterPublisherQueue = xQueueCreate(QUEUE_SIZE, sizeof(WateringEvent));
    if (waterPublisherQueue == NULL) {
        printf("error creating the waterPublisherQueue\n");
    }

    // Start MQTT tasks
    xTaskCreate(mqttConnectTask, "MQTTConnectTask", 2048, NULL, 1, &mqttConnectTaskHandle);
    xTaskCreate(mqttLoopTask, "MQTTLoopTask", 4096, NULL, 1, &mqttLoopTaskHandle);
    xTaskCreate(waterPublisherTask, "WaterPublisherTask", 2048, NULL, 1, &waterPublisherTaskHandle);
#ifdef LIGHT_PIN
    lightPublisherQueue = xQueueCreate(QUEUE_SIZE, sizeof(LightingEvent));
    if (lightPublisherQueue == NULL) {
        printf("error creating the lightPublisherQueue\n");
    }
    xTaskCreate(lightPublisherTask, "LightPublisherTask", 2048, NULL, 1, &lightPublisherTaskHandle);
#endif
#ifdef ENABLE_MQTT_HEALTH
    xTaskCreate(healthPublisherTask, "HealthPublisherTask", 2048, NULL, 1, &healthPublisherTaskHandle);
#endif
}

void setupWifi() {
    delay(10);
    printf("Connecting to "SSID" as "GARDEN_NAME"-controller\n");

    WiFi.setHostname(GARDEN_NAME"-controller");

    WiFi.begin(SSID, PASSWORD);

    while (WiFi.status() != WL_CONNECTED) {
        delay(500);
        printf(".");
    }

    printf("Wifi connected...\n");
}

/*
  waterPublisherTask reads from a queue to publish WateringEvents as an InfluxDB
  line protocol message to MQTT
*/
void waterPublisherTask(void* parameters) {
    WateringEvent we;
    while (true) {
        if (xQueueReceive(waterPublisherQueue, &we, portMAX_DELAY)) {
            char message[50];
            sprintf(message, "water,plant=%d millis=%lu", we.plant_position, we.duration);
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

#ifdef LIGHT_PIN
/*
  lightPublisherTask reads from a queue to publish LightingEvents as an InfluxDB
  line protocol message to MQTT
*/
void lightPublisherTask(void* parameters) {
    int state;
    while (true) {
        if (xQueueReceive(lightPublisherQueue, &state, portMAX_DELAY)) {
            char message[50];
            sprintf(message, "light,garden=\"%s\" state=%d", GARDEN_NAME, state);
            if (client.connected()) {
                printf("publishing to MQTT:\n\ttopic=%s\n\tmessage=%s\n", lightDataTopic, message);
                client.publish(lightDataTopic, message);
            } else {
                printf("unable to publish: not connected to MQTT broker\n");
            }
        }
        vTaskDelay(5 / portTICK_PERIOD_MS);
    }
    vTaskDelete(NULL);
}
#endif

#ifdef ENABLE_MQTT_HEALTH
/*
  healthPublisherTask runs every minute and publishes a message to MQTT to record a health check-in
*/
void healthPublisherTask(void* parameters) {
    WateringEvent we;
    while (true) {
        char message[50];
        sprintf(message, "health garden=\"%s\"", GARDEN_NAME);
        if (client.connected()) {
            printf("publishing to MQTT:\n\ttopic=%s\n\tmessage=%s\n", healthDataTopic, message);
            client.publish(healthDataTopic, message);
        } else {
            printf("unable to publish: not connected to MQTT broker\n");
        }
        vTaskDelay(HEALTH_PUBLISH_INTERVAL / portTICK_PERIOD_MS);
    }
    vTaskDelete(NULL);
}
#endif

/*
  mqttConnectTask will periodically attempt to reconnect to MQTT if needed
*/
void mqttConnectTask(void* parameters) {
    while (true) {
        // Connect to MQTT server if not connected already
        if (!client.connected()) {
            printf("attempting MQTT connection...");
            // Connect with defaul arguments + cleanSession = false for persistent sessions
            if (client.connect(MQTT_CLIENT_NAME, NULL, NULL, 0, 0, 0, 0, false)) {
                printf("connected\n");
#ifndef DISABLE_WATERING
                client.subscribe(waterCommandTopic, 1);
                client.subscribe(stopCommandTopic, 1);
                client.subscribe(stopAllCommandTopic, 1);
#endif
#ifdef LIGHT_PIN
                client.subscribe(lightCommandTopic, 1);
#endif
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
  processIncomingMessage is a callback function for the MQTT client that will
  react to incoming messages. Currently, the topics are:
    - waterCommandTopic: accepts a WateringEvent JSON to water a plant for
                         specified time
    - stopCommandTopic: ignores message and stops the currently-watering plant
    - stopAllCommandTopic: ignores message, stops the currently-watering plant,
                           and clears the wateringQueue
    - lightCommandTopic: accepts LightingEvent JSON to control a grow light
*/
void processIncomingMessage(char* topic, byte* message, unsigned int length) {
    printf("message received:\n\ttopic=%s\n\tmessage=%s\n", topic, (char*)message);

    StaticJsonDocument<JSON_CAPACITY> doc;
    DeserializationError err = deserializeJson(doc, message);
    if (err) {
        printf("deserialize failed: %s\n", err.c_str());
    }

    if (strcmp(topic, waterCommandTopic) == 0) {
        WateringEvent we = {
            doc["plant_position"] | -1,
            doc["duration"] | 0,
            doc["id"] | "N/A"
        };
        printf("received command to water plant %d (%s) for %lu\n", we.plant_position, we.id, we.duration);
        waterPlant(we);
    } else if (strcmp(topic, stopCommandTopic) == 0) {
        printf("received command to stop watering\n");
        stopWatering();
    } else if (strcmp(topic, stopAllCommandTopic) == 0) {
        printf("received command to stop ALL watering\n");
        stopAllWatering();
    } else if (strcmp(topic, lightCommandTopic) == 0) {
        LightingEvent le = {
            doc["state"] | ""
        };
#ifdef LIGHT_PIN
        printf("received command to change state of the light: '%s'\n", le.state);
        changeLight(le);
#endif
    }
}
