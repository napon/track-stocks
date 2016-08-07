// Stocks daily price update

// Usage: go run orca.go [stock_A,stock_B,stock_C, ...]
//			 [price_A,price_B,price_C, ...]
//			 [update time]

// TODO: Handle case when user has multiple shares of the same symbol -
//			1st buy: MSFT at $37 for 1000 stocks
//			2nd buy: MSFT at $50 for  300 stocks

// TODO: Support Thailand stocks

package main

import (
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

var user_stocks map[string]float64
var update_time string

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
func debugMessage(message string) {
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
func populateUserStocks(input_stocks string, input_prices string) {
	debugMessage(fmt.Sprint("\npopulateUserStocks args: ", input_stocks, " ,", input_prices))

	stocks := strings.Split(input_stocks, ",")
	prices := strings.Split(input_prices, ",")

	for i := range stocks {
		var err error
		user_stocks[stocks[i]], err = strconv.ParseFloat(prices[i], 64)
		checkError(err)
	}

	for k, v := range user_stocks {
		debugMessage(fmt.Sprint("key: ", k, " val: ", v))
	}
}

func main() {
	if len(os.Args) < 4 {
		fmt.Printf("Usage: ./orca [stocks] [prices] [update_time]\n")
		os.Exit(1)
	}

	user_stocks = make(map[string]float64)
	populateUserStocks(os.Args[1], os.Args[2])
	update_time = os.Args[3]

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
	result += fmt.Sprintf("%-*s %-*s %-*s\n", 10, "Symbol", 15, "P.Price", 15, "M.Price")
	for symbol, uPrice := range user_stocks {
		quote := fetchQuoteForStock(symbol)
		price := quote.LastPrice
		result += fmt.Sprintf("%-*s %-*.2f %-*.2f\n", 10, strings.ToUpper(symbol), 15, uPrice, 15, price)
		investment += uPrice
		returnInvestment += price
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
