// © 2022 Nokia.
//
// This code is a Contribution to the gNMIc project (“Work”) made under the Google Software Grant and Corporate Contributor License Agreement (“CLA”) and governed by the Apache License 2.0.
// No other rights or licenses in or to any of Nokia’s intellectual property are granted for any other purpose.
// This code is provided on an “as is” basis without any warranties of any kind.
//
// SPDX-License-Identifier: Apache-2.0

package influxdb_output

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"strings"
	"text/template"
	"time"

	"google.golang.org/protobuf/proto"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/openconfig/gnmi/proto/gnmi"

	"github.com/openconfig/gnmic/pkg/api/types"
	"github.com/openconfig/gnmic/pkg/api/utils"
	"github.com/openconfig/gnmic/pkg/cache"
	"github.com/openconfig/gnmic/pkg/formatters"
	"github.com/openconfig/gnmic/pkg/gtemplate"
	"github.com/openconfig/gnmic/pkg/outputs"
)

const (
	defaultURL             = "http://localhost:8086"
	defaultBatchSize       = 1000
	defaultFlushTimer      = 10 * time.Second
	minHealthCheckPeriod   = 30 * time.Second
	defaultCacheFlushTimer = 5 * time.Second

	numWorkers     = 1
	loggingPrefix  = "[influxdb_output:%s] "
	deleteTagValue = "true"
)

func init() {
	outputs.Register("influxdb", func() outputs.Output {
		return &influxDBOutput{
			Cfg:       &Config{},
			eventChan: make(chan *formatters.EventMsg),
			reset:     make(chan struct{}),
			startSig:  make(chan struct{}),
			logger:    log.New(io.Discard, loggingPrefix, utils.DefaultLoggingFlags),
		}
	})
}

type influxDBOutput struct {
	outputs.BaseOutput
	Cfg       *Config
	client    influxdb2.Client
	logger    *log.Logger
	cancelFn  context.CancelFunc
	eventChan chan *formatters.EventMsg
	reset     chan struct{}
	startSig  chan struct{}
	wasUP     bool
	evps      []formatters.EventProcessor
	dbVersion string

	targetTpl *template.Template

	gnmiCache   cache.Cache
	cacheTicker *time.Ticker
	done        chan struct{}
}

type Config struct {
	URL                string           `mapstructure:"url,omitempty"`
	Org                string           `mapstructure:"org,omitempty"`
	Bucket             string           `mapstructure:"bucket,omitempty"`
	Token              string           `mapstructure:"token,omitempty"`
	BatchSize          uint             `mapstructure:"batch-size,omitempty"`
	FlushTimer         time.Duration    `mapstructure:"flush-timer,omitempty"`
	UseGzip            bool             `mapstructure:"use-gzip,omitempty"`
	EnableTLS          bool             `mapstructure:"enable-tls,omitempty"`
	TLS                *types.TLSConfig `mapstructure:"tls,omitempty" json:"tls,omitempty"`
	HealthCheckPeriod  time.Duration    `mapstructure:"health-check-period,omitempty"`
	Debug              bool             `mapstructure:"debug,omitempty"`
	AddTarget          string           `mapstructure:"add-target,omitempty"`
	TargetTemplate     string           `mapstructure:"target-template,omitempty"`
	EventProcessors    []string         `mapstructure:"event-processors,omitempty"`
	EnableMetrics      bool             `mapstructure:"enable-metrics,omitempty"`
	OverrideTimestamps bool             `mapstructure:"override-timestamps,omitempty"`
	TimestampPrecision string           `mapstructure:"timestamp-precision,omitempty"`
	CacheConfig        *cache.Config    `mapstructure:"cache,omitempty"`
	CacheFlushTimer    time.Duration    `mapstructure:"cache-flush-timer,omitempty"`
	DeleteTag          string           `mapstructure:"delete-tag,omitempty"`
}

func (k *influxDBOutput) String() string {
	b, err := json.Marshal(k)
	if err != nil {
		return ""
	}
	return string(b)
}

