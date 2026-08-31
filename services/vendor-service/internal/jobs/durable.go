package jobs

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDeadLetter = errors.New("jobs: worker is dead-lettered")

type DurableCoordinator struct {
	db *pgxpool.Pool
}

func NewDurableCoordinator(
	db *pgxpool.Pool,
) *DurableCoordinator {
	return &DurableCoordinator{db: db}
}

type DurableJob struct {
	Job Job

	Coordinator *DurableCoordinator

	Owner string

	LeaseFor time.Duration

	MaxAttempts int

	RetryBase time.Duration
}

type durableSettings struct {
	interval    time.Duration
	pollEvery   time.Duration
	leaseFor    time.Duration
	maxAttempts int
	retryBase   time.Duration
	owner       string
}

func (j DurableJob) settings() durableSettings {
	interval := j.Job.Interval

	if interval <= 0 {
		interval = 30 * time.Second
	}

	pollEvery := interval

	if pollEvery > 5*time.Second {
		pollEvery = 5 * time.Second
	}

	if pollEvery < time.Second {
		pollEvery = time.Second
	}

	leaseFor := j.LeaseFor

	if leaseFor <= 0 {
		leaseFor = 2 * time.Minute
	}

	maxAttempts := j.MaxAttempts

	if maxAttempts <= 0 {
		maxAttempts = 5
	}

	if maxAttempts > 25 {
		maxAttempts = 25
	}

	retryBase := j.RetryBase

	if retryBase <= 0 {
		retryBase = 5 * time.Second
	}

	owner := strings.TrimSpace(j.Owner)

	if owner == "" {
		owner = "vendor-service-worker"
	}

	return durableSettings{
		interval:    interval,
		pollEvery:   pollEvery,
		leaseFor:    leaseFor,
		maxAttempts: maxAttempts,
		retryBase:   retryBase,
		owner:       owner,
	}
}

func redactedWorkerError(
	err error,
) string {
	if err == nil {
		return ""
	}

	value :=
		strings.TrimSpace(
			strings.ReplaceAll(
				strings.ReplaceAll(
					err.Error(),
					"\r",
					" ",
				),
				"\n",
				" ",
			),
		)

	if len(value) > 1000 {
		value = value[:1000]
	}

	return value
}

func boundedRetryDelay(
	base time.Duration,
	attempt int,
) time.Duration {
	if base <= 0 {
		base = 5 * time.Second
	}

	if attempt < 1 {
		attempt = 1
	}

	delay := base

	for i := 1; i < attempt; i++ {
		delay *= 2

		if delay >= 5*time.Minute {
			return 5 * time.Minute
		}
	}

	if delay > 5*time.Minute {
		return 5 * time.Minute
	}

	return delay
}

func (c *DurableCoordinator) ensure(
	ctx context.Context,
	name string,
	maxAttempts int,
) error {
	if c == nil || c.db == nil {
		return errors.New(
			"jobs: durable coordinator database unavailable",
		)
	}

	_,
		err :=
		c.db.Exec(
			ctx,
			`INSERT INTO vendor_job_controls (
                job_name,
                status,
                next_run_at,
                max_attempts
            )
            VALUES (
                $1,
                'READY',
                now(),
                $2
            )
            ON CONFLICT (job_name)
            DO UPDATE SET
                max_attempts = EXCLUDED.max_attempts`,
			name,
			maxAttempts,
		)

	return err
}

func (c *DurableCoordinator) recoverExpiredLease(
	ctx context.Context,
	name string,
	retryBase time.Duration,
) error {
	_,
		err :=
		c.db.Exec(
			ctx,
			`UPDATE vendor_job_controls
             SET
                attempt_count = attempt_count + 1,
                status = CASE
                    WHEN attempt_count + 1 >= max_attempts
                        THEN 'DEAD_LETTER'
                    ELSE 'RETRY'
                END,
                next_run_at = CASE
                    WHEN attempt_count + 1 >= max_attempts
                        THEN next_run_at
                    ELSE now() + $2::interval
                END,
                lease_owner = NULL,
                lease_expires_at = NULL,
                heartbeat_at = NULL,
                last_error_redacted =
                    'worker lease expired before completion',
                updated_at = now()
             WHERE job_name = $1
               AND status = 'RUNNING'
               AND lease_expires_at IS NOT NULL
               AND lease_expires_at < now()`,
			name,
			retryBase.String(),
		)

	return err
}

