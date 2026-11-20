package mbimport

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const (
	// lease is how long a job is the worker's before another may take it;
	// every saved batch renews it.
	lease = 2 * time.Minute
	// retryAfter is when a job that failed for a passing reason, such as
	// Moneybird being down, runs again. It goes on where it was.
	retryAfter = time.Minute
	// maxAttempts stops a job that keeps failing.
	maxAttempts = 5
)

// Run works on due imports every interval until ctx ends.
func (s *Service) Run(ctx context.Context, interval time.Duration, onError func(error)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if err := s.Work(ctx); err != nil && onError != nil {
			onError(err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Work runs the imports that are due, one at a time.
func (s *Service) Work(ctx context.Context) error {
	now := s.now()
	jobs, err := s.store.LeaseDueJobs(ctx, now, now.Add(lease), 1)
	if err != nil {
		return fmt.Errorf("lease imports: %w", err)
	}
	for _, j := range jobs {
		s.process(ctx, j)
	}
	return nil
}

// process works through a job's steps, saving after every batch, and ends
// it as done, failed, or due again for a passing failure.
func (s *Service) process(ctx context.Context, j Job) {
	err := s.steps(ctx, &j)
	now := s.now()
	j.WaitingUntil, j.UpdatedAt = nil, now
	switch {
	case err == nil:
		j.State, j.Error, j.FinishedAt, j.NextAttemptAt = Done, "", &now, nil
	case errors.Is(err, context.Canceled):
		// Stopping, such as at a restart: the job carries on next time.
		next := now
		j.NextAttemptAt = &next
	case errors.Is(err, ErrTokenRejected) || j.Attempts+1 >= maxAttempts:
		j.State, j.Error, j.FinishedAt, j.NextAttemptAt = Failed, err.Error(), &now, nil
	default:
		j.Attempts++
		j.Error = err.Error()
		next := now.Add(retryAfter)
		j.NextAttemptAt = &next
	}
	if j.State != Running {
		// The token is only kept while the job may still use it.
		j.Token = ""
	}
	// A cancelled ctx must not stop the job's state from being saved.
	_ = s.store.SaveJob(context.WithoutCancel(ctx), j)
}

func (s *Service) steps(ctx context.Context, j *Job) error {
	token, err := s.secrets.Open(j.Token)
	if err != nil {
		return fmt.Errorf("open token: %w", err)
	}
	r := &run{s: s, job: j}
	r.save = func(ctx context.Context) error {
		now := s.now()
		next := now.Add(lease)
		j.UpdatedAt, j.NextAttemptAt = now, &next
		return s.store.SaveJob(ctx, *j)
	}
	r.read = s.source.Open(token, func(until time.Time) {
		// Shown while the import waits for Moneybird's rate limit.
		at := until
		j.WaitingUntil = &at
		_ = r.save(ctx)
	})
	for _, kind := range Steps {
		st := j.Step(kind)
		if st.Done {
			continue
		}
		// A step that ran before starts its counts again; what it imported is skipped.
		*st = Step{Kind: kind, Problems: []string{}}
		if err := r.step(ctx, kind); err != nil {
			return fmt.Errorf("%s: %w", kind, err)
		}
		st = j.Step(kind)
		st.Done = true
		j.WaitingUntil = nil
		if err := r.save(ctx); err != nil {
			return err
		}
	}
	return nil
}
