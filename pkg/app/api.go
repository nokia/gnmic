// © 2022 Nokia.
//
// This code is a Contribution to the gNMIc project (“Work”) made under the Google Software Grant and Corporate Contributor License Agreement (“CLA”) and governed by the Apache License 2.0.
// No other rights or licenses in or to any of Nokia’s intellectual property are granted for any other purpose.
// This code is provided on an “as is” basis without any warranties of any kind.
//
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/AlekSi/pointer"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/openconfig/gnmic/pkg/api/types"
	"github.com/openconfig/gnmic/pkg/api/utils"
	"github.com/openconfig/gnmic/pkg/config"
)

func (a *App) newAPIServer() (*http.Server, error) {
	a.routes()
	var tlscfg *tls.Config
	var err error
	if a.Config.APIServer.TLS != nil {
		tlscfg, err = utils.NewTLSConfig(
			a.Config.APIServer.TLS.CaFile,
			a.Config.APIServer.TLS.CertFile,
			a.Config.APIServer.TLS.KeyFile,
			a.Config.APIServer.TLS.ClientAuth,
			false, // skip-verify
			true,  // genSelfSigned
		)
		if err != nil {
			return nil, err
		}
	}

	if a.Config.APIServer.EnableMetrics {
		a.router.Handle("/metrics", promhttp.HandlerFor(a.reg, promhttp.HandlerOpts{}))
		a.reg.MustRegister(collectors.NewGoCollector())
		a.reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
		a.reg.MustRegister(subscribeResponseReceivedCounter)
		a.reg.MustRegister(subscribeResponseFailedCounter)
		a.registerTargetMetrics()
		go a.startClusterMetrics()
	}
	s := &http.Server{
		Addr:         a.Config.APIServer.Address,
		Handler:      a.router,
		ReadTimeout:  a.Config.APIServer.Timeout / 2,
		WriteTimeout: a.Config.APIServer.Timeout / 2,
	}

	if tlscfg != nil {
		s.TLSConfig = tlscfg
	}

	return s, nil
}

type APIErrors struct {
	Errors []string `json:"errors,omitempty"`
}

func (a *App) handleConfigTargetsGet(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	var err error
	a.configLock.RLock()
	defer a.configLock.RUnlock()
	if id == "" {
		// copy targets map
		targets := make(map[string]*types.TargetConfig, len(a.Config.Targets))
		for n, tc := range a.Config.Targets {
			ntc := tc.DeepCopy()
			ntc.Password = pointer.ToString("****")
			targets[n] = ntc
		}
		err = json.NewEncoder(w).Encode(targets)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		}
		return
	}
	if t, ok := a.Config.Targets[id]; ok {
		tc := t.DeepCopy()
		tc.Password = pointer.ToString("****")
		err = json.NewEncoder(w).Encode(tc)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		}
		return
	}
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(APIErrors{Errors: []string{fmt.Sprintf("target %q not found", id)}})
}

func (a *App) handleConfigTargetsPost(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}
	defer r.Body.Close()
	tc := new(types.TargetConfig)
	err = json.Unmarshal(body, tc)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}
	a.AddTargetConfig(tc)
}

func (a *App) handleConfigTargetsSubscriptions(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	if !a.targetConfigExists(id) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{fmt.Sprintf("target %q not found", id)}})
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}
	defer r.Body.Close()

	var data map[string][]string
	err = json.Unmarshal(body, &data)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}
	subs, ok := data["subscriptions"]
	if !ok {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{"subscriptions not found"}})
		return
	}
	err = a.UpdateTargetSubscription(a.ctx, id, subs)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}
}

func (a *App) handleConfigTargetsDelete(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	err := a.DeleteTarget(r.Context(), id)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}
}

func (a *App) handleConfigSubscriptions(w http.ResponseWriter, r *http.Request) {
	a.handlerCommonGet(w, a.Config.Subscriptions)
}

func (a *App) handleConfigOutputs(w http.ResponseWriter, r *http.Request) {
	a.handlerCommonGet(w, a.Config.Outputs)
}

func (a *App) handleConfigClustering(w http.ResponseWriter, r *http.Request) {
	a.handlerCommonGet(w, a.Config.Clustering)
}

func (a *App) handleConfigAPIServer(w http.ResponseWriter, r *http.Request) {
	a.handlerCommonGet(w, a.Config.APIServer)
}

func (a *App) handleConfigGNMIServer(w http.ResponseWriter, r *http.Request) {
	a.handlerCommonGet(w, a.Config.GnmiServer)
}

func (a *App) handleConfigInputs(w http.ResponseWriter, r *http.Request) {
	a.handlerCommonGet(w, a.Config.Inputs)
}

func (a *App) handleConfigProcessors(w http.ResponseWriter, r *http.Request) {
	a.handlerCommonGet(w, a.Config.Processors)
}

