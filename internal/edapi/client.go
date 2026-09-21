package edapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Cacher defines persistent or in-memory caching for external API calls.
type Cacher interface {
	GetAPICache(ctx context.Context, key string) (string, bool, error)
	SetAPICache(ctx context.Context, key, response string, ttl time.Duration) error
}

// memoryCache provides a fallback in-memory cache when persistent store is unavailable.
type memoryCache struct {
	mu    sync.RWMutex
	items map[string]memItem
}

type memItem struct {
	data      string
	expiresAt time.Time
}

func newMemoryCache() *memoryCache {
	return &memoryCache{items: make(map[string]memItem)}
}

func (m *memoryCache) GetAPICache(_ context.Context, key string) (string, bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	item, ok := m.items[key]
	if !ok || time.Now().UTC().After(item.expiresAt) {
		return "", false, nil
	}
	return item.data, true, nil
}

func (m *memoryCache) SetAPICache(_ context.Context, key, response string, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[key] = memItem{
		data:      response,
		expiresAt: time.Now().UTC().Add(ttl),
	}
	return nil
}

// Option configures Client.
type Option func(*Client)

// WithBaseEDSMURL overrides the default EDSM base URL.
func WithBaseEDSMURL(url string) Option {
	return func(c *Client) {
		if url != "" {
			c.edsmURL = strings.TrimRight(url, "/")
		}
	}
}

// WithBaseSpanshURL overrides the default Spansh base URL.
func WithBaseSpanshURL(url string) Option {
	return func(c *Client) {
		if url != "" {
			c.spanshURL = strings.TrimRight(url, "/")
		}
	}
}

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// WithCacher sets the cache implementation.
func WithCacher(cacher Cacher) Option {
	return func(c *Client) {
		if cacher != nil {
			c.cacher = cacher
		}
	}
}

// WithCacheTTL sets default cache duration.
func WithCacheTTL(ttl time.Duration) Option {
	return func(c *Client) {
		if ttl > 0 {
			c.cacheTTL = ttl
		}
	}
}

// Client interacts with external Elite Dangerous APIs (EDSM, Spansh) with caching.
type Client struct {
	httpClient *http.Client
	cacher     Cacher
	edsmURL    string
	spanshURL  string
	cacheTTL   time.Duration
}

// NewClient creates a new API client with 8h default TTL.
func NewClient(opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		cacher:     newMemoryCache(),
		edsmURL:    "https://www.edsm.net",
		spanshURL:  "https://spansh.co.uk",
		cacheTTL:   8 * time.Hour,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// SystemCoordinates represents 3D galactic coordinates.
type SystemCoordinates struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// SystemInfo represents detailed system information from EDSM.
type SystemInfo struct {
	Name         string            `json:"name"`
	Coords       SystemCoordinates `json:"coords"`
	CoordsLocked bool              `json:"coordsLocked,omitempty"`
	Information  struct {
		Allegiance    string  `json:"allegiance,omitempty"`
		Government    string  `json:"government,omitempty"`
		Faction       string  `json:"faction,omitempty"`
		FactionState  string  `json:"factionState,omitempty"`
		Population    *int64  `json:"population,omitempty"`
		Security      string  `json:"security,omitempty"`
		Economy       string  `json:"economy,omitempty"`
		SecondEconomy string  `json:"secondEconomy,omitempty"`
		Reserve       *string `json:"reserve,omitempty"`
	} `json:"information"`
	PrimaryStar *struct {
		Type string `json:"type,omitempty"`
		Name string `json:"name,omitempty"`
	} `json:"primaryStar,omitempty"`
}

// SearchSystem fetches info for a named system with caching.
func (c *Client) SearchSystem(ctx context.Context, systemName string) (string, error) {
	systemName = strings.TrimSpace(systemName)
	if systemName == "" {
		return "", errors.New("systemName is required")
	}

	cacheKey := fmt.Sprintf("edsm:system:%s", strings.ToLower(systemName))
	if cached, ok, _ := c.cacher.GetAPICache(ctx, cacheKey); ok {
		return cached, nil
	}

	endpoint := fmt.Sprintf("%s/api-v1/system?systemName=%s&showCoordinates=1&showInformation=1&showPrimaryStar=1",
		c.edsmURL, url.QueryEscape(systemName))

	body, err := c.doGet(ctx, endpoint)
	if err != nil {
		return "", err
	}

	// EDSM returns "[]" when a system is not found
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "[]" || trimmed == "{}" || trimmed == "" {
		return "", fmt.Errorf("system '%s' not found in EDSM database", systemName)
	}

	// Validate JSON
	var parsed SystemInfo
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("failed parsing EDSM system response: %w", err)
	}

	formatted, _ := json.MarshalIndent(parsed, "", "  ")
	res := string(formatted)
	_ = c.cacher.SetAPICache(ctx, cacheKey, res, c.cacheTTL)
	return res, nil
}

