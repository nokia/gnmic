// © 2022 Nokia.
//
// This code is a Contribution to the gNMIc project (“Work”) made under the Google Software Grant and Corporate Contributor License Agreement (“CLA”) and governed by the Apache License 2.0.
// No other rights or licenses in or to any of Nokia’s intellectual property are granted for any other purpose.
// This code is provided on an “as is” basis without any warranties of any kind.
//
// SPDX-License-Identifier: Apache-2.0

package event_strings

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/openconfig/gnmic/pkg/api/utils"
	"github.com/openconfig/gnmic/pkg/formatters"
)

const (
	processorType = "event-strings"
	loggingPrefix = "[" + processorType + "] "
	nameField     = "name"
	valueField    = "value"
)

// stringsp provides some of Golang's strings functions to transform: tags, tag names, values and value names
type stringsp struct {
	formatters.BaseProcessor

	Tags       []string                `mapstructure:"tags,omitempty" json:"tags,omitempty"`
	Values     []string                `mapstructure:"values,omitempty" json:"values,omitempty"`
	TagNames   []string                `mapstructure:"tag-names,omitempty" json:"tag-names,omitempty"`
	ValueNames []string                `mapstructure:"value-names,omitempty" json:"value-names,omitempty"`
	Debug      bool                    `mapstructure:"debug,omitempty" json:"debug,omitempty"`
	Transforms []map[string]*transform `mapstructure:"transforms,omitempty" json:"transforms,omitempty"`

	tags      []*regexp.Regexp
	values    []*regexp.Regexp
	tagKeys   []*regexp.Regexp
	valueKeys []*regexp.Regexp

	logger *log.Logger
}

type transform struct {
	op string
	// apply the transformation on name or value
	ApplyOn string `mapstructure:"apply-on,omitempty" json:"apply-on,omitempty"`
	// Keep the old value or not if the name changed
	Keep bool `mapstructure:"keep,omitempty" json:"keep,omitempty"`
	// string to be replaced
	Old string `mapstructure:"old,omitempty" json:"old,omitempty"`
	// replacement string of Old
	New string `mapstructure:"new,omitempty" json:"new,omitempty"`
	// Prefix to be trimmed
	Prefix string `mapstructure:"prefix,omitempty" json:"prefix,omitempty"`
	// Suffix to be trimmed
	Suffix string `mapstructure:"suffix,omitempty" json:"suffix,omitempty"`
	// character to split on
	SplitOn string `mapstructure:"split-on,omitempty" json:"split-on,omitempty"`
	// character to join with
	JoinWith string `mapstructure:"join-with,omitempty" json:"join-with,omitempty"`
	// number of first items to ignore when joining
	IgnoreFirst int `mapstructure:"ignore-first,omitempty" json:"ignore-first,omitempty"`
	// number of last items to ignore when joining
	IgnoreLast int `mapstructure:"ignore-last,omitempty" json:"ignore-last,omitempty"`

	//
	replaceRegexp *regexp.Regexp
}

func init() {
	formatters.Register(processorType, func() formatters.EventProcessor {
		return &stringsp{
			logger: log.New(io.Discard, "", 0),
		}
	})
}

func (s *stringsp) Init(cfg interface{}, opts ...formatters.Option) error {
	err := formatters.DecodeConfig(cfg, s)
	if err != nil {
		return err
	}
	for _, opt := range opts {
		opt(s)
	}
	for i := range s.Transforms {
		for k := range s.Transforms[i] {
			s.Transforms[i][k].op = k
			switch k {
			case "replace":
				s.Transforms[i][k].replaceRegexp, err = regexp.Compile(s.Transforms[i][k].Old)
				if err != nil {
					return err
				}
			}
		}
	}
	// init tags regex
	s.tags = make([]*regexp.Regexp, 0, len(s.Tags))
	for _, reg := range s.Tags {
		re, err := regexp.Compile(reg)
		if err != nil {
			return err
		}
		s.tags = append(s.tags, re)
	}
	// init tag names regex
	s.tagKeys = make([]*regexp.Regexp, 0, len(s.TagNames))
	for _, reg := range s.TagNames {
		re, err := regexp.Compile(reg)
		if err != nil {
			return err
		}
		s.tagKeys = append(s.tagKeys, re)
	}
	// init values regex
	s.values = make([]*regexp.Regexp, 0, len(s.Values))
	for _, reg := range s.Values {
		re, err := regexp.Compile(reg)
		if err != nil {
			return err
		}
		s.values = append(s.values, re)
	}
	// init value Keys regex
	s.valueKeys = make([]*regexp.Regexp, 0, len(s.ValueNames))
	for _, reg := range s.ValueNames {
		re, err := regexp.Compile(reg)
		if err != nil {
			return err
		}
		s.valueKeys = append(s.valueKeys, re)
	}
	if s.logger.Writer() != io.Discard {
		b, err := json.Marshal(s)
		if err != nil {
			s.logger.Printf("initialized processor '%s': %+v", processorType, s)
			return nil
		}
		s.logger.Printf("initialized processor '%s': %s", processorType, string(b))
	}
	return nil
}

