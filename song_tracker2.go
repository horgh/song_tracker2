/*
 * provide aspects of functionality relating to my song_tracker project.
 * I intend to move some/all of the functionality from PHP to Go.
 *
 * TODO:
 * - all 'api' type requests should always respond with json - even errors.
 */

package main

import (
	"database/sql"
	"errors"
	"encoding/json"
	"flag"
	"fmt"
	_ "github.com/lib/pq"
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"os"
	"regexp"
	"strconv"
	"summercat.com/config"
)

// Config holds our config keys.
type Config struct {
	ListenHost string
	ListenPort uint64
	DbUser     string
	DbPass     string
	DbName     string
	DbHost     string
	DbPort     uint64
	UriPrefix  string
}

// HttpHandler is an object implementing the http.Handler interface
// for serving requests.
type HttpHandler struct {
	settings *Config
}

// RequestHandlerFunc is a function that services a specific request.
type RequestHandlerFunc func(http.ResponseWriter, *http.Request,
	*Config)

// RequestHandler defines requests we service.
type RequestHandler struct {
	Method string
	// regex patter on the path to match.
	PathPattern string
	// handler function.
	Func RequestHandlerFunc
}

// TopResult holds row data for a 'top artist' or 'top song' request.
type TopResult struct {
	Count int64
	Label string
}

// global db connection.
// this is so we try to share a single connection for multiple requests.
// NOTE: according to the database/sql documentation, the DB type
//   is indeed safe for concurrent use by multiple goroutines.
var Db *sql.DB

// TopLimitMax defines the maximum number of 'top' results we respond to.
var TopLimitMax = 100

// connectToDb opens a new connection to the database.
func connectToDb(settings *Config) (*sql.DB, error) {
	// connect to the database.
	dsn := fmt.Sprintf("user=%s password=%s dbname=%s host=%s port=%d",
		settings.DbUser, settings.DbPass, settings.DbName, settings.DbHost,
		settings.DbPort)
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Print("Failed to connect to the database: " + err.Error())
		return nil, err
	}
	log.Print("Opened new connection to the database.")
	return db, nil
}

// getDb connects us to the database if necessary, and returns an active
// database connection.
// we use the global Db variable to try to ensure we use a single connection.
func getDb(settings *Config) (*sql.DB, error) {
	// if we have a db connection, ensure that it is still available
	// so that we reconnect if it is not.
	if Db != nil {
		err := Db.Ping()
		if err != nil {
			log.Printf("Database ping failed: %s", err.Error())
			// continue on, but set us so that we attempt to reconnect.
			Db.Close()
			Db = nil
		}
	}
	// connect to the database if necessary.
	if Db == nil {
		db, err := connectToDb(settings)
		if err != nil {
			log.Printf("Failed to connect to the database: %s", err.Error())
			return nil, err
		}
		Db = db
	}
	return Db, nil
}

// send500Error sends an internal server error with the given message in the
// body.
func send500Error(rw http.ResponseWriter, message string) {
	rw.WriteHeader(http.StatusInternalServerError)
	rw.Write([]byte(message))
}

