package stats

import (
	"sync/atomic"
	"time"
)

type Stats struct {
	InFlight            atomic.Int64
	JobsTotal           atomic.Int64
	JobsFailedInternal  atomic.Int64
	lastInternalErrAt   atomic.Int64 // unix nano, 0 = never
}

func (s *Stats) RecordInternalError() {
	s.JobsFailedInternal.Add(1)
	s.lastInternalErrAt.Store(time.Now().UnixNano())
}

// LastInternalErrorAt returns nil if there has never been an error.
func (s *Stats) LastInternalErrorAt() *time.Time {
	ns := s.lastInternalErrAt.Load()
	if ns == 0 {
		return nil
	}
	t := time.Unix(0, ns).UTC()
	return &t
}