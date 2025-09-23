// © 2022 Nokia.
//
// This code is a Contribution to the gNMIc project (“Work”) made under the Google Software Grant and Corporate Contributor License Agreement (“CLA”) and governed by the Apache License 2.0.
// No other rights or licenses in or to any of Nokia’s intellectual property are granted for any other purpose.
// This code is provided on an “as is” basis without any warranties of any kind.
//
// SPDX-License-Identifier: Apache-2.0

package event_override_ts

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"time"

	"github.com/openconfig/gnmic/pkg/api/utils"
	"github.com/openconfig/gnmic/pkg/formatters"
)

const (
	processorType = "event-override-ts"
	loggingPrefix = "[" + processorType + "] "
)

// overrideTS Overrides the message timestamp with the local time
type overrideTS struct {
	formatters.BaseProcessor

	Precision string `mapstructure:"precision,omitempty" json:"precision,omitempty"`
	Debug     bool   `mapstructure:"debug,omitempty" json:"debug,omitempty"`

	logger *log.Logger
}

func init() {
	formatters.Register(processorType, func() formatters.EventProcessor {
		return &overrideTS{
			logger: log.New(io.Discard, "", 0),
		}
	})
}

func (o *overrideTS) Init(cfg any, opts ...formatters.Option) error {
	err := formatters.DecodeConfig(cfg, o)
	if err != nil {
		return err
	}
	for _, opt := range opts {
		opt(o)
	}
	if o.Precision == "" {
		o.Precision = "ns"
	}
	if o.logger.Writer() != io.Discard {
		b, err := json.Marshal(o)
		if err != nil {
			o.logger.Printf("initialized processor '%s': %+v", processorType, o)
			return nil
		}
		o.logger.Printf("initialized processor '%s': %s", processorType, string(b))
	}
	return nil
}

func (o *overrideTS) Apply(es ...*formatters.EventMsg) []*formatters.EventMsg {
	for _, e := range es {
		if e == nil {
			continue
		}
		now := time.Now()
		o.logger.Printf("setting timestamp to %d with precision %s", now.UnixNano(), o.Precision)
		switch o.Precision {
		case "s":
			e.Timestamp = now.Unix()
		case "ms":
			e.Timestamp = now.UnixNano() / 1000000
		case "us":
			e.Timestamp = now.UnixNano() / 1000
		case "ns":
			e.Timestamp = now.UnixNano()
		}
	}
	return es
}

func (o *overrideTS) WithLogger(l *log.Logger) {
	if o.Debug && l != nil {
		o.logger = log.New(l.Writer(), loggingPrefix, l.Flags())
	} else if o.Debug {
		o.logger = log.New(os.Stderr, loggingPrefix, utils.DefaultLoggingFlags)
	}
}
