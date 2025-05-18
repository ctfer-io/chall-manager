package common

import (
	"sync"

	"github.com/ctfer-io/chall-manager/global"
	"go.opentelemetry.io/otel/attribute"
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

func InstanceAttrs(challID, sourceID string, pool bool) attribute.Set {
	attrs := []attribute.KeyValue{
		attribute.String("challenge", challID),
		attribute.Bool("pool", pool),
	}
	if sourceID != "" {
		attrs = append(attrs, attribute.String("source", sourceID))
	}

	return attribute.NewSet(attrs...)
}