func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	nc := &config.Config{
		GlobalFlags:   a.Config.GlobalFlags,
		LocalFlags:    a.Config.LocalFlags,
		FileConfig:    a.Config.FileConfig,
		Targets:       make(map[string]*types.TargetConfig, len(a.Config.Targets)),
		Subscriptions: a.Config.Subscriptions,
		Outputs:       a.Config.Outputs,
		Inputs:        a.Config.Inputs,
		Processors:    a.Config.Processors,
		Clustering:    a.Config.Clustering,
		GnmiServer:    a.Config.GnmiServer,
		APIServer:     a.Config.APIServer,
		Loader:        a.Config.Loader,
		Actions:       a.Config.Actions,
		TunnelServer:  a.Config.TunnelServer,
	}
	for n, t := range a.Config.Targets {
		tc := t.DeepCopy()
		tc.Password = pointer.ToString("****")
		nc.Targets[n] = tc
	}
	a.handlerCommonGet(w, nc)
}

func (a *App) handleTargetsGet(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	if id == "" {
		a.handlerCommonGet(w, a.Targets)
		return
	}
	if t, ok := a.Targets[id]; ok {
		a.handlerCommonGet(w, t)
		return
	}
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(APIErrors{Errors: []string{"no targets found"}})
}

func (a *App) handleTargetsPost(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	tc, ok := a.Config.Targets[id]
	if !ok {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{fmt.Sprintf("target %q not found", id)}})
		return
	}
	go a.TargetSubscribeStream(a.ctx, tc)
}

func (a *App) handleTargetsDelete(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if _, ok := a.Targets[id]; !ok {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{fmt.Sprintf("target %q not found", id)}})
		return
	}
	err := a.DeleteTarget(a.ctx, id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}
}

type clusteringResponse struct {
	ClusterName           string          `json:"name,omitempty"`
	NumberOfLockedTargets int             `json:"number-of-locked-targets"`
	Leader                string          `json:"leader,omitempty"`
	Members               []clusterMember `json:"members,omitempty"`
}

type clusterMember struct {
	Name                  string   `json:"name,omitempty"`
	APIEndpoint           string   `json:"api-endpoint,omitempty"`
	IsLeader              bool     `json:"is-leader,omitempty"`
	NumberOfLockedTargets int      `json:"number-of-locked-nodes"`
	LockedTargets         []string `json:"locked-targets,omitempty"`
}

func (a *App) handleClusteringGet(w http.ResponseWriter, r *http.Request) {
	if a.Config.Clustering == nil {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	resp := new(clusteringResponse)
	resp.ClusterName = a.Config.ClusterName

	var err error
	resp.Leader, err = a.getLeaderName(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}

	services, err := a.locker.GetServices(ctx, fmt.Sprintf("%s-gnmic-api", a.Config.ClusterName), nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}

	instanceNodes, err := a.getInstanceToTargetsMapping(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}

	for _, v := range instanceNodes {
		resp.NumberOfLockedTargets += len(v)
	}

	resp.Members = make([]clusterMember, len(services))
	for i, s := range services {
		scheme := "http://"
		for _, t := range s.Tags {
			if strings.HasPrefix(t, "protocol=") {
				scheme = fmt.Sprintf("%s://", strings.TrimPrefix(t, "protocol="))
			}
		}
		resp.Members[i].APIEndpoint = fmt.Sprintf("%s%s", scheme, s.Address)
		resp.Members[i].Name = strings.TrimSuffix(s.ID, "-api")
		resp.Members[i].IsLeader = resp.Leader == resp.Members[i].Name
		resp.Members[i].NumberOfLockedTargets = len(instanceNodes[resp.Members[i].Name])
		resp.Members[i].LockedTargets = instanceNodes[resp.Members[i].Name]
	}
	b, err := json.Marshal(resp)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}
	w.Write(b)
}

func (a *App) handleHealthzGet(w http.ResponseWriter, r *http.Request) {
	s := map[string]string{"status": "healthy"}
	b, err := json.Marshal(s)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}
	w.Write(b)
}

func (a *App) handleAdminShutdown(w http.ResponseWriter, r *http.Request) {
	a.Logger.Printf("shutting down due to user request")
	a.Cfn()
}

func (a *App) handleClusteringMembersGet(w http.ResponseWriter, r *http.Request) {
	if a.Config.Clustering == nil {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	// get leader
	leader, err := a.getLeaderName(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}

	services, err := a.locker.GetServices(ctx, fmt.Sprintf("%s-gnmic-api", a.Config.ClusterName), nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}

	instanceNodes, err := a.getInstanceToTargetsMapping(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}
	members := make([]clusterMember, len(services))
	for i, s := range services {
		scheme := "http://"
		for _, t := range s.Tags {
			if strings.HasPrefix(t, "protocol=") {
				scheme = fmt.Sprintf("%s://", strings.TrimPrefix(t, "protocol="))
			}
		}
		members[i].APIEndpoint = fmt.Sprintf("%s%s", scheme, s.Address)
		members[i].Name = strings.TrimSuffix(s.ID, "-api")
		members[i].IsLeader = leader == members[i].Name
		members[i].NumberOfLockedTargets = len(instanceNodes[members[i].Name])
		members[i].LockedTargets = instanceNodes[members[i].Name]
	}
	b, err := json.Marshal(members)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}
	w.Write(b)
}