// NearestSystems queries systems within radius (ly) of a system or coordinates.
func (c *Client) NearestSystems(ctx context.Context, systemName string, x, y, z *float64, radius float64, onlyPopulated bool) (string, error) {
	if radius <= 0 {
		radius = 25
	}
	if radius > 100 {
		radius = 100
	}

	var endpoint string
	var cacheKey string

	systemName = strings.TrimSpace(systemName)
	if systemName != "" {
		cacheKey = fmt.Sprintf("edsm:sphere:name:%s:rad:%.1f:pop:%t", strings.ToLower(systemName), radius, onlyPopulated)
		endpoint = fmt.Sprintf("%s/api-v1/sphere-systems?systemName=%s&radius=%.1f&showCoordinates=1&showInformation=1",
			c.edsmURL, url.QueryEscape(systemName), radius)
	} else if x != nil && y != nil && z != nil {
		cacheKey = fmt.Sprintf("edsm:sphere:coord:%.2f_%.2f_%.2f:rad:%.1f:pop:%t", *x, *y, *z, radius, onlyPopulated)
		endpoint = fmt.Sprintf("%s/api-v1/sphere-systems?x=%.2f&y=%.2f&z=%.2f&radius=%.1f&showCoordinates=1&showInformation=1",
			c.edsmURL, *x, *y, *z, radius)
	} else {
		return "", errors.New("either systemName or (x, y, z) coordinates must be provided")
	}

	if cached, ok, _ := c.cacher.GetAPICache(ctx, cacheKey); ok {
		return cached, nil
	}

	body, err := c.doGet(ctx, endpoint)
	if err != nil {
		return "", err
	}

	type SphereItem struct {
		Distance    float64           `json:"distance"`
		BodyCount   int               `json:"bodyCount,omitempty"`
		Name        string            `json:"name"`
		Coords      SystemCoordinates `json:"coords"`
		Information map[string]any    `json:"information,omitempty"`
	}

	var rawList []SphereItem
	if err := json.Unmarshal(body, &rawList); err != nil {
		return "", fmt.Errorf("failed parsing sphere systems: %w", err)
	}

	var filtered []SphereItem
	for _, item := range rawList {
		if onlyPopulated {
			popVal, hasPop := item.Information["population"]
			if !hasPop || popVal == nil {
				continue
			}
			if num, ok := popVal.(float64); !ok || num <= 0 {
				continue
			}
		}
		filtered = append(filtered, item)
	}

	outBytes, _ := json.MarshalIndent(filtered, "", "  ")
	res := string(outBytes)
	_ = c.cacher.SetAPICache(ctx, cacheKey, res, c.cacheTTL)
	return res, nil
}

// SystemStations lists stations in a system.
func (c *Client) SystemStations(ctx context.Context, systemName string) (string, error) {
	systemName = strings.TrimSpace(systemName)
	if systemName == "" {
		return "", errors.New("systemName is required")
	}

	cacheKey := fmt.Sprintf("edsm:stations:%s", strings.ToLower(systemName))
	if cached, ok, _ := c.cacher.GetAPICache(ctx, cacheKey); ok {
		return cached, nil
	}

	endpoint := fmt.Sprintf("%s/api-system-v1/stations?systemName=%s", c.edsmURL, url.QueryEscape(systemName))
	body, err := c.doGet(ctx, endpoint)
	if err != nil {
		return "", err
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "{}" || trimmed == "[]" || trimmed == "" {
		return "", fmt.Errorf("no stations found for system '%s'", systemName)
	}

	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", fmt.Errorf("failed parsing stations response: %w", err)
	}

	outBytes, _ := json.MarshalIndent(raw, "", "  ")
	res := string(outBytes)
	_ = c.cacher.SetAPICache(ctx, cacheKey, res, c.cacheTTL)
	return res, nil
}

