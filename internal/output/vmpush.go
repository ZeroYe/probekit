package output

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ZeroYe/probekit/internal/config"
	"github.com/ZeroYe/probekit/internal/metrics"
	"github.com/ZeroYe/probekit/internal/selfmetrics"
	"go.uber.org/zap"
)

type VMPusher struct {
	cfg     config.VMConfig
	client  *http.Client
	buffer  *RingBuffer
	batcher *Batcher
	logger  *zap.Logger
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
}

func NewVMPusher(cfg config.VMConfig, logger *zap.Logger) *VMPusher {
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:    10,
			IdleConnTimeout: 90 * time.Second,
		},
	}

	return &VMPusher{
		cfg:     cfg,
		client:  client,
		buffer:  NewRingBuffer(cfg.BufferSize),
		batcher: NewBatcher(cfg.BatchSize, cfg.FlushInterval),
		logger:  logger.Named("vmpush"),
		done:    make(chan struct{}),
	}
}

func (v *VMPusher) Name() string {
	return "victoria_metrics"
}

func (v *VMPusher) Start() error {
	v.ctx, v.cancel = context.WithCancel(context.Background())
	go v.flushLoop()
	v.logger.Info("vm pusher started",
		zap.String("push_url", v.cfg.PushURL),
		zap.Duration("flush_interval", v.cfg.FlushInterval),
		zap.Int("batch_size", v.cfg.BatchSize),
	)
	return nil
}

func (v *VMPusher) Stop() error {
	if v.cancel != nil {
		v.cancel()
	}
	<-v.done

	if data := v.batcher.Flush(); data != "" {
		v.push(data)
	}
	return nil
}

func (v *VMPusher) Write(ms []metrics.Metric) error {
	v.buffer.PushBatch(ms)
	return nil
}

func (v *VMPusher) flushLoop() {
	ticker := time.NewTicker(v.cfg.FlushInterval)
	defer ticker.Stop()
	defer close(v.done)

	for {
		select {
		case <-v.ctx.Done():
			return
		case <-ticker.C:
			v.flush()
		}
	}
}

func (v *VMPusher) flush() {
	metrics := v.buffer.PopN(v.cfg.BatchSize)
	if len(metrics) == 0 {
		return
	}

	selfmetrics.QueueLength.Store(int64(v.buffer.Len()))

	data := v.batcher.Add(metrics)
	if data == "" {
		return
	}

	v.push(data)
}

func (v *VMPusher) push(data string) {
	for attempt := 0; attempt <= v.cfg.Retry.MaxRetries; attempt++ {
		if err := v.pushOnce(data); err != nil {
			v.logger.Error("push failed",
				zap.Int("attempt", attempt+1),
				zap.Error(err),
			)
			if attempt < v.cfg.Retry.MaxRetries {
				backoff := v.cfg.Retry.InitialBackoff * time.Duration(1<<uint(attempt))
				if backoff > v.cfg.Retry.MaxBackoff {
					backoff = v.cfg.Retry.MaxBackoff
				}
				select {
				case <-v.ctx.Done():
					return
				case <-time.After(backoff):
				}
			}
			return
		}
		return
	}
}

func (v *VMPusher) pushOnce(data string) error {
	req, err := http.NewRequestWithContext(v.ctx, http.MethodPost, v.cfg.PushURL, bytes.NewBufferString(data))
	if err != nil {
		selfmetrics.PushErrors.Add(1)
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain; version=0.0.4")

	resp, err := v.client.Do(req)
	if err != nil {
		selfmetrics.PushErrors.Add(1)
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(resp.Body)
		selfmetrics.PushErrors.Add(1)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	selfmetrics.MetricsPushed.Add(int64(strings.Count(data, "\n")))
	return nil
}
