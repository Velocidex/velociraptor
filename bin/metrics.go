package main

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	start = time.Now()

	CurrentTime = promauto.NewUntypedFunc(
		prometheus.UntypedOpts{
			Name: "uptime",
			Help: "Time since process start.",
		}, func() float64 {
			return float64(time.Now().Unix() - start.Unix())
		})
)
