package main

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/prometheus/alertmanager/notify"
	"pack.ag/amqp"
)

type amqpHandler struct{}

var i = 0

// ServeHTTP is called upon receiving an alert
// If the alert is not correct, it will reply with a Bad Request, which is a 400 error.
// 400 errors are NOT reprocessed by the alertmanager.
// If any other error occurs, it will return a 500 error, which is reprocessable by the alertmanager.
func (*amqpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	message := notify.WebhookMessage{}
	err := json.NewDecoder(r.Body).Decode(&message)
	if err != nil {
		log.Println(err)
		s := http.StatusBadRequest
		w.WriteHeader(s)
		return
	}
	if len(message.Alerts) != 1 {
		log.Error("Incorrect alerts")
		s := http.StatusBadRequest
		w.WriteHeader(s)
		return
	}

	if sender == nil {
		recoverAMQP()
	}

	if sender == nil {
		log.Error("No connection to AMQP")
		s := http.StatusServiceUnavailable
		w.WriteHeader(s)
		return
	}

	alert, err := json.Marshal(message.Alerts[0])
	if err != nil {
		log.Error("Can not marshal alert: ", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	msg := amqp.NewMessage(alert)
	msg.Header = &amqp.MessageHeader{TTL: 365 * 24 * time.Hour, Durable: true}
	err = sender.Send(ctx, msg)
	if err != nil {
		recoverAMQP()
		err = sender.Send(ctx, msg)
		if err != nil {
			log.Error("Sending message:", err)
			s := http.StatusServiceUnavailable
			w.WriteHeader(s)
			return
		}
	}

	return
}

func init() {
	ctx = context.Background()
}