// StationMarket returns market commodity prices at a station, optionally filtered by commodity.
func (c *Client) StationMarket(ctx context.Context, systemName, stationName, filterCommodity string) (string, error) {
	systemName = strings.TrimSpace(systemName)
	stationName = strings.TrimSpace(stationName)
	filterCommodity = strings.TrimSpace(filterCommodity)

	if systemName == "" || stationName == "" {
		return "", errors.New("both systemName and stationName are required")
	}

	cacheKey := fmt.Sprintf("edsm:market:%s:%s", strings.ToLower(systemName), strings.ToLower(stationName))
	var rawData string
	if cached, ok, _ := c.cacher.GetAPICache(ctx, cacheKey); ok {
		rawData = cached
	} else {
		endpoint := fmt.Sprintf("%s/api-system-v1/stations/market?systemName=%s&stationName=%s",
			c.edsmURL, url.QueryEscape(systemName), url.QueryEscape(stationName))
		body, err := c.doGet(ctx, endpoint)
		if err != nil {
			return "", err
		}
		trimmed := strings.TrimSpace(string(body))
		if trimmed == "{}" || trimmed == "[]" || trimmed == "" {
			return "", fmt.Errorf("no market data found for station '%s' in system '%s'", stationName, systemName)
		}
		rawData = trimmed
		// Market data can cache for 1 hour or standard cacheTTL
		_ = c.cacher.SetAPICache(ctx, cacheKey, rawData, c.cacheTTL)
	}

	type MarketData struct {
		ID          int64  `json:"id"`
		Name        string `json:"name"`
		MarketID    int64  `json:"marketId"`
		Commodities []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			BuyPrice     int64  `json:"buyPrice"`
			Stock        int64  `json:"stock"`
			SellPrice    int64  `json:"sellPrice"`
			Demand       int64  `json:"demand"`
			StockBracket int    `json:"stockBracket"`
		} `json:"commodities"`
	}

	var mkt MarketData
	if err := json.Unmarshal([]byte(rawData), &mkt); err != nil {
		return rawData, nil
	}

	if filterCommodity != "" {
		filterLower := strings.ToLower(filterCommodity)
		var matched []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			BuyPrice     int64  `json:"buyPrice"`
			Stock        int64  `json:"stock"`
			SellPrice    int64  `json:"sellPrice"`
			Demand       int64  `json:"demand"`
			StockBracket int    `json:"stockBracket"`
		}
		for _, c := range mkt.Commodities {
			if strings.Contains(strings.ToLower(c.Name), filterLower) || strings.Contains(strings.ToLower(c.ID), filterLower) {
				matched = append(matched, c)
			}
		}
		mkt.Commodities = matched
	}

	outBytes, _ := json.MarshalIndent(mkt, "", "  ")
	return string(outBytes), nil
}

// SystemFactions returns factions and BGS state in a system.
func (c *Client) SystemFactions(ctx context.Context, systemName string) (string, error) {
	systemName = strings.TrimSpace(systemName)
	if systemName == "" {
		return "", errors.New("systemName is required")
	}

	cacheKey := fmt.Sprintf("edsm:factions:%s", strings.ToLower(systemName))
	if cached, ok, _ := c.cacher.GetAPICache(ctx, cacheKey); ok {
		return cached, nil
	}

	endpoint := fmt.Sprintf("%s/api-system-v1/factions?systemName=%s", c.edsmURL, url.QueryEscape(systemName))
	body, err := c.doGet(ctx, endpoint)
	if err != nil {
		return "", err
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "{}" || trimmed == "[]" || trimmed == "" {
		return "", fmt.Errorf("no faction data found for system '%s'", systemName)
	}

	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", fmt.Errorf("failed parsing factions response: %w", err)
	}

	outBytes, _ := json.MarshalIndent(raw, "", "  ")
	res := string(outBytes)
	_ = c.cacher.SetAPICache(ctx, cacheKey, res, c.cacheTTL)
	return res, nil
}

// SystemBodies returns celestial bodies in a system.
func (c *Client) SystemBodies(ctx context.Context, systemName string) (string, error) {
	systemName = strings.TrimSpace(systemName)
	if systemName == "" {
		return "", errors.New("systemName is required")
	}

	cacheKey := fmt.Sprintf("edsm:bodies:%s", strings.ToLower(systemName))
	if cached, ok, _ := c.cacher.GetAPICache(ctx, cacheKey); ok {
		return cached, nil
	}

	endpoint := fmt.Sprintf("%s/api-system-v1/bodies?systemName=%s", c.edsmURL, url.QueryEscape(systemName))
	body, err := c.doGet(ctx, endpoint)
	if err != nil {
		return "", err
	}

	trimmed := strings.TrimSpace(string(body))
	if trimmed == "{}" || trimmed == "[]" || trimmed == "" {
		return "", fmt.Errorf("no celestial bodies found for system '%s'", systemName)
	}

	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", fmt.Errorf("failed parsing bodies response: %w", err)
	}

	outBytes, _ := json.MarshalIndent(raw, "", "  ")
	res := string(outBytes)
	_ = c.cacher.SetAPICache(ctx, cacheKey, res, c.cacheTTL)
	return res, nil
}

