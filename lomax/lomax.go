// The lemming/lomax is a benchmarking tool for the lemming suite of
// MySQL Data appliations at OpenDNS. Lomax aims to support various
// forks and flavors of MySQL for benchmark and testing.
//
// Example:
//
//	   ./lomax --vector=openstack-generic-test-select.json --config=openstack-generic-config.json
//
//		[main.BenchmarkInitializeDB    ]: Time Taken: 2.995021859s      Ops:   200000        14975 ns/op
//		[main.BenchmarkPrepareStatement]: Time Taken: 1.394912791s      Ops:     5000       278982 ns/op
//		[main.BenchmarkProcessData     ]: Time Taken: 1.214483332s      Ops: 100000000          12.1 ns/op

package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"runtime"
	"strconv"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/olekukonko/tablewriter"
	"github.com/opendns/lemming/lib/log"
)

// USER : The MySQL user, passed in through the config file
var USER string

// PASSWORD : The MySQL user's password, passed in through the config file
var PASSWORD string

// This schema is only valid for datacharmer/test_db
// If you would like to use your own, please change accordingly.
var (
	deptNo    string
	deptName  string
	empNo     int
	fromDate  string
	toDate    string
	birthDate string
	firstName string
	lastName  string
	gender    string
	hireDate  string
	salary    int
	title     string
)

// Make stuff that is common globally accessible
var operationPtr, columnsPtr, dbPtr, tablePtr, conditionPtr string
var logType, logPrefix string
var config map[string]interface{}
var benchmarkData []string
var benchBuffer [][]string
var countPtr float64

var jsonConfig, testVectorConfig string

func init() {
	flag.StringVar(&jsonConfig, "config", "", "JSON config: Input a predefined JSON configuration file.")
	flag.StringVar(&testVectorConfig, "vector", "", "Test Vectors: Input a predefined test vector configuration file.")
	flag.StringVar(&logPrefix, "logprefix", "", "Log Prefix: Defines the prefix for output result file.")
	flag.StringVar(&logType, "logtype", "", "Log Prefix: Defines the output format for storing test results.")
	flag.StringVar(&operationPtr, "operation", "", "Query to run: e.g. SELECT, INSERT..")
	flag.StringVar(&columnsPtr, "cols", "", "Columns to select in a query.")
	flag.StringVar(&dbPtr, "db", "", "DB to perform queries on.")
	flag.StringVar(&tablePtr, "table", "", "Table to use for operations.")
	flag.StringVar(&conditionPtr, "condition", "", "Any conditions to enforce on query.")
	flag.Float64Var(&countPtr, "count", 1, "Number of iterations to perform.")
	flag.StringVar(&USER, "user", "", "MySQL username.")
	flag.StringVar(&PASSWORD, "password", "", "MySQL password.")
}

func setPtrs() {
	configParse()
	if jsonConfig != "" && testVectorConfig != "" {
		operationPtr = config["action"].(string)
		columnsPtr = config["columns"].(string)
		dbPtr = config["test_db"].(string)
		tablePtr = config["test_table"].(string)
		conditionPtr = config["condition"].(string)
		countPtr = config["count"].(float64)
		USER = config["user"].(string)
		PASSWORD = config["pass"].(string)
	}
}

// GetFunctionName : Returns the name of the passed function
func GetFunctionName(i interface{}) string {
	return runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name()
}

// BenchmarkInitializeDB : Benchmark helper function to benchmark initializeDB()
func BenchmarkInitializeDB(bench *testing.B) {
	for iter := 0; iter < bench.N; iter++ {
		db := initializeDB()
		db.Close()
	}
}

// BenchmarkPrepareStatement : Benchmark helper function to benchmark prepareStatement()
func BenchmarkPrepareStatement(bench *testing.B) {
	db := initializeDB()
	defer db.Close()

	for iter := 0; iter < bench.N; iter++ {
		rows := prepareStatement(db, operationPtr, columnsPtr, tablePtr, conditionPtr)
		rows.Close()
	}
}

