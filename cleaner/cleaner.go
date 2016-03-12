/*
 * This program is to make cleanup type updates to the database simpler.
 *
 * It will:
 * - Report artists that may need to be consolidated
 * - Provide a way to consolidate an artist.
 */

package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	_ "github.com/lib/pq"
	"log"
	"os"
)

type args struct {
	DBUser string
	DBPass string
	DBName string
	DBHost string
	DBPort uint64

	Mode string

	ArtistOld string
	ArtistNew string
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime)

	args, err := getArgs()
	if err != nil {
		log.Printf("Invalid arguments: %s", err.Error())
		os.Exit(1)
	}

	db, err := connectToDB(args)
	if err != nil {
		os.Exit(1)
	}

	if args.Mode == "check-artists" {
		if !checkArtists(db, args) {
			os.Exit(1)
		}
		os.Exit(0)
	}

	if args.Mode == "fix-artist" {
		if !fixArtist(db, args) {
			os.Exit(1)
		}
		os.Exit(0)
	}

	log.Printf("Invalid mode: %s", args.Mode)
	os.Exit(1)
}

func getArgs() (*args, error) {
	user := flag.String("user", "songs", "Database username.")
	pass := flag.String("pass", "", "Database password.")
	name := flag.String("name", "songs", "Database name.")
	host := flag.String("host", "localhost", "Database host.")
	port := flag.Uint64("port", 5432, "Database port.")

	mode := flag.String("mode", "check-artists", "Program mode. Must be one of 'check-artists' or 'fix-artist'.")

	artistOld := flag.String("artist-old", "", "Old artist name. For fix-artist mode.")
	artistNew := flag.String("artist-new", "", "New artist name. For fix-artist mode.")

	flag.Parse()

	if len(*user) == 0 {
		err := errors.New("You must provide a database username.")
		flag.PrintDefaults()
		return nil, err
	}

	if len(*pass) == 0 {
		err := errors.New("You must provide a database password.")
		flag.PrintDefaults()
		return nil, err
	}

	if len(*name) == 0 {
		err := errors.New("You must provide a database name.")
		flag.PrintDefaults()
		return nil, err
	}

	if len(*host) == 0 {
		err := errors.New("You must provide a database host.")
		flag.PrintDefaults()
		return nil, err
	}

	if len(*mode) == 0 {
		err := errors.New("You must provide a mode.")
		flag.PrintDefaults()
		return nil, err
	}

	if *mode != "check-artists" &&
		*mode != "fix-artist" {
		err := errors.New("Invalid mode.")
		flag.PrintDefaults()
		return nil, err
	}

	if *mode == "fix-artist" {
		if len(*artistOld) == 0 ||
			len(*artistNew) == 0 {
			err := errors.New("You must provide artist old and new for fix-artist mode.")
			flag.PrintDefaults()
			return nil, err
		}
	}

	return &args{
		DBUser:    *user,
		DBPass:    *pass,
		DBName:    *name,
		DBHost:    *host,
		DBPort:    *port,
		Mode:      *mode,
		ArtistOld: *artistOld,
		ArtistNew: *artistNew,
	}, nil
}

func connectToDB(args *args) (*sql.DB, error) {
	dsn := fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%d",
		args.DBUser, args.DBPass, args.DBName, args.DBHost, args.DBPort)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Print("Failed to connect to the database: " + err.Error())
		return nil, err
	}
	return db, nil
}

func checkArtists(db *sql.DB, args *args) bool {
	// Find any that are that are duplicate if we treat them case
	// insensitively.
	// TODO: This is something we could enforce as a database constraint.
	sql := `
SELECT COUNT(1), LOWER(artist) AS artist
FROM (SELECT DISTINCT artist FROM song) d
GROUP BY LOWER(artist)
ORDER BY 1 DESC
`

	rows, err := db.Query(sql)
	if err != nil {
		log.Printf("Query error: %s", err.Error())
		return false
	}

	for rows.Next() {
		var count uint64
		var artist string
		err := rows.Scan(&count, &artist)
		if err != nil {
			log.Printf("Row scan error: %s", err.Error())
			return false
		}

		if count > 1 {
			log.Printf("Possible duplicate artist: %s", artist)
			continue
		}

		break
	}

	return true
}

func fixArtist(db *sql.DB, args *args) bool {
	var sql string = `
UPDATE song SET artist = $1 WHERE LOWER(artist) = LOWER($2) AND artist <> $3
`

	result, err := db.Exec(sql, args.ArtistNew, args.ArtistOld, args.ArtistNew)
	if err != nil {
		log.Printf("SQL failure: %s", err.Error())
		return false
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("Rows affected failure: %s", err.Error())
		return false
	}

	log.Printf("Updated %d rows to artist %s", rowsAffected, args.ArtistNew)
	return true
}
