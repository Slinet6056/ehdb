package scheduler

import (
	"context"
	"sync"

	"github.com/robfig/cron/v3"
	"github.com/slinet/ehdb/internal/config"
	"github.com/slinet/ehdb/internal/crawler"
	"go.uber.org/zap"
)

// Scheduler manages scheduled tasks
type Scheduler struct {
	cron   *cron.Cron
	cfg    *config.Config
	logger *zap.Logger
	mu     sync.Mutex
}

// New creates a new scheduler
func New(cfg *config.Config, logger *zap.Logger) *Scheduler {
	return &Scheduler{
		cron:   cron.New(),
		cfg:    cfg,
		logger: logger,
	}
}

// Start starts the scheduler
func (s *Scheduler) Start() error {
	// Gallery sync
	if s.cfg.Scheduler.GallerySyncEnabled {
		_, err := s.cron.AddFunc(s.cfg.Scheduler.GallerySyncCron, func() {
			s.mu.Lock()
			defer s.mu.Unlock()

			s.logger.Info("starting scheduled gallery sync", zap.Int("offset", s.cfg.Scheduler.GallerySyncOffset))
			if err := s.syncGalleries(); err != nil {
				s.logger.Error("gallery sync failed", zap.Error(err))
			}
			s.logger.Info("gallery sync completed")
		})
		if err != nil {
			return err
		}
		s.logger.Info("gallery sync task registered",
			zap.String("cron", s.cfg.Scheduler.GallerySyncCron),
			zap.Int("offset", s.cfg.Scheduler.GallerySyncOffset))
	} else {
		s.logger.Info("gallery sync task is disabled")
	}

	// Torrent sync
	if s.cfg.Scheduler.TorrentSyncEnabled {
		_, err := s.cron.AddFunc(s.cfg.Scheduler.TorrentSyncCron, func() {
			s.mu.Lock()
			defer s.mu.Unlock()

			s.logger.Info("starting scheduled torrent sync")
			if err := s.syncTorrents(); err != nil {
				s.logger.Error("torrent sync failed", zap.Error(err))
			}
			s.logger.Info("torrent sync completed")
		})
		if err != nil {
			return err
		}
		s.logger.Info("torrent sync task registered", zap.String("cron", s.cfg.Scheduler.TorrentSyncCron))
	} else {
		s.logger.Info("torrent sync task is disabled")
	}

	// Resync
	if s.cfg.Scheduler.ResyncEnabled {
		_, err := s.cron.AddFunc(s.cfg.Scheduler.ResyncCron, func() {
			s.mu.Lock()
			defer s.mu.Unlock()

			s.logger.Info("starting scheduled resync", zap.Int("hours", s.cfg.Scheduler.ResyncHours))
			if err := s.resyncGalleries(); err != nil {
				s.logger.Error("resync failed", zap.Error(err))
			}
			s.logger.Info("resync completed")
		})
		if err != nil {
			return err
		}
		s.logger.Info("resync task registered",
			zap.String("cron", s.cfg.Scheduler.ResyncCron),
			zap.Int("hours", s.cfg.Scheduler.ResyncHours))
	} else {
		s.logger.Info("resync task is disabled")
	}

	s.cron.Start()
	s.logger.Info("scheduler started")

	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.cron.Stop()
	s.logger.Info("scheduler stopped")
}

// syncGalleries performs gallery synchronization
func (s *Scheduler) syncGalleries() error {
	crawlerCfg := s.cfg.Crawler
	crawlerCfg.Offset = s.cfg.Scheduler.GallerySyncOffset

	crawler, err := crawler.NewGalleryCrawler(&crawlerCfg, s.logger)
	if err != nil {
		return err
	}

	ctx := context.Background()
	return crawler.Sync(ctx)
}

// syncTorrents performs torrent synchronization
func (s *Scheduler) syncTorrents() error {
	crawler, err := crawler.NewTorrentCrawler(&s.cfg.Crawler, s.logger)
	if err != nil {
		return err
	}

	ctx := context.Background()
	return crawler.Sync(ctx)
}

// resyncGalleries performs gallery resynchronization
func (s *Scheduler) resyncGalleries() error {
	resyncer := crawler.NewResyncer(&s.cfg.Crawler, s.logger)
	ctx := context.Background()
	return resyncer.Resync(ctx, s.cfg.Scheduler.ResyncHours)
}
