package main

// This is a proxy that sits between an MQTT broker and the grocy web interface.
// It is required because of esphome's inability to handle chunked HTTP responses.
// The ESP32 publishes messages to <prefix>/heartbeat and <prefix>/scan.
// The heartbeat messages are ISO8601 timestamps.
// The scan messages are in the format <action>:<barcode>:<count>, where action is either
// "add", "consume", or "inventory". The count how many to add, consume, or set the inventory to.

// The POST requests to grocy are made to <hostname>/api/stock/products/by-barcode/<barcode>/<action>.
// The payload of these POST requests is a json string like '{"amount": <count>}', or '{"new_amount": <count>}' for the inventory action.
// The requests have Content-Type: application/json and GROCY-API-KEY headers.
// After POSTing to grocy, this proxy then sends a GET request to <hostname>/api/stock/products/by-barcode/<barcode> to get the new inventory count, and then publishes a message to <prefix>/inventory/<barcode> with the new count.
// It publishes this count to <prefix>/quantity as a raw number.
// If the POST or GET request fails, the proxy publishes either "Net err" or "Bar err" to <prefix>/error depending on the type of error.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const (
	prefix = "72mad/barcode_scanner"
)

var (
	mqttBroker   = os.Getenv("MQTT_BROKER")
	mqttUsername = os.Getenv("MQTT_USERNAME")
	mqttPassword = os.Getenv("MQTT_PASSWORD")
	grocyHost    = os.Getenv("GROCY_HOST")
	grocyApiKey  = os.Getenv("GROCY_API_KEY")
	mqttClientId = "grocy-proxy"
)

func handleScanMessage(client mqtt.Client, msg mqtt.Message) {
	payload := string(msg.Payload())
	parts := strings.Split(payload, "|")
	if len(parts) != 3 {
		fmt.Printf("Invalid scan message: %s\n", payload)
		return
	}
	action, barcode, count := parts[0], parts[1], parts[2]
	var apiAction string
	switch action {
	case "add":
		apiAction = "add"
	case "consume":
		apiAction = "consume"
	case "inventory":
		apiAction = "inventory"
	default:
		fmt.Printf("Invalid action: %s\n", action)
		return
	}

	url := fmt.Sprintf("%s/api/stock/products/by-barcode/%s/%s", grocyHost, barcode, apiAction)
	var jsonData []byte
	if apiAction == "inventory" {
		jsonData, _ = json.Marshal(map[string]string{"new_amount": count})
	} else {
		jsonData, _ = json.Marshal(map[string]string{"amount": count})
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		client.Publish(prefix+"/error", 0, false, "Net err")
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("GROCY-API-KEY", grocyApiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		client.Publish(prefix+"/error", 0, false, "Net err")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// We only care about this error if "No product" is in the payload of the response. Otherwise ignore it
		body, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(body), "No product") {
			client.Publish(prefix+"/error", 2, false, "Bar err")
			return
		}
	}

	// Get the new inventory count
	getUrl := fmt.Sprintf("%s/api/stock/products/by-barcode/%s", grocyHost, barcode)
	getReq, err := http.NewRequest("GET", getUrl, nil)
	if err != nil {
		client.Publish(prefix+"/error", 0, false, "Net err")
		return
	}
	getReq.Header.Set("GROCY-API-KEY", grocyApiKey)

	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		client.Publish(prefix+"/error", 0, false, "Net err")
		return
	}
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusOK {
		client.Publish(prefix+"/error", 2, false, "Net err")
		return
	}
	body, err := io.ReadAll(getResp.Body)
	if err != nil {
		client.Publish(prefix+"/error", 2, false, "Net err")
		return
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		client.Publish(prefix+"/error", 2, false, "Net err")
		return
	}
	fmt.Printf("New inventory for %s: %v\n", barcode, result["stock_amount"])
	if newAmount, ok := result["stock_amount"].(float64); ok {
		client.Publish(prefix+"/quantity", 2, false, fmt.Sprintf("%d", int(newAmount)))
	} else {
		fmt.Printf("Invalid stock_amount: %v\n", result["stock_amount"])
		client.Publish(prefix+"/error", 2, false, "Bar err")
	}
}

func onConnectHandler(client mqtt.Client) {
	fmt.Printf("Connected to MQTT broker %s\n", mqttBroker)
	client.Subscribe(prefix+"/scan", 0, handleScanMessage)
}

func main() {
	opts := mqtt.NewClientOptions().AddBroker(mqttBroker).SetClientID(mqttClientId)
	if mqttUsername != "" && mqttPassword != "" {
		opts.SetUsername(mqttUsername)
		opts.SetPassword(mqttPassword)
	}
	opts.SetAutoReconnect(true)
	opts.SetOnConnectHandler(onConnectHandler)
	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}
	select {}
}
