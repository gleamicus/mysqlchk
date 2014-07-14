package main

import "os"
import "log"
import "fmt"
import "flag"
import "database/sql"
import _ "github.com/go-sql-driver/mysql"
import "net/http"

var db *sql.DB
var wsrepStmt *sql.Stmt
var readOnlyStmt *sql.Stmt

var username = flag.String("username", "clustercheckuser", "MySQL Username")
var password = flag.String("password", "clustercheckpassword!", "MySQL Password")
var host = flag.String("host", "localhost", "MySQL Server")
var port = flag.Int("port", 3306, "MySQL Port")
var timeout = flag.String("timeout", "10s", "MySQL connection timeout")
var availableWhenDonor = flag.Bool("donor", false, "Cluster available while node is a donor")
var availableWhenReadonly = flag.Bool("readonly", false, "Cluster available while node is read only")
var forceFailFile = flag.String("failfile", "/dev/shm/proxyoff", "Create this file to manually fail checks")
var forceUpFile = flag.String("upfile", "/dev/shm/proxyon", "Create this file to manually pass checks")
var bindPort = flag.Int("bindport", 9200, "MySQLChk bind port")
var bindAddr = flag.String("bindaddr", "", "MySQLChk bind address")

func init() {
	flag.Parse()
}

func checkHandler(w http.ResponseWriter, r *http.Request) {
	var fieldName, readOnly string
	var wsrepState int

	if _, err := os.Stat(*forceUpFile); err == nil {
		fmt.Fprint(w, "Cluster node OK by manual override\n")
		return
	}

	if _, err := os.Stat(*forceFailFile); err == nil {
		http.Error(w, "Cluster node unavailable by manual override", http.StatusNotFound)
		return
	}

	err := wsrepStmt.QueryRow().Scan(&fieldName, &wsrepState)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if wsrepState == 2 && *availableWhenDonor == true {
		fmt.Fprint(w, "Cluster node in Donor mode\n")
		return
	} else if wsrepState != 4 {
		http.Error(w, "Cluster node is unavailable", http.StatusServiceUnavailable)
		return
	}

	if *availableWhenReadonly == false {
		err = readOnlyStmt.QueryRow().Scan(&fieldName, &readOnly)
		if err != nil {
			http.Error(w, "Unable to determine read only setting", http.StatusInternalServerError)
			return
		} else if readOnly == "ON" {
			http.Error(w, "Cluster node is read only", http.StatusServiceUnavailable)
			return
		}
	}

	fmt.Fprint(w, "Cluster node OK\n")
}

func main() {
	flag.Parse()

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/?timeout=%s", *username, *password, *host, *port, *timeout)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		panic(err.Error())
	}

	db.SetMaxIdleConns(10)

	readOnlyStmt, err = db.Prepare("show global variables like 'read_only'")
	if err != nil {
		log.Fatal(err)
	}

	wsrepStmt, err = db.Prepare("show global status like 'wsrep_local_state'")
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Listening...")
	http.HandleFunc("/", checkHandler)
	log.Fatal(http.ListenAndServe(fmt.Sprintf("%s:%d", *bindAddr, *bindPort), nil))
}
