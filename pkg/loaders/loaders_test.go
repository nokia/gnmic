// © 2022 Nokia.
//
// This code is a Contribution to the gNMIc project (“Work”) made under the Google Software Grant and Corporate Contributor License Agreement (“CLA”) and governed by the Apache License 2.0.
// No other rights or licenses in or to any of Nokia’s intellectual property are granted for any other purpose.
// This code is provided on an “as is” basis without any warranties of any kind.
//
// SPDX-License-Identifier: Apache-2.0

package loaders

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/openconfig/gnmic/pkg/api/types"
)

var testSet = map[string]struct {
	m1, m2 map[string]*types.TargetConfig
	output *TargetOperation
}{
	"t1": {
		m1: nil,
		m2: nil,
		output: &TargetOperation{
			Add: make(map[string]*types.TargetConfig, 0),
			Del: make([]string, 0),
		},
	},
	"t2": {
		m1: nil,
		m2: map[string]*types.TargetConfig{
			"target1": {Name: "target1"},
		},
		output: &TargetOperation{
			Add: map[string]*types.TargetConfig{
				"target1": {
					Name: "target1",
				},
			},
			Del: make([]string, 0),
		},
	},
	"t3": {
		m1: map[string]*types.TargetConfig{
			"target1": {Name: "target1"},
		},
		m2: map[string]*types.TargetConfig{
			"target1": {Name: "target1"},
		},
		output: &TargetOperation{
			Add: make(map[string]*types.TargetConfig, 0),
			Del: make([]string, 0),
		},
	},
	"t4": {
		m1: map[string]*types.TargetConfig{
			"target1": {Name: "target1"},
			"target2": {Name: "target2"},
		},
		m2: map[string]*types.TargetConfig{
			"target1": {Name: "target1"},
			"target2": {Name: "target2"},
		},
		output: &TargetOperation{
			Add: make(map[string]*types.TargetConfig, 0),
			Del: make([]string, 0),
		},
	},
	"t5": {
		m1: map[string]*types.TargetConfig{
			"target1": {Name: "target1"},
		},
		m2: nil,
		output: &TargetOperation{
			Add: make(map[string]*types.TargetConfig, 0),
			Del: []string{"target1"},
		},
	},
	"t6": {
		m1: map[string]*types.TargetConfig{
			"target1": {Name: "target1"},
		},
		m2: map[string]*types.TargetConfig{
			"target1": {Name: "target1"},
			"target2": {Name: "target2"},
		},
		output: &TargetOperation{
			Add: map[string]*types.TargetConfig{
				"target2": {
					Name: "target2",
				},
			},
			Del: make([]string, 0),
		},
	},
	"t7": {
		m1: map[string]*types.TargetConfig{
			"target1": {Name: "target1"},
		},
		m2: map[string]*types.TargetConfig{
			"target2": {Name: "target2"},
		},
		output: &TargetOperation{
			Add: map[string]*types.TargetConfig{
				"target2": {
					Name: "target2",
				},
			},
			Del: []string{"target1"},
		},
	},
	"t8": {
		m1: map[string]*types.TargetConfig{
			"target1": {Name: "target1"},
		},
		m2: map[string]*types.TargetConfig{
			"target2": {Name: "target2"},
			"target3": {Name: "target3"},
		},
		output: &TargetOperation{
			Add: map[string]*types.TargetConfig{
				"target2": {
					Name: "target2",
				},
				"target3": {
					Name: "target3",
				},
			},
			Del: []string{"target1"},
		},
	},
	"t9": {
		m1: map[string]*types.TargetConfig{
			"target1": {Name: "target1"},
			"target2": {Name: "target2"},
		},
		m2: map[string]*types.TargetConfig{
			"target2": {Name: "target2"},
			"target3": {Name: "target3"},
		},
		output: &TargetOperation{
			Add: map[string]*types.TargetConfig{
				"target3": {
					Name: "target3",
				},
			},
			Del: []string{"target1"},
		},
	},
	"t10-target-change": {
		m1: map[string]*types.TargetConfig{
			"target1": {Address: "ip1"},
			"target2": {Address: "ip2"},
		},
		m2: map[string]*types.TargetConfig{
			"target1": {Address: "ip1"},
			"target2": {Address: "ip2new"},
		},
		output: &TargetOperation{
			Add: map[string]*types.TargetConfig{
				"target2": {
					Address: "ip2new",
				},
			},
			Del: []string{"target2"},
		},
	},
	"t11-tags-change": {
		m1: map[string]*types.TargetConfig{
			"target1": {Name: "target1", Tags: []string{"a"}},
		},
		m2: map[string]*types.TargetConfig{
			"target1": {Name: "target1", Tags: []string{"a", "b"}},
		},
		output: &TargetOperation{
			Add: map[string]*types.TargetConfig{
				"target1": {Name: "target1", Tags: []string{"a", "b"}},
			},
			Del: []string{"target1"},
		},
	},
	"t12-both-empty": {
		m1: map[string]*types.TargetConfig{},
		m2: map[string]*types.TargetConfig{},
		output: &TargetOperation{
			Add: make(map[string]*types.TargetConfig, 0),
			Del: make([]string, 0),
		},
	},
	"t13-slice-order-change": {
		m1: map[string]*types.TargetConfig{
			"target1": {Name: "target1", Tags: []string{"a", "b"}},
		},
		m2: map[string]*types.TargetConfig{
			"target1": {Name: "target1", Tags: []string{"b", "a"}},
		},
		output: &TargetOperation{
			Add: map[string]*types.TargetConfig{
				"target1": {Name: "target1", Tags: []string{"b", "a"}},
			},
			Del: []string{"target1"},
		},
	},
}

func TestGetInstancesTagsMatches(t *testing.T) {
	for name, item := range testSet {
		t.Run(name, func(t *testing.T) {
			res := Diff(item.m1, item.m2)
			t.Logf("exp value: %+v", item.output)
			t.Logf("got value: %+v", res)
			if len(item.output.Add) != len(res.Add) {
				t.Fail()
			}
			if len(item.output.Del) != len(res.Del) {
				t.Fail()
			}
			for k, v1 := range item.output.Add {
				if v2, ok := res.Add[k]; ok {
					if v1.String() != v2.String() {
						t.Fail()
					}
				} else {
					t.Fail()
				}
			}
			if !cmp.Equal(item.output.Del, res.Del) {
				t.Fail()
			}
		})
	}
}
