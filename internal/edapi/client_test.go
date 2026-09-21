package edapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSearchSystemAndCaching(t *testing.T) {
	var edsmCalls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&edsmCalls, 1)
		name := r.URL.Query().Get("systemName")
		if name == "Sol" {
			fmt.Fprint(w, `{"name":"Sol","coords":{"x":0,"y":0,"z":0},"information":{"allegiance":"Federation","population":18000000000}}`)
			return
		}
		fmt.Fprint(w, `[]`)
	}))
	defer ts.Close()

	client := NewClient(
		WithBaseEDSMURL(ts.URL),
		WithCacheTTL(1*time.Hour),
	)

	ctx := context.Background()

	// Call 1: Miss, fetches from server
	res1, err := client.SearchSystem(ctx, "Sol")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res1, `"name": "Sol"`) {
		t.Errorf("expected Sol in response, got %s", res1)
	}
	if atomic.LoadInt32(&edsmCalls) != 1 {
		t.Errorf("expected 1 call to EDSM, got %d", edsmCalls)
	}

	// Call 2: Hit, served from cache
	res2, err := client.SearchSystem(ctx, "Sol")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res1 != res2 {
		t.Errorf("expected cached response to match")
	}
	if atomic.LoadInt32(&edsmCalls) != 1 {
		t.Errorf("expected still 1 call to EDSM due to cache, got %d", edsmCalls)
	}

	// Call 3: Nonexistent system
	_, err = client.SearchSystem(ctx, "UnknownSystem")
	if err == nil {
		t.Fatalf("expected error for unknown system")
	}
}

func TestNearestSystemsAndFiltering(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[
			{"distance":4.38,"name":"Alpha Centauri","coords":{"x":3,"y":0,"z":3},"information":{"population":100000}},
			{"distance":5.0,"name":"Empty System","coords":{"x":2,"y":2,"z":2},"information":{"population":0}},
			{"distance":6.0,"name":"Deep Space","coords":{"x":1,"y":1,"z":1},"information":{}}
		]`)
	}))
	defer ts.Close()

	client := NewClient(WithBaseEDSMURL(ts.URL))
	ctx := context.Background()

	// All systems
	resAll, err := client.NearestSystems(ctx, "Sol", nil, nil, nil, 10, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resAll, "Alpha Centauri") || !strings.Contains(resAll, "Empty System") {
		t.Errorf("expected all systems in result, got: %s", resAll)
	}

	// Only populated
	resPop, err := client.NearestSystems(ctx, "Sol", nil, nil, nil, 10, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(resPop, "Alpha Centauri") {
		t.Errorf("expected Alpha Centauri in result, got: %s", resPop)
	}
	if strings.Contains(resPop, "Empty System") || strings.Contains(resPop, "Deep Space") {
		t.Errorf("expected unpopulated systems to be filtered out, got: %s", resPop)
	}
}

func TestSystemStationsAndMarketFiltering(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/stations/market") {
			fmt.Fprint(w, `{
				"id": 1,
				"name": "Daedalus",
				"marketId": 1234,
				"commodities": [
					{"id":"tritium","name":"Tritium","buyPrice":50000,"stock":100,"sellPrice":49000,"demand":1},
					{"id":"gold","name":"Gold","buyPrice":45000,"stock":50,"sellPrice":44000,"demand":1}
				]
			}`)
			return
		}
		if strings.Contains(r.URL.Path, "/stations") {
			fmt.Fprint(w, `{"name":"Sol","stations":[{"name":"Daedalus","type":"Coriolis Starport","haveMarket":true}]}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	client := NewClient(WithBaseEDSMURL(ts.URL))
	ctx := context.Background()

	stations, err := client.SystemStations(ctx, "Sol")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stations, "Daedalus") {
		t.Errorf("expected Daedalus station, got %s", stations)
	}

	// Filter market for Tritium
	market, err := client.StationMarket(ctx, "Sol", "Daedalus", "tritium")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(market, "Tritium") {
		t.Errorf("expected Tritium in filtered market, got %s", market)
	}
	if strings.Contains(market, "Gold") {
		t.Errorf("expected Gold to be filtered out, got %s", market)
	}
}

func TestPlotNeutronRouteSpansh(t *testing.T) {
	var pollCount int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/route") {
			fmt.Fprint(w, `{"job":"job-123","status":"queued"}`)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/results/job-123") {
			cnt := atomic.AddInt32(&pollCount, 1)
			if cnt == 1 {
				fmt.Fprint(w, `{"status":"queued","result":null}`)
				return
			}
			fmt.Fprint(w, `{"status":"completed","result":{"distance":22000,"system_jumps":[{"system":"Sol"},{"system":"Colonia"}]}}`)
			return
		}
		http.NotFound(w, r)
	}))
	defer ts.Close()

	client := NewClient(WithBaseSpanshURL(ts.URL))
	ctx := context.Background()

	route, err := client.PlotNeutronRoute(ctx, "Sol", "Colonia", 50, 60)
	if err != nil {
		t.Fatalf("unexpected error plotting route: %v", err)
	}
	if !strings.Contains(route, "Colonia") {
		t.Errorf("expected Colonia in result, got: %s", route)
	}
}
