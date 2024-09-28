package service

import (
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
)

func ConnectMQTT(clientId string, user string, pass string, url string) mqtt.Client {
	opts := createClientOptions(clientId, user, pass, url)
	client := mqtt.NewClient(opts)
	token := client.Connect()
	for !token.WaitTimeout(3 * time.Second) {
	}
	if err := token.Error(); err != nil {
		log.Fatal(err)
	}
	return client
}

func createClientOptions(clientId string, user string, pass string, url string) *mqtt.ClientOptions {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", url, 1883))
	opts.SetUsername(user)
	opts.SetPassword(pass)
	opts.SetClientID(clientId)
	opts.SetConnectionLostHandler(onConnectionLost)
	return opts
}

func onConnectionLost(client mqtt.Client, err error) {
	fmt.Printf("Connection lost: %v\n", err)
	retryAttempts := 3
	for i := 1; i <= retryAttempts; i++ {
		fmt.Printf("Attempting to reconnect... (%d/%d)\n", i, retryAttempts)
		time.Sleep(1 * time.Minute) // Delay between retries

		if token := client.Connect(); token.Wait() && token.Error() == nil {
			fmt.Println("Reconnected successfully")
			return // Exit after successful reconnection
		} else {
			fmt.Printf("Reconnection attempt %d failed: %v\n", i, token.Error())
		}
	}

	fmt.Println("Failed to reconnect after 3 attempts")
}
