// Stocks daily price update
// Usage: go run orca.go [csv_file]
//			 [update_time]
//
// update_time := format HH:mm
// csv_file := [symbol,price,amount]
// see sample_input.csv for more info

// TODO: Support Thailand stocks
// TODO: Support ErrorMessage handling from API call

package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jasonlvhit/gocron"
	"github.com/mailgun/mailgun-go"
)

const MAILGUN_URL = "___URL___"
const MAILGUN_PRIVATE_KEY = "___PRIVATE_API_KEY___"
const MAILGUN_PUBLIC_KEY = "___PUBLIC_API_KEY___"
const USER_EMAIL = "___EMAIL___"
const QUOTE_BASE_URL = "http://dev.markitondemand.com/MODApis/Api/v2/Quote/json?symbol="
const DEBUG = false

var user_stocks []UserStock
var update_time string

type UserStock struct {
	Symbol string
	Shares []Share
}

type Share struct {
	Price  float64
	Amount int
}

type Quote struct {
	Status           string
	Name             string
	Symbol           string
	LastPrice        float64
	Change           float64
	ChangePercent    float64
	Timestamp        string
	MSDate           float64
	MarketCap        float64
	Volume           float64
	ChangeYTD        float64
	ChangePercentYTD float64
	High             float64
	Low              float64
	Open             float64
	Message          string // Error message
}

// DEBUG: Print message to console.
func debugMessage(message ...interface{}) {
	if DEBUG {
		fmt.Println(message)
	}
}

// Check if an error has occured.
func checkError(e error) {
	if e != nil {
		fmt.Printf("Error: %s\n", e)
		os.Exit(1)
	}
}

// Parse input data into user_stocks.
func populateUserStocks(csv_file string) {
	debugMessage("populateUserStocks csv: ", csv_file)

	file, err := os.Open(csv_file)
	checkError(err)

	r := csv.NewReader(file)
	records, err := r.ReadAll()
	checkError(err)

	debugMessage(records)

	i := 1
	var currentSymbol string
	var currentShares []Share
	for i < len(records) {
		record := records[i]
		if i == 1 {
			currentSymbol = record[0]
		}

		if currentSymbol == record[0] {
			price, err := strconv.ParseFloat(record[1], 64)
			checkError(err)
			amount, err := strconv.Atoi(record[2])
			checkError(err)
			currentShares = append(currentShares, Share{price, amount})
			i += 1
		} else {
			user_stocks = append(user_stocks, UserStock{currentSymbol, currentShares})

			// Clear data for a new stock.
			currentSymbol = record[0]
			currentShares = make([]Share, 0)
		}
	}

	// Add last stock item.
	user_stocks = append(user_stocks, UserStock{currentSymbol, currentShares})
	debugMessage(user_stocks)
}

func main() {
	if len(os.Args) < 3 {
		fmt.Printf("Usage: ./orca [csv_file] [update_time]\n")
		os.Exit(1)
	}

	user_stocks = make([]UserStock, 0)
	populateUserStocks(os.Args[1])
	update_time = os.Args[2]

	gocron.Clear()
	gocron.Every(1).Day().At(update_time).Do(fetchAndSendUpdate)
	<-gocron.Start()
}

// Fetch updated stocks and send email.
func fetchAndSendUpdate() {
	update := getUserStockPriceResult()
	debugMessage(update)

	mg := mailgun.NewMailgun(
		MAILGUN_URL,
		MAILGUN_PRIVATE_KEY,
		MAILGUN_PUBLIC_KEY,
	)

	email := mg.NewMessage(
		"ORCA PROJECT <mailgun@"+MAILGUN_URL+">",
		"ORCA Daily Update",
		update,
		USER_EMAIL,
	)

	_, _, err := mg.Send(email)
	checkError(err)
}

// Return a well formatted result of current market prices.
func getUserStockPriceResult() string {
	investment := 0.0
	returnInvestment := 0.0
	result := "Stocks Update " + time.Now().Format("Tue Jan 12") + "\n"
	result += "-----------------------------\n"
	result += fmt.Sprintf("%-*s %-*s\n", 10, "Symbol", 15, "M.Price")
	for _, stock := range user_stocks {
		quote := fetchQuoteForStock(stock.Symbol)
		price := quote.LastPrice
		result += fmt.Sprintf("%-*s %-*.2f\n", 10, strings.ToUpper(stock.Symbol), 15, price)
		for _, share := range stock.Shares {
			investment += share.Price * float64(share.Amount)
			returnInvestment += price * float64(share.Amount)
		}
	}
	result += "-----------------------------\n"
	result += "Total amount invested: " + fmt.Sprintf("%.2f\n", investment)
	result += "Current amount: " + fmt.Sprintf("%.2f\n", returnInvestment)
	result += "Gain/Loss = " + fmt.Sprintf("%.2f\n", returnInvestment-investment)
	return result
}

// Return market price of a stock.
func fetchQuoteForStock(code string) *Quote {
	url := QUOTE_BASE_URL + strings.ToLower(code)
	httpResponse := getHttpResponse(url)

	var quote Quote
	err := json.Unmarshal(httpResponse, &quote)
	checkError(err)
	return &quote
}

// Return byte array response of an API call.
func getHttpResponse(url string) []byte {
	resp, err := http.Get(url)
	checkError(err)

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	checkError(err)
	return body
}
