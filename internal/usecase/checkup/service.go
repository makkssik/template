package checkup

import (
	"context"
	"fmt"
	"time"
)

type Service struct {
	database     databasePinger
	queryTimeout time.Duration
}

type ServiceCfg struct {
	Database     databasePinger
	QueryTimeout time.Duration
}

func NewService(cfg ServiceCfg) *Service {
	return &Service{
		database:     cfg.Database,
		queryTimeout: cfg.QueryTimeout,
	}
}

func (s *Service) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()

	if err := s.database.Ping(ctx); err != nil {
		return fmt.Errorf("check database: %w", err)
	}
	return nil
}