func (i *influxDBOutput) Init(ctx context.Context, name string, cfg map[string]interface{}, opts ...outputs.Option) error {
	err := outputs.DecodeConfig(cfg, i.Cfg)
	if err != nil {
		return err
	}
	i.logger.SetPrefix(fmt.Sprintf(loggingPrefix, name))

	options := &outputs.OutputOptions{}
	for _, opt := range opts {
		if err := opt(options); err != nil {
			return err
		}
	}

	// apply logger
	if options.Logger != nil && i.logger != nil {
		i.logger.SetOutput(options.Logger.Writer())
		i.logger.SetFlags(options.Logger.Flags())
	}

	// set defaults
	i.setDefaults()

	// initialize event processors
	i.evps, err = formatters.MakeEventProcessors(i.logger, i.Cfg.EventProcessors,
		options.EventProcessors, options.TargetsConfig, options.Actions)
	if err != nil {
		return err
	}

	// initialize cache
	if i.Cfg.CacheConfig != nil {
		err = i.initCache(ctx, name)
		if err != nil {
			return err
		}
	}

	if i.Cfg.TargetTemplate == "" {
		i.targetTpl = outputs.DefaultTargetTemplate
	} else if i.Cfg.AddTarget != "" {
		i.targetTpl, err = gtemplate.CreateTemplate("target-template", i.Cfg.TargetTemplate)
		if err != nil {
			return err
		}
		i.targetTpl = i.targetTpl.Funcs(outputs.TemplateFuncs)
	}

	ctx, i.cancelFn = context.WithCancel(ctx)
	influxOpts, err := i.clientOpts()
	if err != nil {
		return err
	}
CRCLIENT:
	i.client = influxdb2.NewClientWithOptions(i.Cfg.URL, i.Cfg.Token, influxOpts)
	// start influx health check
	if i.Cfg.HealthCheckPeriod > 0 {
		err = i.health(ctx)
		if err != nil {
			i.logger.Printf("failed to check influxdb health: %v", err)
			time.Sleep(10 * time.Second)
			goto CRCLIENT
		}
		go i.healthCheck(ctx)
	}
	i.wasUP = true
	i.logger.Printf("initialized influxdb client: %s", i.String())

	for k := 0; k < numWorkers; k++ {
		go i.worker(ctx, k)
	}
	go func() {
		<-ctx.Done()
		i.Close()
	}()
	return nil
}

func (i *influxDBOutput) setDefaults() {
	if i.Cfg.URL == "" {
		i.Cfg.URL = defaultURL
	}
	if i.Cfg.BatchSize == 0 {
		i.Cfg.BatchSize = defaultBatchSize
	}
	if i.Cfg.FlushTimer == 0 {
		i.Cfg.FlushTimer = defaultFlushTimer
	}
	if i.Cfg.HealthCheckPeriod != 0 && i.Cfg.HealthCheckPeriod < minHealthCheckPeriod {
		i.Cfg.HealthCheckPeriod = minHealthCheckPeriod
	}
	if i.Cfg.CacheConfig != nil {
		if i.Cfg.CacheFlushTimer == 0 {
			i.Cfg.CacheFlushTimer = defaultCacheFlushTimer
		}
	}
}

func (i *influxDBOutput) Write(ctx context.Context, rsp proto.Message, meta outputs.Meta) {
	if rsp == nil {
		return
	}
	var err error
	rsp, err = outputs.AddSubscriptionTarget(rsp, meta, i.Cfg.AddTarget, i.targetTpl)
	if err != nil {
		i.logger.Printf("failed to add target to the response: %v", err)
	}
	switch rsp := rsp.(type) {
	case *gnmi.SubscribeResponse:
		measName := "default"
		if subName, ok := meta["subscription-name"]; ok {
			measName = subName
		}
		if i.gnmiCache != nil {
			i.gnmiCache.Write(ctx, measName, rsp)
			return
		}
		events, err := formatters.ResponseToEventMsgs(measName, rsp, meta, i.evps...)
		if err != nil {
			i.logger.Printf("failed to convert message to event: %v", err)
			return
		}
		for _, ev := range events {
			select {
			case <-ctx.Done():
				return
			case <-i.reset:
				return
			case i.eventChan <- ev:
			}
		}
	}
}

func (i *influxDBOutput) WriteEvent(ctx context.Context, ev *formatters.EventMsg) {
	select {
	case <-ctx.Done():
		return
	case <-i.reset:
		return
	default:
		var evs = []*formatters.EventMsg{ev}
		for _, proc := range i.evps {
			evs = proc.Apply(evs...)
		}
		for _, pev := range evs {
			i.eventChan <- pev
		}
	}
}

func (i *influxDBOutput) Close() error {
	i.logger.Printf("closing client...")
	if i.Cfg.CacheConfig != nil {
		i.stopCache()
	}
	i.cancelFn()
	i.logger.Printf("closed.")
	return nil
}

func (i *influxDBOutput) healthCheck(ctx context.Context) {
	ticker := time.NewTicker(i.Cfg.HealthCheckPeriod)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			i.health(ctx)
		}
	}
}