func (s *stringsp) Apply(es ...*formatters.EventMsg) []*formatters.EventMsg {
	for _, e := range es {
		if e == nil {
			continue
		}
		for k, v := range e.Values {
			for _, re := range s.valueKeys {
				if re.MatchString(k) {
					s.logger.Printf("value name '%s' matched regex '%s'", k, re.String())
					s.applyValueTransformations(e, k, v)
				}
			}
			for _, re := range s.values {
				if vs, ok := v.(string); ok {
					if re.MatchString(vs) {
						s.logger.Printf("value '%s' matched regex '%s'", vs, re.String())
						s.applyValueTransformations(e, k, vs)
					}
				}
			}
		}
		for k, v := range e.Tags {
			for _, re := range s.tagKeys {
				if re.MatchString(k) {
					s.logger.Printf("tag name '%s' matched regex '%s'", k, re.String())
					s.applyTagTransformations(e, k, v)
				}
			}
			for _, re := range s.tags {
				if re.MatchString(v) {
					s.logger.Printf("tag '%s' matched regex '%s'", k, re.String())
					s.applyTagTransformations(e, k, v)
				}
			}
		}
	}
	return es
}

func (s *stringsp) WithLogger(l *log.Logger) {
	if s.Debug && l != nil {
		s.logger = log.New(l.Writer(), loggingPrefix, l.Flags())
	} else if s.Debug {
		s.logger = log.New(os.Stderr, loggingPrefix, utils.DefaultLoggingFlags)
	}
}

func (s *stringsp) applyValueTransformations(e *formatters.EventMsg, k string, v interface{}) {
	for _, trans := range s.Transforms {
		for _, t := range trans {
			if !t.Keep {
				delete(e.Values, k)
			}
			k, v = t.apply(k, v)
			e.Values[k] = v
		}
	}
}

func (s *stringsp) applyTagTransformations(e *formatters.EventMsg, k, v string) {
	for _, trans := range s.Transforms {
		for _, t := range trans {
			if !t.Keep {
				delete(e.Tags, k)
			}
			var vi interface{}
			k, vi = t.apply(k, v)
			if vs, ok := vi.(string); ok {
				e.Tags[k] = vs
				v = vs // change the original value in case it's used in the next transform
				continue
			}
			s.logger.Printf("failed to assert %v type as string", vi)
		}
	}
}

func (t *transform) apply(k string, v interface{}) (string, interface{}) {
	switch t.op {
	case "replace":
		return t.replace(k, v)
	case "trim-prefix":
		return t.trimPrefix(k, v)
	case "trim-suffix":
		return t.trimSuffix(k, v)
	case "title":
		return t.toTitle(k, v)
	case "to-lower":
		return t.toLower(k, v)
	case "to-upper":
		return t.toUpper(k, v)
	case "split":
		return t.split(k, v)
	case "path-base":
		return t.pathBase(k, v)
	}
	return k, v
}

func (t *transform) replace(k string, v interface{}) (string, interface{}) {
	switch t.ApplyOn {
	case nameField:
		k = t.replaceRegexp.ReplaceAllString(k, t.New)
	case valueField:
		if vs, ok := v.(string); ok {
			v = t.replaceRegexp.ReplaceAllString(vs, t.New)
		}
	}
	return k, v
}

func (t *transform) trimPrefix(k string, v interface{}) (string, interface{}) {
	switch t.ApplyOn {
	case nameField:
		k = strings.TrimPrefix(k, t.Prefix)
	case valueField:
		if vs, ok := v.(string); ok {
			v = strings.TrimPrefix(vs, t.Prefix)
		}
	}
	return k, v
}

func (t *transform) trimSuffix(k string, v interface{}) (string, interface{}) {
	switch t.ApplyOn {
	case nameField:
		k = strings.TrimSuffix(k, t.Suffix)
	case valueField:
		if vs, ok := v.(string); ok {
			v = strings.TrimSuffix(vs, t.Suffix)
		}
	}
	return k, v
}

func (t *transform) toTitle(k string, v interface{}) (string, interface{}) {
	switch t.ApplyOn {
	case nameField:
		k = cases.Title(language.English).String(k)
	case valueField:
		if vs, ok := v.(string); ok {
			v = cases.Title(language.English).String(vs)
		}
	}
	return k, v
}

func (t *transform) toLower(k string, v interface{}) (string, interface{}) {
	switch t.ApplyOn {
	case nameField:
		k = strings.ToLower(k)
	case valueField:
		if vs, ok := v.(string); ok {
			v = strings.ToLower(vs)
		}
	}
	return k, v
}

func (t *transform) toUpper(k string, v interface{}) (string, interface{}) {
	switch t.ApplyOn {
	case nameField:
		k = strings.ToUpper(k)
	case valueField:
		if vs, ok := v.(string); ok {
			v = strings.ToUpper(vs)
		}
	}
	return k, v
}

func (t *transform) split(k string, v interface{}) (string, interface{}) {
	switch t.ApplyOn {
	case nameField:
		items := strings.Split(k, t.SplitOn)
		numItems := len(items)
		if numItems <= t.IgnoreFirst || numItems <= t.IgnoreLast || t.IgnoreFirst >= numItems-t.IgnoreLast {
			return "", v
		}
		k = strings.Join(items[t.IgnoreFirst:numItems-t.IgnoreLast], t.JoinWith)
	case valueField:
		if vs, ok := v.(string); ok {
			items := strings.Split(vs, t.SplitOn)
			numItems := len(items)
			if numItems <= t.IgnoreFirst || numItems <= t.IgnoreLast || t.IgnoreFirst >= numItems-t.IgnoreLast {
				return k, ""
			}
			v = strings.Join(items[t.IgnoreFirst:numItems-t.IgnoreLast], t.JoinWith)
		}
	}
	return k, v
}

func (t *transform) pathBase(k string, v interface{}) (string, interface{}) {
	switch t.ApplyOn {
	case nameField:
		k = filepath.Base(k)
	case valueField:
		if vs, ok := v.(string); ok {
			v = filepath.Base(vs)
		}
	}
	return k, v
}
