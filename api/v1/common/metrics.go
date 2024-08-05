package common

import (
	"sync"

	"github.com/ctfer-io/chall-manager/global"
	"go.opentelemetry.io/otel/metric"
)

var (
	challengesUDCounter     metric.Int64UpDownCounter
	challengesUDCounterOnce sync.Once

	instancesUDCounter     metric.Int64UpDownCounter
	instancesUDCounterOnce sync.Once
)

func ChallengesUDCounter() metric.Int64UpDownCounter {
	challengesUDCounterOnce.Do(func() {
		cnt, err := global.Meter.Int64UpDownCounter("challenges",
			metric.WithDescription("The number of registered challenges"),
		)
		if err != nil {
			panic(err)
		}
		challengesUDCounter = cnt
	})
	return challengesUDCounter
}

func InstancesUDCounter() metric.Int64UpDownCounter {
	instancesUDCounterOnce.Do(func() {
		cnt, err := global.Meter.Int64UpDownCounter("instances",
			metric.WithDescription("The number of registered instances"),
		)
		if err != nil {
			panic(err)
		}
		instancesUDCounter = cnt
	})
	return instancesUDCounter
}
