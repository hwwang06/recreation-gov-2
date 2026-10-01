package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"github.com/adhocteam/recreation-mcp-server/internal/cache"
	"github.com/adhocteam/recreation-mcp-server/internal/models"
	"github.com/adhocteam/recreation-mcp-server/pkg/util"
)

// WeatherClient defines the interface for the OpenWeatherMap API client
type WeatherClient interface {
	GetCurrentWeather(ctx context.Context, lat, lon float64, units string) (*models.Weather, error)
}

// weatherClient implements the WeatherClient interface
type weatherClient struct {
	baseURL    string
	apiKey     string
	httpClient *util.HTTPClient
	cache      *cache.Cache
	logger     *util.Logger
}

// NewWeatherClient creates a new OpenWeatherMap API client
func NewWeatherClient(baseURL, apiKey string, httpClient *util.HTTPClient, cache *cache.Cache, logger *util.Logger) WeatherClient {
	return &weatherClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: httpClient,
		cache:      cache,
		logger:     logger,
	}
}

// GetCurrentWeather retrieves current weather conditions for a location
func (c *weatherClient) GetCurrentWeather(ctx context.Context, lat, lon float64, units string) (*models.Weather, error) {
	c.logger.Debug("Getting current weather", "lat", lat, "lon", lon, "units", units)

	if units == "" {
		units = "imperial"
	}

	endpoint := fmt.Sprintf("%s/weather", c.baseURL)
	queryParams := url.Values{}
	queryParams.Add("lat", strconv.FormatFloat(lat, 'f', 6, 64))
	queryParams.Add("lon", strconv.FormatFloat(lon, 'f', 6, 64))
	queryParams.Add("units", units)
	queryParams.Add("appid", c.apiKey)
	fullURL := fmt.Sprintf("%s?%s", endpoint, queryParams.Encode())

	cacheKey := cache.GenerateKey("weather:current", map[string]string{
		"lat":   strconv.FormatFloat(lat, 'f', 6, 64),
		"lon":   strconv.FormatFloat(lon, 'f', 6, 64),
		"units": units,
	})

	if cached, found := c.cache.Get(cacheKey); found {
		c.logger.Debug("Cache hit for current weather")
		var weather models.Weather
		if err := json.Unmarshal(cached, &weather); err == nil {
			return &weather, nil
		}
	}

	resp, err := c.httpClient.Get(ctx, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get current weather: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := util.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var weatherResp struct {
		Main struct {
			Temp      float64 `json:"temp"`
			FeelsLike float64 `json:"feels_like"`
			TempMin   float64 `json:"temp_min"`
			TempMax   float64 `json:"temp_max"`
			Pressure  int     `json:"pressure"`
			Humidity  int     `json:"humidity"`
		} `json:"main"`
		Weather []struct {
			Main        string `json:"main"`
			Description string `json:"description"`
		} `json:"weather"`
		Wind struct {
			Speed float64 `json:"speed"`
			Deg   int     `json:"deg"`
		} `json:"wind"`
		Clouds struct {
			All int `json:"all"`
		} `json:"clouds"`
		Visibility int `json:"visibility"`
		Sys        struct {
			Sunrise int64 `json:"sunrise"`
			Sunset  int64 `json:"sunset"`
		} `json:"sys"`
		Timezone int `json:"timezone"`
	}

	if err := json.Unmarshal(body, &weatherResp); err != nil {
		return nil, fmt.Errorf("failed to parse weather response: %w", err)
	}

	conditions := ""
	description := ""
	if len(weatherResp.Weather) > 0 {
		conditions = weatherResp.Weather[0].Main
		description = weatherResp.Weather[0].Description
	}

	weather := &models.Weather{
		Temperature:   weatherResp.Main.Temp,
		FeelsLike:     weatherResp.Main.FeelsLike,
		TempMin:       weatherResp.Main.TempMin,
		TempMax:       weatherResp.Main.TempMax,
		Pressure:      weatherResp.Main.Pressure,
		Humidity:      weatherResp.Main.Humidity,
		Conditions:    conditions,
		Description:   description,
		WindSpeed:     weatherResp.Wind.Speed,
		WindDirection: weatherResp.Wind.Deg,
		Clouds:        weatherResp.Clouds.All,
		Visibility:    weatherResp.Visibility,
		Sunrise:       weatherResp.Sys.Sunrise,
		Sunset:        weatherResp.Sys.Sunset,
		Timezone:      weatherResp.Timezone,
	}

	if cacheData, err := json.Marshal(weather); err == nil {
		_ = c.cache.Set(cacheKey, cacheData)
	}

	c.logger.Info("Current weather retrieved", "lat", lat, "lon", lon)
	return weather, nil
}