// BenchmarkProcessData : Benchmark helper function to benchmark processData()
func BenchmarkProcessData(bench *testing.B) {
	db := initializeDB()
	defer db.Close()

	rows := prepareStatement(db, operationPtr, columnsPtr, tablePtr, conditionPtr)
	defer rows.Close()

	for iter := 0; iter < bench.N; iter++ {
		_ = processData(rows)
	}
}

func configParse(inputFile ...string) {

	if inputFile != nil {
		file, err := ioutil.ReadFile(fmt.Sprintf("./lib/%s", inputFile[0]))
		if err != nil {
			log.Error(fmt.Sprintf("File IO Error: %s\n", err.Error()))
		}
		fileTestConfig, errTestConfig := ioutil.ReadFile(fmt.Sprintf("./testvectors/%s", inputFile[1]))
		if errTestConfig != nil {
			log.Error(fmt.Sprintf("Test config File IO Error: %s\n", err.Error()))
		}
		json.Unmarshal(file, &config)
		json.Unmarshal(fileTestConfig, &config)
		USER = config["user"].(string)
		PASSWORD = config["pass"].(string)
	} else {
		if jsonConfig != "" {
			file, err := ioutil.ReadFile(fmt.Sprintf("./lib/%s", jsonConfig))
			if err != nil {
				log.Error(fmt.Sprintf("File IO Error: %s\n", err.Error()))
			}
			json.Unmarshal(file, &config)
		}

		if testVectorConfig != "" {
			fileTestConfig, errTestConfig := ioutil.ReadFile(fmt.Sprintf("./testvectors/%s", testVectorConfig))
			if errTestConfig != nil {
				log.Error(fmt.Sprintf("Test config File IO Error: %s\n", errTestConfig.Error()))
			}
			json.Unmarshal(fileTestConfig, &config)
		}
	}
}

func validateInput() {
	if tablePtr == "" && testVectorConfig == "" {
		log.Error("Please specify a MySQL table using the --table option.")
	} else if dbPtr == "" && testVectorConfig == "" {
		log.Error("Please specify a MySQL database using the --database option.")
	} else if operationPtr == "" && testVectorConfig == "" {
		log.Error("Please specify a MySQL operation using the --operation option or specify a test vector using --vector option.")
	} else if columnsPtr == "" && testVectorConfig == "" {
		log.Error("Please specify columns to operate on using the --cols option or specify a test vector using the --vector option.")
	} else if USER == "" && jsonConfig == "" {
		log.Error("Please specify a MySQL user using the --user option.")
	} else if PASSWORD == "" && jsonConfig == "" {
		log.Error("Please specify the MySQL password for the user using the --password option.")
	}
}

func initializeDB(inputParams ...string) *sql.DB {
	// lomax_test.go uses custom command function name for testing purposes only
	if len(inputParams) != 0 {
		db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@/%s", inputParams[0], inputParams[1], inputParams[2]))
		if err != nil {
			log.Error(err.Error())
		}
		return db
	}

	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@/%s", USER, PASSWORD, dbPtr))
	if err != nil {
		log.Error(err.Error())
	}
	return db
}

func prepareStatement(db *sql.DB, operationPtr string, columnsPtr string, tablePtr string, conditionPtr string) *sql.Rows {
	stmtOut, err := db.Prepare(fmt.Sprintf("%s %s FROM %s %s", operationPtr, columnsPtr, tablePtr, conditionPtr))
	if err != nil {
		log.Error(err.Error())
	}
	rows, err := stmtOut.Query()
	if err != nil {
		log.Error(err.Error())
	}
	defer stmtOut.Close()
	return rows
}

func processData(rows *sql.Rows, inputParams ...string) bool {
	if inputParams != nil {
		tablePtr = inputParams[0]
	}

	for rows.Next() {
		switch tablePtr {
		case "employees":
			err := rows.Scan(&empNo, &birthDate, &firstName, &lastName, &gender, &hireDate)
			if err != nil {
				log.Error(err.Error())
			}
			// log.Debug(strconv.Itoa(empNo), birthDate, firstName, lastName, gender, hireDate)
		case "departments":
			err := rows.Scan(&deptNo, &deptName)
			if err != nil {
				log.Error(err.Error())
			}
			// log.Debug(strconv.Itoa(deptNo), deptName)

		default:
			log.Error("Invalid table specified, please check the --table option.")
			return false
		}
		err := rows.Err()
		if err != nil {
			log.Error(err.Error())
			return false
		}
	}
	// Only reaches here if rows is empty.
	if rows != nil {
		return true
	}
	return false
}