func (a *App) handleClusteringLeaderGet(w http.ResponseWriter, r *http.Request) {
	if a.Config.Clustering == nil {
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	// get leader
	leader, err := a.getLeaderName(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}

	services, err := a.locker.GetServices(ctx, fmt.Sprintf("%s-gnmic-api", a.Config.ClusterName), nil)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}

	instanceNodes, err := a.getInstanceToTargetsMapping(ctx)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}

	members := make([]clusterMember, 1)
	for _, s := range services {
		if strings.TrimSuffix(s.ID, "-api") != leader {
			continue
		}
		scheme := "http://"
		for _, t := range s.Tags {
			if strings.HasPrefix(t, "protocol=") {
				scheme = fmt.Sprintf("%s://", strings.TrimPrefix(t, "protocol="))
			}
		}
		// add the leader as a member then break from loop
		members[0].APIEndpoint = fmt.Sprintf("%s%s", scheme, s.Address)
		members[0].Name = strings.TrimSuffix(s.ID, "-api")
		members[0].IsLeader = true
		members[0].NumberOfLockedTargets = len(instanceNodes[members[0].Name])
		members[0].LockedTargets = instanceNodes[members[0].Name]
		break
	}
	b, err := json.Marshal(members)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}
	w.Write(b)
}

func (a *App) handleClusteringLeaderDelete(w http.ResponseWriter, r *http.Request) {
	if a.Config.Clustering == nil {
		return
	}

	if !a.isLeader {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{"not leader"}})
		return
	}

	err := a.locker.Unlock(r.Context(), a.leaderKey())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}
}

func (a *App) handleClusteringDrainInstance(w http.ResponseWriter, r *http.Request) {
	if a.Config.Clustering == nil {
		return
	}

	if !a.isLeader {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{"not leader"}})
		return
	}

	vars := mux.Vars(r)
	id := vars["id"]
	if id == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	services, err := a.locker.GetServices(ctx, fmt.Sprintf("%s-gnmic-api", a.Config.ClusterName),
		[]string{
			fmt.Sprintf("instance-name=%s", id),
		})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}
	if len(services) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{"unknown instance: " + id}})
		return
	}
	targets, err := a.getInstanceTargets(ctx, id)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}

	go func() {
		a.dispatchLock.Lock()
		defer a.dispatchLock.Unlock()

		for _, t := range targets {
			err = a.unassignTarget(a.ctx, t, services[0].ID)
			if err != nil {
				a.Logger.Printf("failed to unassign target %s: %v", t, err)
				continue
			}
			tc, ok := a.Config.Targets[t]
			if !ok {
				a.Logger.Printf("could not find target %s config", t)
				continue
			}
			err = a.dispatchTarget(a.ctx, tc, id+"-api")
			if err != nil {
				a.Logger.Printf("failed to dispatch target %s: %v", t, err)
				continue
			}
		}
	}()
}

func (a *App) handleClusterRebalance(w http.ResponseWriter, r *http.Request) {
	if a.Config.Clustering == nil {
		return
	}

	if !a.isLeader {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{"not leader"}})
		return
	}

	go func() {
		err := a.clusterRebalanceTargets()
		if err != nil {
			a.Logger.Printf("failed to rebalance: %v", err)
		}
	}()
}

// helpers
func headersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

func (a *App) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if (!a.Config.APIServer.HealthzDisableLogging && r.URL.Path == "/api/v1/healthz") || r.URL.Path != "/api/v1/healthz" {
			next = handlers.LoggingHandler(a.Logger.Writer(), next)
		}
		next.ServeHTTP(w, r)
	})
}

func (a *App) handlerCommonGet(w http.ResponseWriter, i interface{}) {
	a.configLock.RLock()
	defer a.configLock.RUnlock()
	b, err := json.Marshal(i)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(APIErrors{Errors: []string{err.Error()}})
		return
	}
	w.Write(b)
}

func (a *App) getLeaderName(ctx context.Context) (string, error) {
	leaderKey := fmt.Sprintf("gnmic/%s/leader", a.Config.ClusterName)
	leader, err := a.locker.List(ctx, leaderKey)
	if err != nil {
		return "", nil
	}
	return leader[leaderKey], nil
}

func (a *App) getInstanceTargets(ctx context.Context, instance string) ([]string, error) {
	locks, err := a.locker.List(ctx, fmt.Sprintf("gnmic/%s/targets", a.Config.Clustering.ClusterName))
	if err != nil {
		return nil, err
	}
	if a.Config.Debug {
		a.Logger.Println("current locks:", locks)
	}
	targets := make([]string, 0)
	for k, v := range locks {
		if v == instance {
			targets = append(targets, filepath.Base(k))
		}
	}
	sort.Strings(targets)
	return targets, nil
}
