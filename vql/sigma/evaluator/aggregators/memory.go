package aggregators

import (
	"context"
	"sync"
	"time"

	"github.com/bradleyjkemp/sigma-go/evaluator"
	"github.com/bradleyjkemp/sigma-go/internal/slidingstatistics"
)

type inMemory struct {
	sync.Mutex
	timeframe time.Duration
	counts    map[string]*slidingstatistics.Counter
	averages  map[string]*slidingstatistics.Averager
	sums      map[string]*slidingstatistics.Counter
}

func (i *inMemory) count(ctx context.Context, groupBy evaluator.GroupedByValues) (float64, error) {
	i.Lock()
	defer i.Unlock()
	c, ok := i.counts[groupBy.Key()]
	if !ok {
		c = slidingstatistics.Count(i.timeframe)
		i.counts[groupBy.Key()] = c
	}

	return float64(c.IncrementN(time.Now(), 1)), nil
}

func (i *inMemory) average(ctx context.Context, groupBy evaluator.GroupedByValues, value float64) (float64, error) {
	i.Lock()
	defer i.Unlock()
	a, ok := i.averages[groupBy.Key()]
	if !ok {
		a = slidingstatistics.Average(i.timeframe)
		i.averages[groupBy.Key()] = a
	}

	return a.Average(time.Now(), value), nil
}

func (i *inMemory) sum(ctx context.Context, groupBy evaluator.GroupedByValues, value float64) (float64, error) {
	i.Lock()
	defer i.Unlock()
	a, ok := i.sums[groupBy.Key()]
	if !ok {
		a = slidingstatistics.Count(i.timeframe)
		i.sums[groupBy.Key()] = a
	}

	return a.IncrementN(time.Now(), value), nil
}

func InMemory(timeframe time.Duration) []evaluator.Option {
	i := &inMemory{
		timeframe: timeframe,
		counts:    map[string]*slidingstatistics.Counter{},
	}

	return []evaluator.Option{
		evaluator.CountImplementation(i.count),
		evaluator.SumImplementation(i.sum),
		evaluator.AverageImplementation(i.average),
	}
}