func runBenchmarks() {
	fmt.Println(fmt.Sprintf("Running benchmarks, please wait..."))

	if logPrefix == "" {
		log.Warning(fmt.Sprintf("[%s]: No --logprefix defined, log file will NOT be created", GetFunctionName(exportData)))
	}

	br := testing.Benchmark(BenchmarkInitializeDB)
	collectData(br, BenchmarkInitializeDB)

	br = testing.Benchmark(BenchmarkPrepareStatement)
	collectData(br, BenchmarkPrepareStatement)

	br = testing.Benchmark(BenchmarkProcessData)
	collectData(br, BenchmarkProcessData)

	printData()
}

func writeToFile() *os.File {
	filePtr, err := os.OpenFile(fmt.Sprintf("./results/%s.%s.%s", logPrefix, logType, strconv.FormatInt(time.Now().Unix(), 10)), os.O_RDWR|os.O_APPEND|os.O_CREATE, 0666)
	if err != nil {
		log.Error("[%s]: Cannot create file for writing.", GetFunctionName(writeToFile))
	}
	return filePtr
}

func collectData(br testing.BenchmarkResult, funcPtr func(*testing.B)) {
	if logType == "json" && logPrefix != "" {
		benchmarkStr := fmt.Sprintf("%s,%s", br.T, br.N)
		benchmarkData = append(benchmarkData, string(benchmarkStr))
		benchBuffer = append(benchBuffer, []string{fmt.Sprintf("%s", GetFunctionName(funcPtr)), fmt.Sprintf("%s", br.T), fmt.Sprintf("%d", br.N), fmt.Sprintf("%d", br.MemAllocs), fmt.Sprintf("%d", br.MemBytes)})
	} else if logType == "csv" && logPrefix != "" {
		benchmarkStr := fmt.Sprintf("%s,%s", br.T, br.N)
		benchmarkData = append(benchmarkData, string(benchmarkStr))
		benchBuffer = append(benchBuffer, []string{fmt.Sprintf("%s", GetFunctionName(funcPtr)), fmt.Sprintf("%s", br.T), fmt.Sprintf("%d", br.N), fmt.Sprintf("%d", br.MemAllocs), fmt.Sprintf("%d", br.MemBytes)})
	} else {
		benchBuffer = append(benchBuffer, []string{fmt.Sprintf("%s", GetFunctionName(funcPtr)), fmt.Sprintf("%s", br.T), fmt.Sprintf("%d", br.N), fmt.Sprintf("%d", br.MemAllocs), fmt.Sprintf("%d", br.MemBytes)})
	}
}

func printData() {

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Function", "Time Taken", "Iterations", "MemAllocs", "MemBytes"})

	for _, value := range benchBuffer {
		table.Append(value)
	}
	table.Render()
}

func exportData() {
	if logType == "json" {
		filePtr := writeToFile()
		var tempString [][]string
		for _, value := range benchBuffer {
			tempString = append(tempString, value)
		}
		jsonString, _ := json.MarshalIndent(tempString, "", "  ")
		for _, value := range jsonString {
			_, err := filePtr.WriteString(string(value))
			if err != nil {
				log.Error("[%s]: Couldn't write to the JSON output file", GetFunctionName(exportData))
			}
		}
		defer filePtr.Close()
	} else if logType == "csv" {
		filePtr := writeToFile()
		csvWriter := csv.NewWriter(filePtr)
		for _, value := range benchBuffer {
			err := csvWriter.Write(value)
			if err != nil {
				log.Error("[%s]: Cannot write to CSV file", GetFunctionName(exportData))
			}
		}
		csvWriter.Flush()
		defer filePtr.Close()
	} else {
		log.Warning("No --logtype specified, only logging to stdout.")
	}
}

func main() {
	flag.Parse()

	setPtrs()

	validateInput()

	runBenchmarks()

	if logPrefix != "" {
		exportData()
	}
}
