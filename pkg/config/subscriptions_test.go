// © 2022 Nokia.
//
// This code is a Contribution to the gNMIc project (“Work”) made under the Google Software Grant and Corporate Contributor License Agreement (“CLA”) and governed by the Apache License 2.0.
// No other rights or licenses in or to any of Nokia’s intellectual property are granted for any other purpose.
// This code is provided on an “as is” basis without any warranties of any kind.
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"bytes"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/openconfig/gnmic/pkg/api/types"
)

func mustParseTime(tm string) time.Time {
	tmi, err := time.Parse(time.RFC3339Nano, tm)
	if err != nil {
		panic(fmt.Sprintf("cannot parse time: %v", err))
	}

	return tmi
}

var getSubscriptionsTestSet = map[string]struct {
	envs   []string
	in     []byte
	out    map[string]*types.SubscriptionConfig
	outErr error
}{
	"no_globals": {
		in: []byte(`
subscriptions:
  sub1:
    paths: 
      - /valid/path
`),
		out: map[string]*types.SubscriptionConfig{
			"sub1": {
				Name:  "sub1",
				Paths: []string{"/valid/path"},
			},
		},
		outErr: nil,
	},
	// 	"with_globals": {
	// 		in: []byte(`
	// subscribe-sample-interval: 10s
	// subscriptions:
	//   sub1:
	//     paths:
	//       - /valid/path
	// `),
	// 		out: map[string]*types.SubscriptionConfig{
	// 			"sub1": {
	// 				Name:           "sub1",
	// 				Paths:          []string{"/valid/path"},
	// 				SampleInterval: pointer.ToDuration(10 * time.Second),
	// 			},
	// 		},
	// 		outErr: nil,
	// 	},
	"2_subs": {
		in: []byte(`
subscriptions:
  sub1:
    paths: 
      - /valid/path
  sub2:
    paths: 
      - /valid/path2
    mode: stream
    stream-mode: on_change
`),
		out: map[string]*types.SubscriptionConfig{
			"sub1": {
				Name:  "sub1",
				Paths: []string{"/valid/path"},
			},
			"sub2": {
				Name:       "sub2",
				Paths:      []string{"/valid/path2"},
				Mode:       "stream",
				StreamMode: "on_change",
			},
		},
		outErr: nil,
	},
	// 	"2_subs_with_globals": {
	// 		in: []byte(`
	// subscribe-sample-interval: 10s
	// subscriptions:
	//   sub1:
	//     paths:
	//       - /valid/path
	//   sub2:
	//     paths:
	//       - /valid/path2
	//     mode: stream
	//     stream-mode: on_change
	// `),
	// 		out: map[string]*types.SubscriptionConfig{
	// 			"sub1": {
	// 				Name:           "sub1",
	// 				Paths:          []string{"/valid/path"},
	// 				SampleInterval: pointer.ToDuration(10 * time.Second),
	// 			},
	// 			"sub2": {
	// 				Name:           "sub2",
	// 				Paths:          []string{"/valid/path2"},
	// 				Mode:           "stream",
	// 				StreamMode:     "on_change",
	// 				SampleInterval: pointer.ToDuration(10 * time.Second),
	// 			},
	// 		},
	// 		outErr: nil,
	// 	},
	"3_subs_with_env": {
		envs: []string{
			"SUB1_PATH=/valid/path",
			"SUB2_PATH=/valid/path2",
		},
		in: []byte(`
subscriptions:
  sub1:
    paths: 
      - ${SUB1_PATH}
  sub2:
    paths: 
      - ${SUB2_PATH}
    mode: stream
    stream-mode: on_change
`),
		out: map[string]*types.SubscriptionConfig{
			"sub1": {
				Name:  "sub1",
				Paths: []string{"/valid/path"},
			},
			"sub2": {
				Name:       "sub2",
				Paths:      []string{"/valid/path2"},
				Mode:       "stream",
				StreamMode: "on_change",
			},
		},
		outErr: nil,
	},
	"history_snapshot": {
		in: []byte(`
subscriptions:
  sub1:
    paths: 
      - /valid/path
    history:
      snapshot: 2022-07-14T07:30:00.0Z
`),
		out: map[string]*types.SubscriptionConfig{
			"sub1": {
				Name:  "sub1",
				Paths: []string{"/valid/path"},
				History: &types.HistoryConfig{
					Snapshot: mustParseTime("2022-07-14T07:30:00.0Z"),
				},
			},
		},
		outErr: nil,
	},
	"history_range": {
		in: []byte(`
subscriptions:
  sub1:
    paths: 
      - /valid/path
    history:
      start: 2021-07-14T07:30:00.0Z
      end: 2022-07-14T07:30:00.0Z
`),
		out: map[string]*types.SubscriptionConfig{
			"sub1": {
				Name:  "sub1",
				Paths: []string{"/valid/path"},
				History: &types.HistoryConfig{
					Start: mustParseTime("2021-07-14T07:30:00.0Z"),
					End:   mustParseTime("2022-07-14T07:30:00.0Z"),
				},
			},
		},
		outErr: nil,
	},
	"subscription_list": {
		in: []byte(`
subscriptions:
  sub1:
    stream-subscriptions:
      - paths:
        - /valid/path1
        stream-mode: sample
      - paths:
        - /valid/path2
        stream-mode: on-change
`),
		out: map[string]*types.SubscriptionConfig{
			"sub1": {
				Name: "sub1",
				StreamSubscriptions: []*types.SubscriptionConfig{
					{
						Paths:      []string{"/valid/path1"},
						StreamMode: "sample",
					},
					{
						Paths:      []string{"/valid/path2"},
						StreamMode: "on-change",
					},
				},
			},
		},
		outErr: nil,
	},
}

