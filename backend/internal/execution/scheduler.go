package execution

import (
	"context"
	"encoding/json"
	"log"
	"time"
)

// Scheduler is deliberately provider- and ACP-blind. It turns due Trigger
// rows into Service.RunNow calls; the registered Executor owns runtime work.
type Scheduler struct {
	service *Service
	ticker  *time.Ticker
}

func NewScheduler(service *Service) *Scheduler { return &Scheduler{service: service} }

func (s *Scheduler) Start(ctx context.Context) {
	s.ticker = time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case <-ctx.Done():
				s.ticker.Stop()
				return
			case <-s.ticker.C:
				s.Tick(ctx)
			}
		}
	}()
}

func (s *Scheduler) Tick(ctx context.Context) {
	if s == nil || s.service == nil {
		return
	}
	now := time.Now().UTC()
	triggers, err := s.service.repo.DueTriggers(now)
	if err != nil {
		log.Printf("[execution] list due triggers: %v", err)
		return
	}
	for _, trigger := range triggers {
		if err := s.service.RunNow(ctx, trigger.JobID); err != nil {
			continue
		}
		if trigger.Kind == TriggerAt {
			if err := s.service.repo.AdvanceTrigger(trigger.ID, TriggerExhausted, nil); err != nil {
				log.Printf("[execution] exhaust trigger %s: %v", trigger.ID, err)
			}
			continue
		}
		var recurrence struct {
			EveryMinutes int `json:"everyMinutes"`
		}
		if json.Unmarshal(trigger.Spec, &recurrence) != nil || recurrence.EveryMinutes < 1 {
			_ = s.service.repo.AdvanceTrigger(trigger.ID, TriggerPaused, nil)
			continue
		}
		next := now.Add(time.Duration(recurrence.EveryMinutes) * time.Minute)
		if err := s.service.repo.AdvanceTrigger(trigger.ID, TriggerArmed, &next); err != nil {
			log.Printf("[execution] advance trigger %s: %v", trigger.ID, err)
		}
	}
}