// getParametersTopArtists retrieves and validates parameters to a
// top artists request.
// we return: user_id, limit (limit of top count), days back to build
//   the top artists count for. if days back is -1, we find the count
//   for all time.
func getParametersTopArtists(request *http.Request) (int64, int64, int64, error) {
	// pull the parameters out and convert and validate them.
	err := request.ParseForm()
	if err != nil {
		return 0, 0, 0, err
	}

	// user_id. required.
	userIdStr, exists := request.Form["user_id"]
	if !exists || len(userIdStr) != 1 {
		return 0, 0, 0, errors.New("No user ID given")
	}
	userId, err := strconv.ParseInt(userIdStr[0], 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	if userId < 0 {
		return 0, 0, 0, errors.New("Invalid user ID")
	}

	// limit. required.
	limitStr, exists := request.Form["limit"]
	if !exists || len(limitStr) != 1 {
		return 0, 0, 0, errors.New("No limit given")
	}
	limit, err := strconv.ParseInt(limitStr[0], 10, 64)
	if err != nil {
		return 0, 0, 0, err
	}
	if limit < 1 || int(limit) > TopLimitMax {
		return 0, 0, 0, errors.New("Invalid limit")
	}

	// days_back. optional.
	daysBackStr, exists := request.Form["days_back"]
	var daysBack int64 = -1
	if exists && len(daysBackStr) == 1 {
		daysBack, err = strconv.ParseInt(daysBackStr[0], 10, 64)
		if err != nil {
			return 0, 0, 0, err
		}
		if daysBack < 1 {
			return 0, 0, 0, errors.New("Invalid days back")
		}
	}
	log.Printf("Parameters: user_id [%d] limit [%d] days_back [%d]",
		userId, limit, daysBack)
	return userId, limit, daysBack, nil
}

// retrieveTopArtists retrieves the top artist counts.
// we find the top 'limit' artists for the given user.
// we do this for the specified number of days back. if the given
// days back is set as -1, we find the top artists of all time.
func retrieveTopArtists(settings *Config, userId int64, limit int64,
	daysBack int64) ([]TopResult, error) {
	// we need a database connection.
	// TODO: we could try a cache first.
	db, err := getDb(settings)
	if err != nil {
		return nil, err
	}

	query := `
SELECT
COUNT(s.id) AS count,
s.artist AS label
FROM play p
LEFT JOIN song s
ON p.song_id = s.id
WHERE
p.user_id = $1
AND s.artist != 'N/A'
AND p.create_time > current_timestamp - CAST($2 AS INTERVAL)
GROUP BY s.artist
ORDER BY count DESC
LIMIT $3
`
	interval := fmt.Sprintf("%d days", daysBack)
	if daysBack == -1 {
		// arbitrary. another alternative is to take out the create_time
		// comparison, but that means having a separate query (or messing
		// around with parameters more than I want)
		interval = "1000 years"
	}
	log.Printf("Using interval [%s]", interval)

	rows, err := db.Query(query, userId, interval, limit)
	if err != nil {
		return nil, err
	}

	var results []TopResult
	for rows.Next() {
		var result TopResult
		err := rows.Scan(&result.Count, &result.Label)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

// responseTopCount sends the response to a top artists or songs request.
func responseTopArtists(rw http.ResponseWriter, counts []TopResult) error {
	type TopResponse struct {
		Counts []TopResult
	}
	topResponse := TopResponse{Counts: counts}
	b, err := json.Marshal(topResponse)
	if err != nil {
		return err
	}
	rw.Header().Set("Content-Type", "application/json; charset=utf8")
	rw.Write(b)
	return nil
}

// handlerTopArtists looks up the top artists for a user.
func handlerTopArtists(rw http.ResponseWriter, request *http.Request,
	settings *Config) {
	// find our parameters.
	userId, limit, daysBack, err := getParametersTopArtists(request)
	if err != nil {
		msg := fmt.Sprintf("Failed to retrieve parameters: %s", err.Error())
		log.Printf(msg)
		send500Error(rw, msg)
		return
	}

	// find the counts.
	counts, err := retrieveTopArtists(settings, userId, limit, daysBack)
	if err != nil {
		msg := fmt.Sprintf("Failed to retrieve top artists: %s", err.Error())
		log.Printf(msg)
		send500Error(rw, msg)
		return
	}

	// build and send the response.
	err = responseTopArtists(rw, counts)
	if err != nil {
		msg := fmt.Sprintf("Failed to generate response: %s", err.Error())
		log.Printf(msg)
		send500Error(rw, msg)
		return
	}
}

// handlerTopSongs looks up the top songs for a user.
func handlerTopSongs(rw http.ResponseWriter, request *http.Request,
	settings *Config) {
	// TODO
}

// ServeHTTP is a function to implement the http.Handler interface.
// we service http requests.
func (handler HttpHandler) ServeHTTP(rw http.ResponseWriter,
	request *http.Request) {
	log.Printf("Serving new [%s] request from [%s] to path [%s]",
		request.Method, request.RemoteAddr, request.URL.Path)

	// define our handlers.
	var handlers = []RequestHandler{
		RequestHandler{
			Method: "GET",
			PathPattern: "^" + handler.settings.UriPrefix + "/top/artists",
			Func: handlerTopArtists,
		},
		RequestHandler{
			Method: "GET",
			PathPattern: "^" + handler.settings.UriPrefix + "/top/songs",
			Func: handlerTopSongs,
		},
	}

	// find a matching handler.
	for _, actionHandler := range handlers {
		if actionHandler.Method != request.Method {
			continue
		}
		matched, err := regexp.MatchString(actionHandler.PathPattern,
			request.URL.Path)
		if err != nil {
			log.Printf("Error matching regex: %s", err.Error())
			continue
		}
		if matched {
			actionHandler.Func(rw, request, handler.settings)
			return
		}
	}

	// there was no matching handler - send a 404.
	log.Printf("No handler for this request.")
	rw.WriteHeader(http.StatusNotFound)
	rw.Write([]byte("404 Not Found"))
}

// main is the entry point of the program.
func main() {
	log.SetFlags(log.Ltime)

	// command line arguments.
	configPath := flag.String("config-file", "",
		"Path to a configuration file.")
	flag.Parse()
	// config file is required.
	if len(*configPath) == 0 {
		log.Print("You must specify a configuration file.")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// load up our settings.
	var settings Config
	err := config.GetConfig(*configPath, &settings)
	if err != nil {
		log.Fatalf("Failed to retrieve config: %s", err.Error())
	}

	// start listening.
	var listenHostPort = fmt.Sprintf("%s:%d", settings.ListenHost,
		settings.ListenPort)
	listener, err := net.Listen("tcp", listenHostPort)
	if err != nil {
		log.Fatal("Failed to open port: " + err.Error())
	}

	httpHandler := HttpHandler{settings: &settings}

	// XXX: this will serve requests forever - should we have a signal
	//   or a method to cause this to gracefully stop?
	log.Print("Starting to serve requests.")
	err = fcgi.Serve(listener, httpHandler)
	if err != nil {
		log.Fatal("Failed to start serving HTTP: " + err.Error())
	}
}