// ServerStatus returns current Elite Dangerous server status.
func (c *Client) ServerStatus(ctx context.Context) (string, error) {
	cacheKey := "edsm:server_status"
	// Server status cached for 10 minutes max
	if cached, ok, _ := c.cacher.GetAPICache(ctx, cacheKey); ok {
		return cached, nil
	}

	endpoint := fmt.Sprintf("%s/api-status-v1/elite-server", c.edsmURL)
	body, err := c.doGet(ctx, endpoint)
	if err != nil {
		return "", err
	}

	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return "", fmt.Errorf("failed parsing server status: %w", err)
	}

	outBytes, _ := json.MarshalIndent(raw, "", "  ")
	res := string(outBytes)
	_ = c.cacher.SetAPICache(ctx, cacheKey, res, 10*time.Minute)
	return res, nil
}

// PlotNeutronRoute plans a neutron highway jump route via Spansh.
func (c *Client) PlotNeutronRoute(ctx context.Context, from, to string, jumpRange float64, efficiency int) (string, error) {
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)
	if from == "" || to == "" {
		return "", errors.New("both 'from' and 'to' systems are required")
	}
	if jumpRange <= 0 {
		jumpRange = 50.0
	}
	if efficiency <= 0 || efficiency > 100 {
		efficiency = 60
	}

	cacheKey := fmt.Sprintf("spansh:route:%s:%s:%.1f:%d", strings.ToLower(from), strings.ToLower(to), jumpRange, efficiency)
	if cached, ok, _ := c.cacher.GetAPICache(ctx, cacheKey); ok {
		return cached, nil
	}

	// 1. Submit route planning job
	endpoint := fmt.Sprintf("%s/api/route?from=%s&to=%s&range=%.1f&efficiency=%d",
		c.spanshURL, url.QueryEscape(from), url.QueryEscape(to), jumpRange, efficiency)

	jobRespBytes, err := c.doGet(ctx, endpoint)
	if err != nil {
		return "", fmt.Errorf("failed requesting Spansh route: %w", err)
	}

	var jobResp struct {
		Job   string `json:"job"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(jobRespBytes, &jobResp); err != nil {
		return "", fmt.Errorf("failed parsing Spansh job response: %w", err)
	}
	if jobResp.Error != "" {
		return "", fmt.Errorf("spansh route error: %s", jobResp.Error)
	}
	if jobResp.Job == "" {
		return "", errors.New("spansh returned no job ID")
	}

	// 2. Poll for job completion (up to 15 seconds)
	resultsURL := fmt.Sprintf("%s/api/results/%s", c.spanshURL, jobResp.Job)
	deadline := time.Now().Add(15 * time.Second)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}

		resBody, err := c.doGet(ctx, resultsURL)
		if err != nil {
			continue
		}

		var pollResp struct {
			Status string          `json:"status"`
			Result json.RawMessage `json:"result"`
			Error  string          `json:"error"`
		}
		if err := json.Unmarshal(resBody, &pollResp); err == nil {
			if pollResp.Error != "" {
				return "", fmt.Errorf("spansh route computation failed: %s", pollResp.Error)
			}
			if len(pollResp.Result) > 0 && string(pollResp.Result) != "null" {
				var formatted any
				_ = json.Unmarshal(pollResp.Result, &formatted)
				outBytes, _ := json.MarshalIndent(formatted, "", "  ")
				res := string(outBytes)
				_ = c.cacher.SetAPICache(ctx, cacheKey, res, c.cacheTTL)
				return res, nil
			}
		}
	}

	return "", fmt.Errorf("neutron route planning timed out after 15s (job: %s)", jobResp.Job)
}

func (c *Client) doGet(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ed-assist/1.0.0 (Elite Dangerous Assistant)")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("external API request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("resource not found (HTTP 404)")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("external API returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}
