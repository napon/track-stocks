// Stocks daily price update
// Usage: go run orca.go [csv_file]
//			 [update_time]
//
// update_time := format HH:mm UTC time
// csv_file := [symbol,price,amount,isThaiStock]
// see sample_input.csv for more info

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
	"github.com/leekchan/accounting"
	"github.com/mailgun/mailgun-go"
)

const MAILGUN_URL = "___URL___"
const MAILGUN_PRIVATE_KEY = "___API_PRIVATE_KEY___"
const MAILGUN_PUBLIC_KEY = "___API_PUBLIC_KEY___"
const USER_EMAIL = "___EMAIL___"

const QUOTE_BASE_URL = "https://finance.yahoo.com/webservice/v1/symbols/"
const QUOTE_TH_STOCK = ".BK"
const QUOTE_URL_SUFFIX = "/quote?format=json"
const QUOTE_USER_AGENT = "Mozilla/5.0 (Linux; Android 6.0.1; MotoG3 Build/MPI24.107-55) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/51.0.2704.81 Mobile Safari/537.36"

const DEBUG = true

var user_stocks map[string]*UserStock
var update_time string

type UserStock struct {
	Shares []*Share
	IsThai bool
}

type Share struct {
	Price  float64
	Amount int
}

type Response struct {
	Quotes []*Quote
}

