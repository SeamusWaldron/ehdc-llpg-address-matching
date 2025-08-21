package main

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

const (
	host     = "localhost"
	port     = 15432
	user     = "user"
	password = "password"
	dbname   = "ehdc_gis"
)

func main() {
	psqlInfo := fmt.Sprintf("host=%s port=%d user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", psqlInfo)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	err = db.Ping()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Successfully connected!")

	createTables(db)
	importEHDCData(db)
	importOSData(db)

	r := gin.Default()
	r.GET("/", func(c *gin.Context) {
		c.File("./index.html")
	})
	r.GET("/data", func(c *gin.Context) {
		rows, err := db.Query(`
			SELECT 
				e.locaddress, 
				COALESCE(o.latitude, e.latitude) as latitude,
				COALESCE(o.longitude, e.longitude) as longitude
			FROM ehdc_addresses e
			LEFT JOIN os_addresses o ON e.bs7666uprn = o.uprn
		`)
		if err != nil {
			log.Fatal(err)
		}
		defer rows.Close()

		var addresses []map[string]interface{}
		for rows.Next() {
			var locaddress string
			var latitude, longitude float64
			err := rows.Scan(&locaddress, &latitude, &longitude)
			if err != nil {
				log.Fatal(err)
			}
			addresses = append(addresses, map[string]interface{}{"locaddress": locaddress, "latitude": latitude, "longitude": longitude})
		}

		c.JSON(http.StatusOK, addresses)
	})

	r.Run(":8080")
}

func createTables(db *sql.DB) {
	// Create the table for the East Hampshire data
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS ehdc_addresses (
			ogc_fid SERIAL PRIMARY KEY,
			locaddress TEXT,
			easting NUMERIC,
			northing NUMERIC,
			lgcstatusc TEXT,
			bs7666uprn BIGINT,
			bs7666usrn BIGINT,
			landparcel TEXT,
			blpuclass TEXT,
			postal TEXT,
			latitude NUMERIC,
			longitude NUMERIC
		);
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Create the table for the OS Open UPRN data
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS os_addresses (
			uprn BIGINT PRIMARY KEY,
			latitude NUMERIC,
			longitude NUMERIC
		);
	`)
	if err != nil {
		log.Fatal(err)
	}

	// Create the PostGIS extension
	_, err = db.Exec("CREATE EXTENSION IF NOT EXISTS postgis")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Tables created successfully!")
}

func importEHDCData(db *sql.DB) {
	file, err := os.Open("ehdc_llpg_20250710-1.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	r := csv.NewReader(file)
	// Skip header
	_, err = r.Read()
	if err != nil {
		log.Fatal(err)
	}

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		// Convert easting and northing to latitude and longitude
		// This is a simplified conversion and might not be perfectly accurate
		// For a real application, you would use a proper library for this conversion
		// For now, we'll just store them as is and handle the conversion later

		var uprn interface{}
		if record[5] != "" {
			parsedUPRN, parseErr := strconv.ParseInt(record[5], 10, 64)
			if parseErr != nil {
				log.Printf("Warning: Could not parse UPRN '%s' to int64. Inserting NULL. Error: %v", record[5], parseErr)
				uprn = nil
			} else {
				uprn = parsedUPRN
			}
		} else {
			uprn = nil
		}

		var usrn interface{}
		if record[6] != "" {
			parsedUSRN, parseErr := strconv.ParseInt(record[6], 10, 64)
			if parseErr != nil {
				log.Printf("Warning: Could not parse USRN '%s' to int64. Inserting NULL. Error: %v", record[6], parseErr)
				usrn = nil
			} else {
				usrn = parsedUSRN
			}
		} else {
			usrn = nil
		}

		_, err = db.Exec(`
			INSERT INTO ehdc_addresses (locaddress, easting, northing, lgcstatusc, bs7666uprn, bs7666usrn, landparcel, blpuclass, postal)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, record[1], record[2], record[3], record[4], uprn, usrn, record[7], record[8], record[9])
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println("Finished importing EHDC data")
}

func importOSData(db *sql.DB) {
	file, err := os.Open("osopenuprn_202507_csv/osopenuprn_202507.csv")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	r := csv.NewReader(file)
	// Skip header
	_, err = r.Read()
	if err != nil {
		log.Fatal(err)
	}

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		var uprn interface{}
		if record[0] != "" {
			parsedUPRN, parseErr := strconv.ParseInt(record[0], 10, 64)
			if parseErr != nil {
				log.Printf("Warning: Could not parse UPRN '%s' from OS data to int64. Inserting NULL. Error: %v", record[0], parseErr)
				uprn = nil
			} else {
				uprn = parsedUPRN
			}
		} else {
			uprn = nil
		}

		_, err = db.Exec(`
			INSERT INTO os_addresses (uprn, latitude, longitude)
			VALUES ($1, $2, $3)
		`, uprn, record[3], record[4])
		if err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println("Finished importing OS data")
}