func (i *influxDBOutput) health(ctx context.Context) error {
	res, err := i.client.Health(ctx)
	if err != nil {
		i.logger.Printf("failed health check: %v", err)
		if i.wasUP {
			close(i.reset)
			i.reset = make(chan struct{})
		}
		return err
	}
	if res != nil {
		if res.Version != nil {
			i.dbVersion = *res.Version
		}
		b, err := json.Marshal(res)
		if err != nil {
			i.logger.Printf("failed to marshal health check result: %v", err)
			i.logger.Printf("health check result: %+v", res)
			if i.wasUP {
				close(i.reset)
				i.reset = make(chan struct{})
			}
			return err
		}
		i.wasUP = true
		close(i.startSig)
		i.startSig = make(chan struct{})
		i.logger.Printf("health check result: %s", string(b))
		return nil
	}
	i.wasUP = true
	close(i.startSig)
	i.startSig = make(chan struct{})
	i.logger.Print("health check result is nil")
	return nil
}

func (i *influxDBOutput) worker(ctx context.Context, idx int) {
	firstStart := true
START:
	if !firstStart {
		i.logger.Printf("worker-%d waiting for client recovery", idx)
		<-i.startSig
	}
	i.logger.Printf("starting worker-%d", idx)
	writer := i.client.WriteAPI(i.Cfg.Org, i.Cfg.Bucket)
	//defer writer.Flush()
	for {
		select {
		case <-ctx.Done():
			if ctx.Err() != nil {
				i.logger.Printf("worker-%d err=%v", idx, ctx.Err())
			}
			i.logger.Printf("worker-%d terminating...", idx)
			return
		case ev := <-i.eventChan:
			if len(ev.Values) == 0 && len(ev.Deletes) == 0 {
				continue
			}
			if len(ev.Values) == 0 && i.Cfg.DeleteTag == "" {
				continue
			}
			for n, v := range ev.Values {
				switch v := v.(type) {
				//lint:ignore SA1019 still need DecimalVal for backward compatibility
				case *gnmi.Decimal64:
					ev.Values[n] = float64(v.Digits) / math.Pow10(int(v.Precision))
				}
			}
			if ev.Timestamp == 0 || i.Cfg.OverrideTimestamps {
				ev.Timestamp = time.Now().UnixNano()
			}
			if subscriptionName, ok := ev.Tags["subscription-name"]; ok {
				ev.Name = subscriptionName
				delete(ev.Tags, "subscription-name")
			}

			if len(ev.Values) > 0 {
				i.convertUints(ev)
				writer.WritePoint(influxdb2.NewPoint(ev.Name, ev.Tags, ev.Values, time.Unix(0, ev.Timestamp)))
			}

			if len(ev.Deletes) > 0 && i.Cfg.DeleteTag != "" {
				tags := make(map[string]string, len(ev.Tags))
				for k, v := range ev.Tags {
					tags[k] = v
				}
				tags[i.Cfg.DeleteTag] = deleteTagValue
				values := make(map[string]any, len(ev.Deletes))
				for _, del := range ev.Deletes {
					values[del] = ""
				}
				writer.WritePoint(influxdb2.NewPoint(ev.Name, tags, values, time.Unix(0, ev.Timestamp)))
			}
		case <-i.reset:
			firstStart = false
			i.logger.Printf("resetting worker-%d...", idx)
			goto START
		case err := <-writer.Errors():
			i.logger.Printf("worker-%d write error: %v", idx, err)
		}
	}
}

func (i *influxDBOutput) convertUints(ev *formatters.EventMsg) {
	if !strings.HasPrefix(i.dbVersion, "1.8") {
		return
	}
	for k, v := range ev.Values {
		switch v := v.(type) {
		case uint:
			ev.Values[k] = int(v)
		case uint8:
			ev.Values[k] = int(v)
		case uint16:
			ev.Values[k] = int(v)
		case uint32:
			ev.Values[k] = int(v)
		case uint64:
			ev.Values[k] = int(v)
		}
	}
}

func (i *influxDBOutput) clientOpts() (*influxdb2.Options, error) {
	iopts := influxdb2.DefaultOptions().
		SetUseGZip(i.Cfg.UseGzip).
		SetBatchSize(i.Cfg.BatchSize).
		SetFlushInterval(uint(i.Cfg.FlushTimer.Milliseconds()))
	if i.Cfg.TLS != nil {
		tlsConfig, err := utils.NewTLSConfig(
			i.Cfg.TLS.CaFile, i.Cfg.TLS.CertFile, i.Cfg.TLS.KeyFile, "", i.Cfg.TLS.SkipVerify,
			false)
		if err != nil {
			return nil, err
		}
		iopts.SetTLSConfig(tlsConfig)
	}
	if i.Cfg.EnableTLS {
		iopts.SetTLSConfig(&tls.Config{
			InsecureSkipVerify: true,
		})
	}
	switch i.Cfg.TimestampPrecision {
	case "s":
		iopts.SetPrecision(time.Second)
	case "ms":
		iopts.SetPrecision(time.Millisecond)
	case "us":
		iopts.SetPrecision(time.Microsecond)
	}
	if i.Cfg.Debug {
		iopts.SetLogLevel(3)
	}
	return iopts, nil
}
