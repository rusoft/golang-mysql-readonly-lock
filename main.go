package main

import "fmt"
import "log"
import "time"
import "os"
import "os/signal"
import "syscall"
import "database/sql"
import "flag"

import _ "github.com/go-sql-driver/mysql"
import "gopkg.in/ini.v1"


var timeout = 3600
var quiet = false
var verbose = false
var canExit = false
var cnf_files []string
var log_e = log.New(os.Stderr, "ERROR ", 1)

var qLock = [...]string{"FLUSH LOGS;", "FLUSH TABLES WITH READ LOCK;", "SET GLOBAL read_only = ON;"}
var qUnlock = [...]string{"SET GLOBAL read_only = OFF;", "UNLOCK TABLES;"}

// https://stackoverflow.com/a/18411978
func VersionOrdinal(version string) string {
	// ISO/IEC 14651:2011
	const maxByte = 1<<8 - 1
	vo := make([]byte, 0, len(version)+8)
	j := -1
	for i := 0; i < len(version); i++ {
		b := version[i]
		if '0' > b || b > '9' {
			vo = append(vo, b)
			j = -1
			continue
		}
		if j == -1 {
			vo = append(vo, 0x00)
			j = len(vo) - 1
		}
		if vo[j] == 1 && vo[j+1] == '0' {
			vo[j+1] = b
			continue
		}
		if vo[j]+1 > maxByte {
			panic("VersionOrdinal: invalid version")
		}
		vo = append(vo, b)
		vo[j]++
	}
	return string(vo)
}

func read_cnf(cnf string) (string, string, string, string) {
	if verbose {
		fmt.Println("Read cnf-file: "+cnf)
	}

	username := ""
	password := ""
	hostname := ""
	socket   := ""

	// No cnf? Return defaults
	if _, err := os.Stat(cnf); os.IsNotExist(err) {
		if !quiet {
			log_e.Println(err.Error())
		}
		return username, password, hostname, socket
	}

	cfg, err := ini.Load(cnf)
	if err != nil {
		if !quiet {
			log_e.Println(err.Error())
		}
		return username, password, hostname, socket
	}

	if cfg.Section("client").HasKey("host") {
		hostname = cfg.Section("client").Key("host").String()
	}
	if cfg.Section("client").HasKey("user") {
		username = cfg.Section("client").Key("user").String()
	}
	if cfg.Section("client").HasKey("password") {
		password = cfg.Section("client").Key("password").String()
	}
	if cfg.Section("client").HasKey("socket") {
		socket = cfg.Section("client").Key("socket").String()
	}

	return username, password, hostname, socket
}

func dbGetVersion(db *sql.DB) string {
	var v string
	rows, err := db.Query("SELECT VERSION() as v;")
	if err != nil {
		if !quiet {
			log_e.Println(err.Error())
		}
		return v
	}
	defer rows.Close()
	for rows.Next() {
		err := rows.Scan(&v)
		if err != nil {
			if !quiet {
				log_e.Fatal(err)
			}
		}
	}
	err = rows.Err()
	if err != nil {
		if !quiet {
			log_e.Fatal(err)
		}
	}
	return v
}

func dbLock(db *sql.DB) {
	v := dbGetVersion(db)
	my, need := VersionOrdinal(v), VersionOrdinal("5.5.0")
	if my > need {
		_, err := db.Query("FLUSH ENGINE LOGS;")
		if err != nil {
			if !quiet {
				log_e.Println(err.Error())
			}
		}
	}
	for _,q := range qLock {
		// Execute the query
		if verbose {
			fmt.Println("Queue: "+q)
		}
		_, err := db.Query(q)
		if err != nil {
			if !quiet {
				log_e.Println(err.Error())
			}
		}
	}
}

func dbUnlock(db *sql.DB) {
	for _,q := range qUnlock {
		// Execute the query
		if verbose {
			fmt.Println("Queue: "+q)
		}
		_, err := db.Query(q)
		if err != nil {
			if !quiet {
				log_e.Println(err.Error())
			}
		}
	}
}

func main() {

	numbPtr := flag.Int("timeout", timeout, "wait timeout to unlock MySQL databases")
	boolPtrV := flag.Bool("verbose", false, "be verbose")
	boolPtrQ := flag.Bool("quiet", false, "be quiet")

	flag.Parse()

	timeout = *numbPtr
	verbose = *boolPtrV
	quiet = *boolPtrQ

	cnf_files = append(cnf_files, "~/.my.cnf", "/etc/mysql/root.cnf", "/etc/mysql/debian.cnf")
	if len(flag.Args()) > 0 {
		for _,cnf := range flag.Args() {
			cnf_files = append(cnf_files, cnf)
		}
	}
	if verbose {
		fmt.Printf("cnf-files: %v\n", cnf_files)
	}

	sigs := make(chan os.Signal, 1)

	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// goroutine for signals
	go func() {
		<-sigs
		canExit = true
	}()

	if verbose {
		fmt.Printf("awaiting signal or timeout in %d seconds\n", timeout)
	}


	dsn      := ""
	var db *sql.DB
	var err error

	for _,cnf := range cnf_files {

		username, password, hostname, socket := read_cnf(cnf)
		dsn = ""

		if socket != ""  {
			if _, err := os.Stat(socket); !os.IsNotExist(err) {
				dsn = username+":"+password+"@unix("+socket+")/"
			}
		}
		if dsn == ""  {
			if hostname != ""  {
				dsn = username+":"+password+"@tcp("+hostname+")/"
			}
		}
		if dsn == "" {
			if !quiet {
				log_e.Printf("cnf-file %s not parsed or don't have needed fields 'hostname'/'socket'!", cnf)
			}
			continue
		}
		if username == "" || password == "" {
			if !quiet {
				log_e.Printf("cnf-file %s not parsed or don't have needed fields 'username', 'password'!", cnf)
			}
			continue
		}

		db, err = sql.Open("mysql", dsn)

		if err != nil {
			if !quiet {
				log_e.Println(err.Error())
			}
			dsn = ""
			continue
		}
		defer db.Close()

		// Open doesn't open a connection. Validate DSN data:
		err = db.Ping()
		if err != nil {
			if !quiet {
				log_e.Println(err.Error())
			}
			dsn = ""
			continue
		}

		// We found working credentials
		break

	}

	if dsn == "" {
		if !quiet {
			log_e.Println("No active auth credentials available!")
		}
		os.Exit(1)
	}

	dbLock(db)

	// Dumb check for SIGNAL and exit
	for i:= 0; i<timeout; i++ {
		time.Sleep(time.Second)

		err = db.Ping()
		if err != nil {
			if !quiet {
				log_e.Println(err.Error())
			}
			canExit = true
		}

		if canExit {
			break
		}
	}

	dbUnlock(db)

	if verbose {
		fmt.Println("exiting")
	}
}