func TestGetSubscriptions(t *testing.T) {
	for name, data := range getSubscriptionsTestSet {
		t.Run(name, func(t *testing.T) {
			for _, e := range data.envs {
				p := strings.SplitN(e, "=", 2)
				os.Setenv(p[0], p[1])
			}
			cfg := New()
			cfg.Debug = true
			cfg.SetLogger()
			cfg.FileConfig.SetConfigType("yaml")
			err := cfg.FileConfig.ReadConfig(bytes.NewBuffer(data.in))
			if err != nil {
				t.Logf("failed reading config: %v", err)
				t.Fail()
			}
			err = cfg.FileConfig.Unmarshal(cfg)
			if err != nil {
				t.Logf("failed fileConfig.Unmarshal: %v", err)
				t.Fail()
			}
			v := cfg.FileConfig.Get("subscriptions")
			t.Logf("raw interface subscriptions: %+v", v)
			outs, err := cfg.GetSubscriptions(nil)
			t.Logf("exp value: %+v", data.out)
			t.Logf("got value: %+v", outs)
			if err != nil {
				t.Logf("failed getting subscriptions: %v", err)
				t.Fail()
			}
			if !reflect.DeepEqual(outs, data.out) {
				t.Log("maps not equal")
				t.Fail()
			}
		})
	}
}

