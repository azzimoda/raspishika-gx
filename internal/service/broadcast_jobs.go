package service

import (
	"context"
	"errors"
	"time"

	"github.com/azzimoda/raspishika-gx/internal/model"
	"github.com/azzimoda/raspishika-gx/internal/repository"
	"github.com/rs/zerolog/log"
)

// initialBroadcastPlatforms is the set of platforms a manual broadcast is
// enqueued for. The VK platform joins as soon as the VK bot lands (Phase 2d).
var initialBroadcastPlatforms = []model.Platform{model.PlatformTelegram}

// MassBroadcastEnqueuer records a manual broadcast for the schedule-bot
// workers instead of sending it directly.
type MassBroadcastEnqueuer interface {
	EnqueueMassBroadcast(ctx context.Context, audience string, spec string, htmlText string, createdBy int64) error
}

// BroadcastJobs enqueues manual mass broadcasts into broadcast_jobs, one row
// per target platform, and reads their status for the admin audit.
type BroadcastJobs struct {
	repo      repository.BroadcastJobRepository
	platforms []model.Platform
}

func NewBroadcastJobs(repo repository.BroadcastJobRepository, platforms []model.Platform) *BroadcastJobs {
	if len(platforms) == 0 {
		platforms = initialBroadcastPlatforms
	}
	return &BroadcastJobs{repo: repo, platforms: platforms}
}

func (s *BroadcastJobs) EnqueueMassBroadcast(ctx context.Context, audience string, spec string, htmlText string, createdBy int64) error {
	var specPtr *string
	if spec != "" {
		specPtr = &spec
	}
	var enqueueErr error
	for _, platform := range s.platforms {
		job := &model.BroadcastJob{
			Audience:  audience,
			Platform:  platform,
			Spec:      specPtr,
			HTML:      htmlText,
			Status:    model.JobPending,
			CreatedBy: createdBy,
		}
		if err := s.repo.Enqueue(ctx, job); err != nil {
			enqueueErr = errors.Join(enqueueErr, err)
		}
	}
	return enqueueErr
}

// Recent returns the newest broadcast jobs across all platforms.
func (s *BroadcastJobs) Recent(ctx context.Context, limit int) ([]*model.BroadcastJob, error) {
	return s.repo.Recent(ctx, limit)
}

// BroadcastJobPoller is the per-platform worker claimed by a schedule-bot
// process. It polls broadcast_jobs for its platform and executes every claim
// synchronously, so a job is finished only after its delivery pass ends.
type BroadcastJobPoller struct {
	bc       *BroadcastService
	repo     repository.BroadcastJobRepository
	platform model.Platform
}

func NewBroadcastJobPoller(bc *BroadcastService, repo repository.BroadcastJobRepository, platform model.Platform) *BroadcastJobPoller {
	return &BroadcastJobPoller{bc: bc, repo: repo, platform: platform}
}

// pollInterval bounds how long a worker waits before re-checking the queue.
const pollInterval = 5 * time.Second

func (p *BroadcastJobPoller) Run(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		p.pollPending(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// pollPending claims and processes every pending job for the platform. It
// stops when the queue is empty or on the first claim error.
func (p *BroadcastJobPoller) pollPending(ctx context.Context) {
	for ctx.Err() == nil {
		job, err := p.repo.ClaimNext(ctx, p.platform)
		if err != nil {
			log.Error().Err(err).Msg("Failed to claim broadcast job")
			p.bc.reportError(err, "Failed to claim broadcast job")
			return
		}
		if job == nil {
			return
		}
		log.Info().Int64("job", job.ID).Str("platform", string(p.platform)).Msg("Claimed broadcast job")
		p.process(ctx, job)
	}
}

// process resolves the audience fresh and sends the job content to every
// recipient, then records the terminal job state.
func (p *BroadcastJobPoller) process(ctx context.Context, job *model.BroadcastJob) {
	if p.bc == nil || p.bc.Chat == nil || p.bc.Stats == nil {
		p.fail(ctx, job, errors.New("broadcast service is not initialized"))
		return
	}
	chats, err := p.bc.Chat.ResolveAudience(ctx, job.Audience, job.Spec)
	if err != nil {
		p.fail(ctx, job, err)
		return
	}
	if len(chats) == 0 {
		job.Status = model.JobDone
		if err := p.repo.Finish(ctx, job); err != nil {
			p.bc.reportError(err, "Failed to finish empty broadcast job")
		}
		return
	}
	if err := p.bc.SendMassText(ctx, chats, job.HTML); err != nil {
		p.fail(ctx, job, err)
		return
	}
	job.Status = model.JobDone
	if err := p.repo.Finish(ctx, job); err != nil {
		p.bc.reportError(err, "Failed to finish broadcast job")
	}
}

func (p *BroadcastJobPoller) fail(ctx context.Context, job *model.BroadcastJob, jobErr error) {
	job.Status = model.JobFailed
	if jobErr != nil {
		value := jobErr.Error()
		job.Error = &value
	}
	if err := p.repo.Finish(ctx, job); err != nil {
		log.Error().Err(err).Int64("job", job.ID).Msg("Failed to mark broadcast job failed")
	}
}
