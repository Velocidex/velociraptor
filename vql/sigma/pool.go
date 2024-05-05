package sigma

import (
	"context"
	"sync"

	"www.velocidex.com/golang/velociraptor/vql/functions"
	"www.velocidex.com/golang/velociraptor/vql/sigma/evaluator"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

const (
	WORKER_COUNT = 50
)

type workerJob struct {
	sigma_context *SigmaContext

	output_chan chan types.Row
	event       *evaluator.Event
	rules       []*evaluator.VQLRuleEvaluator
	scope       vfilter.Scope
	ctx         context.Context
	wg          *sync.WaitGroup
	debug       bool
}

func (self *workerJob) Run() {
	defer self.wg.Done()

	for _, rule := range self.rules {
		event := rule.MaybeEnrichWithVQL(self.ctx, self.scope, self.event)
		match, err := rule.Match(self.ctx, self.scope, event)
		if err != nil {
			functions.DeduplicatedLog(self.ctx, self.scope,
				"While evaluating rule %v: %v", rule.Title, err)
			continue
		}

		if !self.debug && !match.Match {
			continue
		}

		// Make a copy here because another thread might match at the same
		// time.
		event_copy := self.sigma_context.AddDetail(
			self.ctx, self.scope, event, rule)
		event_copy.Set("_Match", match).
			Set("_Rule", rule)

		self.sigma_context.IncHitCount()

		select {
		case <-self.ctx.Done():
			return

		case self.output_chan <- event_copy:
		}
	}
}

type workerPool struct {
	sigma_context *SigmaContext
	output_chan   chan types.Row

	in_chan chan *workerJob

	wg    *sync.WaitGroup
	debug bool
}

func (self *workerPool) Run(
	ctx context.Context,
	scope vfilter.Scope,
	event *evaluator.Event,
	rules []*evaluator.VQLRuleEvaluator) {
	job := &workerJob{
		sigma_context: self.sigma_context,
		output_chan:   self.output_chan,
		event:         event,
		rules:         rules,
		scope:         scope,
		ctx:           ctx,
		wg:            self.wg,
		debug:         self.debug,
	}

	// Will be cleared whent the job is done.
	self.wg.Add(1)
	select {
	case <-ctx.Done():
		return

	case self.in_chan <- job:
	}
}

func NewWorkerPool(
	ctx context.Context,
	wg *sync.WaitGroup,
	sigma_context *SigmaContext,
	output_chan chan types.Row) *workerPool {

	in_chan := make(chan *workerJob)

	for i := 0; i < WORKER_COUNT; i++ {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return

				case job, ok := <-in_chan:
					if !ok {
						return
					}
					job.Run()
				}
			}
		}()
	}

	return &workerPool{
		sigma_context: sigma_context,
		debug:         sigma_context.debug,
		output_chan:   output_chan,
		wg:            wg,
		in_chan:       in_chan,
	}
}