// func TestConfig_CreateSubscribeRequest(t *testing.T) {
// 	type fields struct {
// 		GlobalFlags        GlobalFlags
// 		LocalFlags         LocalFlags
// 		FileConfig         *viper.Viper
// 		Targets            map[string]*types.TargetConfig
// 		Subscriptions      map[string]*types.SubscriptionConfig
// 		Outputs            map[string]map[string]interface{}
// 		Inputs             map[string]map[string]interface{}
// 		Processors         map[string]map[string]interface{}
// 		Clustering         *clustering
// 		GnmiServer         *gnmiServer
// 		APIServer          *APIServer
// 		Loader             map[string]interface{}
// 		Actions            map[string]map[string]interface{}
// 		logger             *log.Logger
// 		setRequestTemplate []*template.Template
// 		setRequestVars     map[string]interface{}
// 	}
// 	type args struct {
// 		sc     *types.SubscriptionConfig
// 		target *types.TargetConfig
// 	}
// 	tests := []struct {
// 		name    string
// 		fields  fields
// 		args    args
// 		want    *gnmi.SubscribeRequest
// 		wantErr bool
// 	}{
// 		{
// 			name: "once_subscription",
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Paths: []string{
// 						"interface",
// 					},
// 					Mode:     "once",
// 					Encoding: pointer.ToString("json_ietf"),
// 				},
// 			},
// 			want: &gnmi.SubscribeRequest{
// 				Request: &gnmi.SubscribeRequest_Subscribe{
// 					Subscribe: &gnmi.SubscriptionList{
// 						Subscription: []*gnmi.Subscription{
// 							{
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{{
// 										Name: "interface",
// 									}},
// 								},
// 							},
// 						},
// 						Mode:     gnmi.SubscriptionList_ONCE,
// 						Encoding: gnmi.Encoding_JSON_IETF,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "once_subscription_multiple_paths",
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Paths: []string{
// 						"interface",
// 						"network-instance",
// 					},
// 					Mode:     "once",
// 					Encoding: pointer.ToString("json_ietf"),
// 				},
// 			},
// 			want: &gnmi.SubscribeRequest{
// 				Request: &gnmi.SubscribeRequest_Subscribe{
// 					Subscribe: &gnmi.SubscriptionList{
// 						Mode: gnmi.SubscriptionList_ONCE,
// 						Subscription: []*gnmi.Subscription{
// 							{
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{{
// 										Name: "interface",
// 									}},
// 								},
// 							},
// 							{
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{{
// 										Name: "network-instance",
// 									}},
// 								},
// 							},
// 						},
// 						Encoding: gnmi.Encoding_JSON_IETF,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "poll_subscription",
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Paths: []string{
// 						"interface",
// 					},
// 					Mode:     "poll",
// 					Encoding: pointer.ToString("json_ietf"),
// 				},
// 			},
// 			want: &gnmi.SubscribeRequest{
// 				Request: &gnmi.SubscribeRequest_Subscribe{
// 					Subscribe: &gnmi.SubscriptionList{
// 						Subscription: []*gnmi.Subscription{
// 							{
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{{
// 										Name: "interface",
// 									}},
// 								},
// 							},
// 						},
// 						Mode:     gnmi.SubscriptionList_POLL,
// 						Encoding: gnmi.Encoding_JSON_IETF,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "poll_subscription_multiple_paths",
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Paths: []string{
// 						"interface",
// 						"network-instance",
// 					},
// 					Mode:     "poll",
// 					Encoding: pointer.ToString("json_ietf"),
// 				},
// 			},
// 			want: &gnmi.SubscribeRequest{
// 				Request: &gnmi.SubscribeRequest_Subscribe{
// 					Subscribe: &gnmi.SubscriptionList{
// 						Subscription: []*gnmi.Subscription{
// 							{
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{{
// 										Name: "interface",
// 									}},
// 								},
// 							},
// 							{
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{{
// 										Name: "network-instance",
// 									}},
// 								},
// 							},
// 						},
// 						Mode:     gnmi.SubscriptionList_POLL,
// 						Encoding: gnmi.Encoding_JSON_IETF,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "stream_subscription",
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Paths: []string{
// 						"interface",
// 					},
// 					Mode:     "stream",
// 					Encoding: pointer.ToString("json_ietf"),
// 				},
// 			},
// 			want: &gnmi.SubscribeRequest{
// 				Request: &gnmi.SubscribeRequest_Subscribe{
// 					Subscribe: &gnmi.SubscriptionList{
// 						Subscription: []*gnmi.Subscription{
// 							{
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{{
// 										Name: "interface",
// 									}},
// 								},
// 							},
// 						},
// 						Mode:     gnmi.SubscriptionList_STREAM,
// 						Encoding: gnmi.Encoding_JSON_IETF,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "stream_subscription_multiple_paths",
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Paths: []string{
// 						"interface",
// 						"network-instance",
// 					},
// 					Mode:     "stream",
// 					Encoding: pointer.ToString("json_ietf"),
// 				},
// 			},
// 			want: &gnmi.SubscribeRequest{
// 				Request: &gnmi.SubscribeRequest_Subscribe{
// 					Subscribe: &gnmi.SubscriptionList{
// 						Subscription: []*gnmi.Subscription{
// 							{
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{{
// 										Name: "interface",
// 									}},
// 								},
// 							},
// 							{
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{{
// 										Name: "network-instance",
// 									}},
// 								},
// 							},
// 						},
// 						Mode:     gnmi.SubscriptionList_STREAM,
// 						Encoding: gnmi.Encoding_JSON_IETF,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "stream_sample_subscription",
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Paths: []string{
// 						"interface",
// 					},
// 					StreamMode:     "sample",
// 					Encoding:       pointer.ToString("json_ietf"),
// 					SampleInterval: pointer.ToDuration(5 * time.Second),
// 				},
// 			},
// 			want: &gnmi.SubscribeRequest{
// 				Request: &gnmi.SubscribeRequest_Subscribe{
// 					Subscribe: &gnmi.SubscriptionList{
// 						Subscription: []*gnmi.Subscription{
// 							{
// 								Mode: gnmi.SubscriptionMode_SAMPLE,
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{{
// 										Name: "interface",
// 									}},
// 								},
// 							},
// 						},
// 						Encoding: gnmi.Encoding_JSON_IETF,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "stream_on_change_subscription",
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Paths: []string{
// 						"interface",
// 					},
// 					StreamMode: "on-change",
// 					Encoding:   pointer.ToString("json_ietf"),
// 				},
// 			},
// 			want: &gnmi.SubscribeRequest{
// 				Request: &gnmi.SubscribeRequest_Subscribe{
// 					Subscribe: &gnmi.SubscriptionList{
// 						Subscription: []*gnmi.Subscription{
// 							{
// 								Mode: gnmi.SubscriptionMode_ON_CHANGE,
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{{
// 										Name: "interface",
// 									}},
// 								},
// 							},
// 						},
// 						Encoding: gnmi.Encoding_JSON_IETF,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "stream_target_defined_subscription",
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Paths: []string{
// 						"interface",
// 					},
// 					StreamMode: "on_change",
// 					Encoding:   pointer.ToString("json_ietf"),
// 				},
// 			},
// 			want: &gnmi.SubscribeRequest{
// 				Request: &gnmi.SubscribeRequest_Subscribe{
// 					Subscribe: &gnmi.SubscriptionList{
// 						Subscription: []*gnmi.Subscription{
// 							{
// 								Mode: gnmi.SubscriptionMode_ON_CHANGE,
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{{
// 										Name: "interface",
// 									}},
// 								},
// 							},
// 						},
// 						Encoding: gnmi.Encoding_JSON_IETF,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "subscription_with_history_snapshot",
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Paths: []string{
// 						"interface",
// 					},
// 					Mode:     "once",
// 					Encoding: pointer.ToString("json_ietf"),
// 					History: &types.HistoryConfig{
// 						Snapshot: mustParseTime("2022-07-14T07:30:00.0Z"),
// 					},
// 				},
// 			},
// 			want: &gnmi.SubscribeRequest{
// 				Request: &gnmi.SubscribeRequest_Subscribe{
// 					Subscribe: &gnmi.SubscriptionList{
// 						Subscription: []*gnmi.Subscription{
// 							{
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{{
// 										Name: "interface",
// 									}},
// 								},
// 							},
// 						},
// 						Encoding: gnmi.Encoding_JSON_IETF,
// 						Mode:     gnmi.SubscriptionList_ONCE,
// 					},
// 				},
// 				Extension: []*gnmi_ext.Extension{
// 					{
// 						Ext: &gnmi_ext.Extension_History{
// 							History: &gnmi_ext.History{
// 								Request: &gnmi_ext.History_SnapshotTime{
// 									SnapshotTime: 1657783800000000,
// 								},
// 							},
// 						},
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "combined_on-change_and_sample",
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Encoding: pointer.ToString("json_ietf"),
// 					StreamSubscriptions: []*types.SubscriptionConfig{
// 						{
// 							Paths: []string{
// 								"interface/admin-state",
// 							},
// 							StreamMode: "ON_CHANGE",
// 						},
// 						{
// 							Paths: []string{
// 								"interface/statistics",
// 							},
// 							StreamMode: "SAMPLE",
// 						},
// 					},
// 				},
// 			},
// 			want: &gnmi.SubscribeRequest{
// 				Request: &gnmi.SubscribeRequest_Subscribe{
// 					Subscribe: &gnmi.SubscriptionList{
// 						Subscription: []*gnmi.Subscription{
// 							{
// 								Mode: gnmi.SubscriptionMode_ON_CHANGE,
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{
// 										{
// 											Name: "interface",
// 										},
// 										{
// 											Name: "admin-state",
// 										},
// 									},
// 								},
// 							},
// 							{
// 								Mode: gnmi.SubscriptionMode_SAMPLE,
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{
// 										{
// 											Name: "interface",
// 										},
// 										{
// 											Name: "statistics",
// 										},
// 									},
// 								},
// 							},
// 						},
// 						Encoding: gnmi.Encoding_JSON_IETF,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "combined_on-change_and_sample_multiple_paths",
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Encoding: pointer.ToString("json_ietf"),
// 					StreamSubscriptions: []*types.SubscriptionConfig{
// 						{
// 							Paths: []string{
// 								"interface/admin-state",
// 								"interface/oper-state",
// 							},
// 							StreamMode: "ON_CHANGE",
// 						},
// 						{
// 							Paths: []string{
// 								"interface/statistics",
// 								"interface/subinterface/statistics",
// 							},
// 							StreamMode: "SAMPLE",
// 						},
// 					},
// 				},
// 			},
// 			want: &gnmi.SubscribeRequest{
// 				Request: &gnmi.SubscribeRequest_Subscribe{
// 					Subscribe: &gnmi.SubscriptionList{
// 						Subscription: []*gnmi.Subscription{
// 							{
// 								Mode: gnmi.SubscriptionMode_ON_CHANGE,
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{
// 										{
// 											Name: "interface",
// 										},
// 										{
// 											Name: "admin-state",
// 										},
// 									},
// 								},
// 							},
// 							{
// 								Mode: gnmi.SubscriptionMode_ON_CHANGE,
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{
// 										{
// 											Name: "interface",
// 										},
// 										{
// 											Name: "oper-state",
// 										},
// 									},
// 								},
// 							},
// 							{
// 								Mode: gnmi.SubscriptionMode_SAMPLE,
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{
// 										{
// 											Name: "interface",
// 										},
// 										{
// 											Name: "statistics",
// 										},
// 									},
// 								},
// 							},
// 							{
// 								Mode: gnmi.SubscriptionMode_SAMPLE,
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{
// 										{
// 											Name: "interface",
// 										},
// 										{
// 											Name: "subinterface",
// 										},
// 										{
// 											Name: "statistics",
// 										},
// 									},
// 								},
// 							},
// 						},
// 						Encoding: gnmi.Encoding_JSON_IETF,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "invalid_combined_paths_and_subscriptions",
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Paths:    []string{"network-instance"},
// 					Encoding: pointer.ToString("json_ietf"),
// 					StreamSubscriptions: []*types.SubscriptionConfig{
// 						{
// 							Paths: []string{
// 								"interface/admin-state",
// 							},
// 							StreamMode: "ON_CHANGE",
// 						},
// 						{
// 							Paths: []string{
// 								"interface/statistics",
// 							},
// 							StreamMode: "SAMPLE",
// 						},
// 					},
// 				},
// 			},
// 			wantErr: true,
// 		},
// 		{
// 			name: "invalid_combined_subscriptions_mode",
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Encoding: pointer.ToString("json_ietf"),
// 					StreamSubscriptions: []*types.SubscriptionConfig{
// 						{
// 							Paths: []string{
// 								"interface/admin-state",
// 							},
// 							Mode: "ONCE",
// 						},
// 						{
// 							Paths: []string{
// 								"interface/statistics",
// 							},
// 							StreamMode: "SAMPLE",
// 						},
// 					},
// 				},
// 			},
// 			wantErr: true,
// 		},
// 		{
// 			name: "invalid_subscription mode",
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Encoding: pointer.ToString("json_ietf"),
// 					Mode:     "ONCE",
// 					StreamSubscriptions: []*types.SubscriptionConfig{
// 						{
// 							Paths: []string{
// 								"interface/admin-state",
// 							},
// 							Mode: "ON_CHANGE",
// 						},
// 						{
// 							Paths: []string{
// 								"interface/statistics",
// 							},
// 							StreamMode: "SAMPLE",
// 						},
// 					},
// 				},
// 			},
// 			wantErr: true,
// 		},
// 		{
// 			name: "encoding_from_target",
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Paths: []string{
// 						"interface",
// 					},
// 					Mode: "once",
// 				},
// 				target: &types.TargetConfig{
// 					Encoding: pointer.ToString("json_ietf"),
// 				},
// 			},
// 			want: &gnmi.SubscribeRequest{
// 				Request: &gnmi.SubscribeRequest_Subscribe{
// 					Subscribe: &gnmi.SubscriptionList{
// 						Subscription: []*gnmi.Subscription{
// 							{
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{{
// 										Name: "interface",
// 									}},
// 								},
// 							},
// 						},
// 						Mode:     gnmi.SubscriptionList_ONCE,
// 						Encoding: gnmi.Encoding_JSON_IETF,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 		{
// 			name: "encoding_from_global",
// 			fields: fields{
// 				GlobalFlags: GlobalFlags{Encoding: "json_ietf"},
// 			},
// 			args: args{
// 				sc: &types.SubscriptionConfig{
// 					Paths: []string{
// 						"interface",
// 					},
// 					Mode: "once",
// 				},
// 			},
// 			want: &gnmi.SubscribeRequest{
// 				Request: &gnmi.SubscribeRequest_Subscribe{
// 					Subscribe: &gnmi.SubscriptionList{
// 						Subscription: []*gnmi.Subscription{
// 							{
// 								Path: &gnmi.Path{
// 									Elem: []*gnmi.PathElem{{
// 										Name: "interface",
// 									}},
// 								},
// 							},
// 						},
// 						Mode:     gnmi.SubscriptionList_ONCE,
// 						Encoding: gnmi.Encoding_JSON_IETF,
// 					},
// 				},
// 			},
// 			wantErr: false,
// 		},
// 	}
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			c := &Config{
// 				GlobalFlags:        tt.fields.GlobalFlags,
// 				LocalFlags:         tt.fields.LocalFlags,
// 				FileConfig:         tt.fields.FileConfig,
// 				Targets:            tt.fields.Targets,
// 				Subscriptions:      tt.fields.Subscriptions,
// 				Outputs:            tt.fields.Outputs,
// 				Inputs:             tt.fields.Inputs,
// 				Processors:         tt.fields.Processors,
// 				Clustering:         tt.fields.Clustering,
// 				GnmiServer:         tt.fields.GnmiServer,
// 				APIServer:          tt.fields.APIServer,
// 				Loader:             tt.fields.Loader,
// 				Actions:            tt.fields.Actions,
// 				logger:             tt.fields.logger,
// 				setRequestTemplate: tt.fields.setRequestTemplate,
// 				setRequestVars:     tt.fields.setRequestVars,
// 			}
// 			got, err := c.CreateSubscribeRequest(tt.args.sc, tt.args.target)
// 			if err != nil && tt.wantErr {
// 				t.Logf("expected error: %v", err)
// 				return
// 			}
// 			if (err != nil) != tt.wantErr {
// 				t.Logf("Config.CreateSubscribeRequest() error   = %v", err)
// 				t.Logf("Config.CreateSubscribeRequest() wantErr = %v", tt.wantErr)
// 				t.Fail()
// 				return
// 			}
// 			t.Logf("got:\n%s", prototext.Format(got))
// 			if !testutils.SubscribeRequestsEqual(got, tt.want) {
// 				t.Logf("Config.CreateSubscribeRequest() got  = %v", got)
// 				t.Logf("Config.CreateSubscribeRequest() want = %v", tt.want)
// 				t.Fail()
// 			}
// 		})
// 	}
// }