func (c *DurableCoordinator) Claim(
	ctx context.Context,
	name string,
	owner string,
	leaseFor time.Duration,
	maxAttempts int,
	retryBase time.Duration,
) (
	bool,
	error,
) {
	name = strings.TrimSpace(name)
	owner = strings.TrimSpace(owner)

	if name == "" || owner == "" {
		return false,
			errors.New(
				"jobs: worker name and owner required",
			)
	}

	if err :=
		c.ensure(
			ctx,
			name,
			maxAttempts,
		); err != nil {

		return false, err
	}

	if err :=
		c.recoverExpiredLease(
			ctx,
			name,
			retryBase,
		); err != nil {

		return false, err
	}

	var claimed string

	err :=
		c.db.QueryRow(
			ctx,
			`UPDATE vendor_job_controls
             SET
                status = 'RUNNING',
                lease_owner = $2,
                lease_expires_at =
                    now() + $3::interval,
                heartbeat_at = now(),
                updated_at = now()
             WHERE job_name = $1
               AND status IN ('READY','RETRY')
               AND next_run_at <= now()
               AND lease_owner IS NULL
             RETURNING job_name`,
			name,
			owner,
			leaseFor.String(),
		).Scan(
			&claimed,
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	return claimed != "", nil
}

func (c *DurableCoordinator) Heartbeat(
	ctx context.Context,
	name string,
	owner string,
	leaseFor time.Duration,
) error {
	tag,
		err :=
		c.db.Exec(
			ctx,
			`UPDATE vendor_job_controls
             SET
                heartbeat_at = now(),
                lease_expires_at =
                    now() + $3::interval,
                updated_at = now()
             WHERE job_name = $1
               AND status = 'RUNNING'
               AND lease_owner = $2`,
			name,
			owner,
			leaseFor.String(),
		)

	if err != nil {
		return err
	}

	if tag.RowsAffected() != 1 {
		return errors.New(
			"jobs: worker lease no longer owned",
		)
	}

	return nil
}

func (c *DurableCoordinator) Complete(
	ctx context.Context,
	name string,
	owner string,
	interval time.Duration,
) error {
	tag,
		err :=
		c.db.Exec(
			ctx,
			`UPDATE vendor_job_controls
             SET
                status = 'READY',
                next_run_at =
                    now() + $3::interval,
                lease_owner = NULL,
                lease_expires_at = NULL,
                heartbeat_at = NULL,
                attempt_count = 0,
                last_error_redacted = '',
                updated_at = now()
             WHERE job_name = $1
               AND status = 'RUNNING'
               AND lease_owner = $2`,
			name,
			owner,
			interval.String(),
		)

	if err != nil {
		return err
	}

	if tag.RowsAffected() != 1 {
		return errors.New(
			"jobs: completion lost worker lease",
		)
	}

	return nil
}

func (c *DurableCoordinator) Fail(
	ctx context.Context,
	name string,
	owner string,
	retryBase time.Duration,
	passErr error,
) (
	bool,
	error,
) {
	var attempt int
	var maxAttempts int

	err :=
		c.db.QueryRow(
			ctx,
			`SELECT
                attempt_count + 1,
                max_attempts
             FROM vendor_job_controls
             WHERE job_name = $1
               AND status = 'RUNNING'
               AND lease_owner = $2`,
			name,
			owner,
		).Scan(
			&attempt,
			&maxAttempts,
		)

	if err != nil {
		return false, err
	}

	dead :=
		attempt >= maxAttempts

	status := "RETRY"

	if dead {
		status = "DEAD_LETTER"
	}

	delay :=
		boundedRetryDelay(
			retryBase,
			attempt,
		)

	tag,
		err :=
		c.db.Exec(
			ctx,
			`UPDATE vendor_job_controls
             SET
                status = $3,
                attempt_count = $4,
                next_run_at = CASE
                    WHEN $3 = 'DEAD_LETTER'
                        THEN next_run_at
                    ELSE now() + $5::interval
                END,
                lease_owner = NULL,
                lease_expires_at = NULL,
                heartbeat_at = NULL,
                last_error_redacted = $6,
                updated_at = now()
             WHERE job_name = $1
               AND lease_owner = $2`,
			name,
			owner,
			status,
			attempt,
			delay.String(),
			redactedWorkerError(
				passErr,
			),
		)

	if err != nil {
		return false, err
	}

	if tag.RowsAffected() != 1 {
		return false,
			errors.New(
				"jobs: failure lost worker lease",
			)
	}

	return dead, nil
}

func (c *DurableCoordinator) Replay(
	ctx context.Context,
	name string,
	reason string,
) error {
	tag,
		err :=
		c.db.Exec(
			ctx,
			`UPDATE vendor_job_controls
             SET
                status = 'READY',
                next_run_at = now(),
                lease_owner = NULL,
                lease_expires_at = NULL,
                heartbeat_at = NULL,
                attempt_count = 0,
                replay_count = replay_count + 1,
                last_error_redacted = $2,
                updated_at = now()
             WHERE job_name = $1
               AND status = 'DEAD_LETTER'`,
			strings.TrimSpace(
				name,
			),
			strings.TrimSpace(
				reason,
			),
		)

	if err != nil {
		return err
	}

	if tag.RowsAffected() != 1 {
		return errors.New(
			"jobs: worker is not dead-lettered",
		)
	}

	return nil
}

func (j DurableJob) runClaimed(
	ctx context.Context,
	settings durableSettings,
	onError func(
		string,
		error,
	),
) {
	heartbeatCtx,
		stopHeartbeat :=
		context.WithCancel(
			ctx,
		)

	var heartbeatErr error
	var heartbeatMu sync.Mutex
	var heartbeatWG sync.WaitGroup

	every :=
		settings.leaseFor / 3

	if every < time.Second {
		every = time.Second
	}

	heartbeatWG.Add(1)

	go func() {
		defer heartbeatWG.Done()

		ticker :=
			time.NewTicker(
				every,
			)

		defer ticker.Stop()

		for {
			select {
			case <-heartbeatCtx.Done():
				return

			case <-ticker.C:
				err :=
					j.Coordinator.
						Heartbeat(
							heartbeatCtx,
							j.Job.Name,
							settings.owner,
							settings.leaseFor,
						)

				if err != nil {
					heartbeatMu.Lock()
					heartbeatErr = err
					heartbeatMu.Unlock()

					return
				}
			}
		}
	}()

	passErr :=
		j.Job.Pass(
			ctx,
		)

	stopHeartbeat()
	heartbeatWG.Wait()

	heartbeatMu.Lock()

	if passErr == nil {
		passErr = heartbeatErr
	}

	heartbeatMu.Unlock()

	if passErr == nil {
		if err :=
			j.Coordinator.
				Complete(
					ctx,
					j.Job.Name,
					settings.owner,
					settings.interval,
				); err != nil &&
			onError != nil {

			onError(
				j.Job.Name,
				err,
			)
		}

		return
	}

	dead,
		failErr :=
		j.Coordinator.
			Fail(
				ctx,
				j.Job.Name,
				settings.owner,
				settings.retryBase,
				passErr,
			)

	if onError != nil {
		onError(
			j.Job.Name,
			passErr,
		)

		if failErr != nil {
			onError(
				j.Job.Name,
				failErr,
			)
		}

		if dead {
			onError(
				j.Job.Name,
				ErrDeadLetter,
			)
		}
	}
}

func (j DurableJob) runOnce(
	ctx context.Context,
	onError func(
		string,
		error,
	),
) {
	if j.Job.Pass == nil ||
		j.Coordinator == nil {

		return
	}

	settings :=
		j.settings()

	claimed,
		err :=
		j.Coordinator.
			Claim(
				ctx,
				j.Job.Name,
				settings.owner,
				settings.leaseFor,
				settings.maxAttempts,
				settings.retryBase,
			)

	if err != nil {
		if onError != nil {
			onError(
				j.Job.Name,
				err,
			)
		}

		return
	}

	if !claimed {
		return
	}

	j.runClaimed(
		ctx,
		settings,
		onError,
	)
}

func (j DurableJob) Run(
	ctx context.Context,
	onError func(
		string,
		error,
	),
) {
	settings :=
		j.settings()

	j.runOnce(
		ctx,
		onError,
	)

	ticker :=
		time.NewTicker(
			settings.pollEvery,
		)

	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			j.runOnce(
				ctx,
				onError,
			)
		}
	}
}
