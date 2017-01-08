/*
 * take a path to an audio file, and extract metadata/tags from it.
 *
 * take this information and record a play to the song tracker API.
 *
 * the intention is this can then be used together with any audio
 * player to scrobble with.
 * in particular I want to be able to call it together with mplayer.
 */

package main

import (
	"errors"
	"flag"
	"log"
	"os"

	"github.com/horgh/song_tracker2/client"
)

// Args describes arguments on command line
type Args struct {
	// Config is path to a configuration file.
	Config string

	// File is path to the audio file.
	File string
}

// main is the program entry
func main() {
	// turn down log prefixes
	log.SetFlags(0)

	args, err := getArgs()
	if err != nil {
		log.Printf(err.Error())
		flag.PrintDefaults()
		os.Exit(1)
	}

	err = client.ExtractAndRecord(args.Config, args.File)
	if err != nil {
		log.Print(err.Error())
		os.Exit(1)
	}
}

// getArgs retrieves and validates command line arguments
func getArgs() (*Args, error) {
	config := flag.String("config", "", "Path to the configuration file")
	file := flag.String("file", "", "Path to the audio file")

	flag.Parse()

	if len(*config) == 0 {
		return nil, errors.New("You must specify a configuration file")
	}
	if len(*file) == 0 {
		return nil, errors.New("You must specify a file")
	}

	// TODO: check files exist and are readable

	return &Args{Config: *config, File: *file}, nil
}
