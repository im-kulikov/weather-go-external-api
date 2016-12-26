package main

import (
  "net/http"
  "log"
  "encoding/json"
  "strings"
  "time"
  "flag"
)

type weatherProvider interface {
  temperature(city string) (float64, error) // in Kelvin, naturally
}

type openWeatherMap struct{
  apiKey string
}

func (w openWeatherMap) temperature(city string) (float64, error) {
  begin := time.Now()
  resp, err := http.Get("http://api.openweathermap.org/data/2.5/weather?APPID=" + w.apiKey + "&q=" + city)
  if err != nil {
    return 0, err
  }

  defer resp.Body.Close()

  var d struct {
    Main struct {
      Kelvin float64 `json:"temp"`
    } `json:"main"`
  }

  if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
    return 0, err
  }

  log.Printf("openWeatherMap: %s: %.2f, took: %s", city, d.Main.Kelvin, time.Since(begin).String())
  return d.Main.Kelvin, nil
}

type weatherUnderground struct {
  apiKey string
}

func (w weatherUnderground) temperature(city string) (float64, error) {
  begin := time.Now()
  resp, err := http.Get("http://api.wunderground.com/api/" + w.apiKey + "/conditions/q/" + city + ".json")
  if err != nil {
    return 0, err
  }

  defer resp.Body.Close()

  var d struct {
    Observation struct {
      Celsius float64 `json:"temp_c"`
    } `json:"current_observation"`
  }

  if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
    return 0, err
  }

  kelvin := d.Observation.Celsius + 273.15
  log.Printf("weatherUnderground: %s: %.2f, took: %s", city, kelvin, time.Since(begin).String())
  return kelvin, nil
}

func temperature(city string, providers ...weatherProvider) (float64, error) {
  sum := 0.0

  for _, provider := range providers {
    k, err := provider.temperature(city)
    if err != nil {
      return 0, err
    }

    sum += k
  }

  return sum / float64(len(providers)), nil
}

type multiWeatherProvider []weatherProvider

func (w multiWeatherProvider) temperature(city string) (float64, error) {
  // Make a channel for temperatures, and a channel for errors.
  // Each provider will push a value into only one.
  temps := make(chan float64, len(w))
  errs := make(chan error, len(w))

  // For each provider, spawn a goroutine with an anonymous function.
  // That function will invoke the temperature method, and forward the response.
  for _, provider := range w {
    go func(p weatherProvider) {
      k, err := p.temperature(city)
      if err != nil {
        errs <- err
        return
      }
      temps <- k
    }(provider)
  }

  sum := 0.0

  // Collect a temperature or an error from each provider.
  for i := 0; i < len(w); i++ {
    select {
    case temp := <-temps:
      sum += temp
    case err := <-errs:
      return 0, err
    }
  }

  // Return the average, same as before.
  return sum / float64(len(w)), nil
}

func main() {
  wundergroundAPIKey := flag.String("wunderground.api.key", "0123456789abcdef", "wunderground.com API key")
  openWeatherAPIKey := flag.String("openweather.api.key", "0123456789abcdef", "openweathermap.org API key")
	flag.Parse()

  log.Printf("wunderground apiKey: %s", *wundergroundAPIKey)
  log.Printf("openWeather apiKey: %s", *openWeatherAPIKey)

  http.HandleFunc("/", hello)

  mw := multiWeatherProvider{
    openWeatherMap{apiKey: *openWeatherAPIKey},
    weatherUnderground{apiKey: *wundergroundAPIKey},
  }

  http.HandleFunc("/weather/", func(w http.ResponseWriter, r *http.Request) {
    begin := time.Now()
    city := strings.SplitN(r.URL.Path, "/", 3)[2]

    temp, err := mw.temperature(city)
    if err != nil {
      http.Error(w, err.Error(), http.StatusInternalServerError)
      return
    }

    w.Header().Set("Content-Type", "application/json; charset=utf-8")
    json.NewEncoder(w).Encode(map[string]interface{}{
      "city": city,
      "temp": temp,
      "took": time.Since(begin).String(),
    })
  })

  log.Printf("Go to http://127.0.0.1:8080/")
  
  http.ListenAndServe(":8080", nil)
}

func hello(w http.ResponseWriter, r *http.Request) {
  w.Write([]byte("Hello world"))
}