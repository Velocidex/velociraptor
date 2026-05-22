package sigma

import (
	"context"
	"sync"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/utils"
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
	log_ctx     *SigmaExecutionContext
	event       *evaluator.Event
	rules       []*evaluator.VQLRuleEvaluator
	scope       vfilter.Scope
	ctx         context.Context
	wg          *sync.WaitGroup
	debug       bool
}

// Run a single pass over the ruleset.
func (self *workerJob) Run() {
	defer self.wg.Done()

	start := utils.GetTime().Now().UnixNano()
	defer func() {
		self.log_ctx.ChargeTime(utils.GetTime().Now().UnixNano() - start)
	}()

	// Create a subscope for the entire evaluation chain.
	vars := ordereddict.NewDict().Set("Event", self.event)
	subscope := self.scope.Copy().AppendVars(vars)
	defer subscope.Close()

	for _, rule := range self.rules {
		vars.Update("Rule", rule.Rule)

		// Makes a copy of the event if it is changed.
		event := rule.MaybeEnrichWithVQL(self.ctx, subscope, self.event)
		match, err := rule.Match(self.ctx, subscope, event)
		if err != nil {
			functions.DeduplicatedLog(self.ctx, subscope,
				"While evaluating rule %v: %v", rule.Title, err)
			continue
		}

		if !self.debug && !match.Match {
			continue
		}

		// If this source rule feeds one or more correlations, dispatch
		// the matching event to each correlator and emit one row per
		// fired correlation.
		if len(rule.Correlators) > 0 {
			for _, correlator := range rule.Correlators {
				// Only feed the correlator when the base rule actually
				// matched. Otherwise (in debug mode) we would pollute
				// the timespan state with non matching events.
				corr_match := match
				if match.Match {
					corr_match, err = correlator.Match(
						self.ctx, subscope, rule, event)
					if err != nil {
						functions.DeduplicatedLog(self.ctx, subscope,
							"While evaluating correlation %v: %v",
							correlator.Title, err)
						continue
					}
				}

				if !self.debug && !corr_match.Match {
					continue
				}

				// Report the correlation rule as the hit.
				if !self.emit(subscope, event, corr_match,
					correlator.VQLRuleEvaluator) {
					return
				}
			}
			continue
		}

		if !self.emit(subscope, event, match, rule) {
			return
		}
	}
}

// emit writes a single matching row to the output channel. It operates
// on a copy of the event so as not to interfere with other threads.
// Returns false if the context was cancelled.
func (self *workerJob) emit(
	subscope vfilter.Scope,
	event *evaluator.Event,
	match *evaluator.Result,
	rule *evaluator.VQLRuleEvaluator) bool {

	event_copy := evaluator.NewEvent(event.Copy())
	if match.CorrelationHits == nil {
		event_copy.Set("_Match", match)
	} else {
		event_copy.Set("_Correlations", match.CorrelationHits)
	}

	event_copy.Set("_Rule", rule)

	self.sigma_context.AddDetail(self.ctx, subscope, event_copy, rule)
	rule.MaybeEnrichForReporting(self.ctx, subscope, event_copy)

	self.sigma_context.IncHitCount()

	select {
	case <-self.ctx.Done():
		return false

	case self.output_chan <- event_copy:
		return true
	}
}

type workerPool struct {
	sigma_context *SigmaContext
	output_chan   chan types.Row

	in_chan chan *workerJob

	wg    *sync.WaitGroup
	debug bool
}

func (self *workerPool) RunInline(
	log_ctx *SigmaExecutionContext,
	ctx context.Context,
	scope vfilter.Scope,
	event *evaluator.Event,
	rules []*evaluator.VQLRuleEvaluator) {

	job := &workerJob{
		sigma_context: self.sigma_context,
		output_chan:   self.output_chan,
		log_ctx:       log_ctx,
		event:         event,
		rules:         rules,
		scope:         scope,
		ctx:           ctx,
		wg:            self.wg,
		debug:         self.debug,
	}

	self.wg.Add(1)
	job.Run()
}

func (self *workerPool) Run(
	log_ctx *SigmaExecutionContext,
	ctx context.Context,
	scope vfilter.Scope,
	event *evaluator.Event,
	rules []*evaluator.VQLRuleEvaluator) {

	job := &workerJob{
		sigma_context: self.sigma_context,
		output_chan:   self.output_chan,
		log_ctx:       log_ctx,
		event:         event,
		rules:         rules,
		scope:         scope,
		ctx:           ctx,
		wg:            self.wg,
		debug:         self.debug,
	}

	// Will be cleared when the job is done.
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
