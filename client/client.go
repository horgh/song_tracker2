package client

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"summercat.com/taglib"
)

// hold configuration
type Config struct {
	Username string
	Password string
	// to api.php
	URL   string
	Debug string
}

// hold metadata/tags from audio file
type Tags struct {
	Artist        string
	Album         string
	Title         string
	LengthSeconds int
}

// parse a song tracker configuration
func ParseConfig(config string) (*Config, error) {
	fd, err := os.Open(config)
	if err != nil {
		log.Printf("Unable to open: %s: %s", config, err.Error())
		return nil, err
	}
	defer fd.Close()

	// options we parse out
	username := ""
	password := ""
	url := ""
	debug := ""

	scanner := bufio.NewScanner(fd)
	for scanner.Scan() {
		// skip comments
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) == 0 {
			continue
		}

		pieces := strings.Split(line, "=")
		if len(pieces) != 2 {
			log.Printf("Invalid line: %s", line)
			return nil, fmt.Errorf("Invalid configuration line: %s", line)
		}

		key := strings.TrimSpace(pieces[0])
		value := strings.TrimSpace(pieces[1])
		if len(key) == 0 || len(value) == 0 {
			log.Printf("Key/value is blank: %s", line)
			return nil, fmt.Errorf("Key/value is blank: %s", line)
		}

		if key == "username" {
			username = value
			continue
		}
		if key == "password" {
			password = value
			continue
		}
		if key == "url" {
			url = value
			continue
		}
		if key == "debug" {
			debug = value
			continue
		}
		log.Printf("Unknown config key: %s", key)
		return nil, fmt.Errorf("Unknown config key: %s", key)
	}
	if err = scanner.Err(); err != nil {
		log.Printf("Reading error: %s", err.Error())
		return nil, err
	}

	if username == "" || password == "" || url == "" || debug == "" {
		log.Printf("Missing required configuration key")
		return nil, errors.New("Missing required configuration key")
	}

	return &Config{
		Username: username,
		Password: password,
		URL:      url,
		Debug:    debug,
	}, nil
}

// extract tags from an audio file
func ExtractTags(file string) (*Tags, error) {
	tags, err := taglib.ExtractTags(file)
	if err != nil {
		return nil, err
	}

	properties, err := taglib.ExtractProperties(file)
	if err != nil {
		return nil, err
	}

	return &Tags{
		Artist:        tags.Artist,
		Album:         tags.Album,
		Title:         tags.Title,
		LengthSeconds: properties.LengthSeconds,
	}, nil
}

// send API request to record a play
func RecordPlay(config *Config, tags *Tags) error {
	log.Printf("Recording Artist [%s] Album [%s] Title [%s] Seconds [%d]",
		tags.Artist, tags.Album, tags.Title, tags.LengthSeconds)

	// api wants time in milliseconds...
	lengthMilliseconds := tags.LengthSeconds * 1000

	v := url.Values{}
	v.Set("user", config.Username)
	v.Set("pass", config.Password)
	v.Set("artist", tags.Artist)
	v.Set("album", tags.Album)
	v.Set("title", tags.Title)
	v.Set("length", fmt.Sprintf("%d", lengthMilliseconds))

	// NOTE: we set up a http.Transport to use TLS settings (we do not want
	//   to check certificates because my site does not have a valid one
	//   right now), and then set the transport on the http.Client, and then
	//   make the request.
	//   we have to do it in this round about way rather than simply
	//   http.Get() or the like in order to pass through the TLS setting it
	//   appears.
	tlsConfig := &tls.Config{
		InsecureSkipVerify: true,
	}
	httpTransport := &http.Transport{
		TLSClientConfig: tlsConfig,
	}
	httpClient := &http.Client{
		Transport: httpTransport,
	}

	httpResponse, err := httpClient.PostForm(config.URL, v)
	if err != nil {
		log.Print("HTTP POST failure")
		// it appears we do not need to call Body.Close() here - if we try
		// then we get a runtime error about nil pointer dereference.
		return err
	}

	body, err := ioutil.ReadAll(httpResponse.Body)
	httpResponse.Body.Close()
	if err != nil {
		log.Print("Failed to read response body: " + err.Error())
		return err
	}
	log.Printf("Response body: %s", body)

	if httpResponse.StatusCode != 200 {
		log.Printf("HTTP response is not 200")
		return fmt.Errorf("HTTP code %d", httpResponse.StatusCode)
	}

	log.Printf("Play recorded!")
	return nil
}

// ExtractAndRecord parses the configuration, extracts metadata,
// and records a play. easy all in one.
func ExtractAndRecord(configFile string, file string) error {
	// parse config
	config, err := ParseConfig(configFile)
	if err != nil {
		return err
	}

	// extract tag data
	tags, err := ExtractTags(file)
	if err != nil {
		return err
	}

	// send request
	err = RecordPlay(config, tags)
	if err != nil {
		return err
	}

	log.Printf("Play recorded")
	return nil
}