type Quote struct {
	Symbol    string
	LastPrice float64
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

// Override: flatten structure of response into Quote.
func (response *Response) UnmarshalJSON(b []byte) error {
	var f interface{}
	json.Unmarshal(b, &f)
	m := f.(map[string]interface{})
	n := m["list"].(map[string]interface{})
	v := n["resources"]
	resources := v.([]interface{})

	for _, resource := range resources {
		r := resource.(map[string]interface{})["resource"].(map[string]interface{})
		stock := r["fields"].(map[string]interface{})
		symbol := strings.ToUpper(stock["symbol"].(string))
		price, _ := strconv.ParseFloat(stock["price"].(string), 64)
		response.Quotes = append(response.Quotes, &Quote{symbol, price})
	}

	return nil
}

// Parse input data into user_stocks.
func populateUserStocks(csv_file string) {
	debugMessage("populateUserStocks csv: ", csv_file)

	file, err := os.Open(csv_file)
	checkError(err)

	r := csv.NewReader(file)
	records, err := r.ReadAll()
	checkError(err)

	debugMessage("records:", records)

	for i := 1; i < len(records); i++ {
		record := records[i]
		symbol := strings.ToUpper(record[0])
		price, err := strconv.ParseFloat(record[1], 64)
		checkError(err)
		amount, err := strconv.Atoi(record[2])
		checkError(err)
		isBKKStock, err := strconv.ParseBool(record[3])
		checkError(err)

		stock := user_stocks[symbol]
		share := &Share{price, amount}

		if stock == nil {
			stock = &UserStock{IsThai: isBKKStock, Shares: []*Share{share}}
			user_stocks[symbol] = stock
		} else {
			stock.Shares = append(stock.Shares, share)
		}
	}

	debugMessage("user_stocks:", user_stocks)
}

// Fetch updated stocks and send email.
func fetchAndSendUpdate() {
	update := getUserStockPriceResult()
	debugMessage("\n\nOutput result:\n", update)

	if DEBUG {
		return
	}

	mg := mailgun.NewMailgun(
		MAILGUN_URL,
		MAILGUN_PRIVATE_KEY,
		MAILGUN_PUBLIC_KEY,
	)

	email := mg.NewMessage(
		"ORCA PROJECT <orca@"+MAILGUN_URL+">",
		"ORCA Daily Update",
		update,
		USER_EMAIL,
	)

	_, _, err := mg.Send(email)
	checkError(err)

	fmt.Println("[", update_time, "] Sent an update to", USER_EMAIL)
}

// Return a well formatted result of current market prices.
func getUserStockPriceResult() string {
	investmentUSD := 0.0
	investmentTHB := 0.0
	returnInvestmentUSD := 0.0
	returnInvestmentTHB := 0.0
	result := "Stocks Update - " + time.Now().Format("Mon Jan 2") + "\n"
	result += "-----------------------------\n"
	result += fmt.Sprintf("%-*s %-*s\n", 10, "Symbol", 15, "M.Price")

	quotes := fetchStocksUpdate()
	for symbol, stock := range user_stocks {
		price := quotes[symbol].LastPrice
		result += fmt.Sprintf("%-*s %-*.2f\n", 10, symbol, 15, price)
		for _, share := range stock.Shares {
			if stock.IsThai {
				investmentTHB += share.Price * float64(share.Amount)
				returnInvestmentTHB += price * float64(share.Amount)
			} else {
				investmentUSD += share.Price * float64(share.Amount)
				returnInvestmentUSD += price * float64(share.Amount)
			}
		}
	}

	if investmentUSD > 0.0 {
		ac := accounting.Accounting{Symbol: "$", Precision: 2}
		result += "-----------------------------\n"
		result += "USD Summary\n"
		result += "Total amount invested: " + ac.FormatMoney(investmentUSD) + "\n"
		result += "Current investments: " + ac.FormatMoney(returnInvestmentUSD) + "\n"
		result += "Gain/Loss = " + ac.FormatMoney(returnInvestmentUSD-investmentUSD) + "\n"
	}

	if investmentTHB > 0.0 {
		ac := accounting.Accounting{Symbol: "฿", Precision: 2}
		result += "-----------------------------\n"
		result += "THB Summary\n"
		result += "Total amount invested: " + ac.FormatMoney(investmentTHB) + "\n"
		result += "Current investments: " + ac.FormatMoney(returnInvestmentTHB) + "\n"
		result += "Gain/Loss = " + ac.FormatMoney(returnInvestmentTHB-investmentTHB) + "\n"
	}

	result += "-----------------------------"
	return result
}

// Return market prices of user's stocks.
func fetchStocksUpdate() map[string]*Quote {
	var symbols []string
	for k, v := range user_stocks {
		s := k
		if v.IsThai {
			s += QUOTE_TH_STOCK
		}
		symbols = append(symbols, s)
	}
	debugMessage("stocks to fetch:", symbols)
	return fetchQuoteForStocks(strings.Join(symbols, ","))
}

// Return market price of stocks.
// Args: codes - list of comma separated symbols (eg. AAPL,MSFT)
func fetchQuoteForStocks(codes string) map[string]*Quote {
	url := QUOTE_BASE_URL + codes + QUOTE_URL_SUFFIX
	debugMessage("url:", url)

	httpResponse := getHttpResponse(url)
	debugMessage("http response:", string(httpResponse))

	var r Response
	err := json.Unmarshal(httpResponse, &r)
	checkError(err)

	quotes := make(map[string]*Quote)
	for _, q := range r.Quotes {
		// Remove country specific annotation.
		symbol := strings.Split(q.Symbol, ".")[0]
		quotes[symbol] = q
	}

	return quotes
}

// Return byte array response of an API call.
func getHttpResponse(url string) []byte {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	checkError(err)

	req.Header.Set("User-Agent", QUOTE_USER_AGENT)
	resp, err := client.Do(req)
	checkError(err)

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	checkError(err)
	return body
}

func main() {
	if len(os.Args) < 3 {
		fmt.Printf("Usage: ./orca [csv_file] [update_time]\n")
		os.Exit(1)
	}

	fmt.Println("[", time.Now().UTC().Format("15:04"), "] Started running..")

	user_stocks = make(map[string]*UserStock)
	populateUserStocks(os.Args[1])
	update_time = os.Args[2]

	gocron.Remove(fetchAndSendUpdate)
	gocron.Clear()

	if DEBUG {
		fetchAndSendUpdate()
	} else {
		gocron.ChangeLoc(time.UTC)
		gocron.Every(1).Day().At(update_time).Do(fetchAndSendUpdate)
		<-gocron.Start()
	}
}
