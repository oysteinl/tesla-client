package service

import (
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	log "github.com/sirupsen/logrus"
	"time"
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
	return opts
}
