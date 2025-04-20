package binance

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"main/internal/exchanges/common"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/fatih/color"
)

type Client struct {
	APIKey    string
	APISecret string
	BaseURL   string
	// Cache pour les règles de symbole
	symbolRules map[string]SymbolRules
}

// DetailedBalance représente les informations détaillées d'un solde d'actif
type DetailedBalance struct {
	Free   float64
	Locked float64
	Total  float64
}

type SymbolRules struct {
	MinQty      float64
	MaxQty      float64
	StepSize    float64
	MinNotional float64
}

// internal/exchanges/binance/client.go
// Modifions la fonction NewClient pour accepter directement les clés API

func NewClient(apiKey, apiSecret string) *Client {
	return &Client{
		APIKey:      apiKey,
		APISecret:   apiSecret,
		BaseURL:     "https://api.binance.com",
		symbolRules: make(map[string]SymbolRules),
	}
}

func (c *Client) SetBaseURL(url string) {
	c.BaseURL = url
}

// Generates HMAC SHA256 signature for a signed request
func (c *Client) signRequest(queryString string) string {
	h := hmac.New(sha256.New, []byte(c.APISecret))
	h.Write([]byte(queryString))
	return hex.EncodeToString(h.Sum(nil))
}

// Sends an HTTP request and returns the response body
func (c *Client) sendRequest(method, endpoint, queryString string) ([]byte, error) {
	fullURL := fmt.Sprintf("%s%s?%s", c.BaseURL, endpoint, queryString)

	req, err := http.NewRequest(method, fullURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-MBX-APIKEY", c.APIKey)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(resp.Body)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("error: HTTP status %d - %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func (c *Client) CheckConnection() error {
	_, err := c.sendRequest("GET", "/api/v3/ping", "")
	if err != nil {
		color.Red("Failed to connect to Binance: %v", err)
		return err
	}

	color.Green("Connexion à l'API BINANCE réussie")
	fmt.Println("")
	return nil
}

func (c *Client) GetBalanceUSD() float64 {
	color.Blue("Checking USDC balance...")

	timestamp := time.Now().UnixMilli()
	queryString := fmt.Sprintf("timestamp=%d", timestamp)
	signature := c.signRequest(queryString)
	signedQuery := fmt.Sprintf("%s&signature=%s", queryString, signature)

	body, err := c.sendRequest("GET", "/api/v3/account", signedQuery)
	if err != nil {
		log.Fatalf("Error fetching balance: %v", err)
	}

	balances, _, _, err := jsonparser.Get(body, "balances")
	if err != nil {
		log.Fatalf("Error getting balances: %v", err)
	}

	var freeFloat float64
	_, _ = jsonparser.ArrayEach(balances, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		asset, _ := jsonparser.GetString(value, "asset")
		if asset == "USDC" {
			freeStr, _ := jsonparser.GetString(value, "free")
			free, _ := strconv.ParseFloat(freeStr, 64)
			freeFloat = free
		}
	})

	color.Green("USDC Balance: %.2f", freeFloat)
	return freeFloat
}

func (c *Client) GetLastPriceBTC() float64 {
	queryString := "symbol=BTCUSDC"
	body, err := c.sendRequest("GET", "/api/v3/ticker/price", queryString)
	if err != nil {
		log.Fatalf("Error fetching BTC price: %v", err)
	}

	priceStr, err := jsonparser.GetString(body, "price")
	if err != nil {
		log.Fatalf("Error extracting price: %v", err)
	}

	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		log.Fatalf("Error converting price: %v", err)
	}
	return price
}

// Récupère et met en cache les règles pour un symbole
func (c *Client) GetSymbolRules(symbol string) (SymbolRules, error) {
	// Vérifier si nous avons déjà les règles en cache
	if rules, ok := c.symbolRules[symbol]; ok {
		return rules, nil
	}

	// Sinon, récupérer les informations d'échange
	info, err := c.GetExchangeInfo()
	if err != nil {
		return SymbolRules{}, err
	}

	var rules SymbolRules
	var symbolFound bool

	// Parcourir tous les symboles pour trouver celui qui nous intéresse
	_, _ = jsonparser.ArrayEach(info, func(symbolData []byte, dataType jsonparser.ValueType, offset int, err error) {
		symbolName, _ := jsonparser.GetString(symbolData, "symbol")
		if symbolName == symbol {
			symbolFound = true
			// Parcourir tous les filtres pour trouver LOT_SIZE et MIN_NOTIONAL
			_, _ = jsonparser.ArrayEach(symbolData, func(filter []byte, dataType jsonparser.ValueType, offset int, err error) {
				filterType, _ := jsonparser.GetString(filter, "filterType")
				if filterType == "LOT_SIZE" {
					minQtyStr, _ := jsonparser.GetString(filter, "minQty")
					maxQtyStr, _ := jsonparser.GetString(filter, "maxQty")
					stepSizeStr, _ := jsonparser.GetString(filter, "stepSize")

					rules.MinQty, _ = strconv.ParseFloat(minQtyStr, 64)
					rules.MaxQty, _ = strconv.ParseFloat(maxQtyStr, 64)
					rules.StepSize, _ = strconv.ParseFloat(stepSizeStr, 64)
				} else if filterType == "MIN_NOTIONAL" {
					minNotionalStr, _ := jsonparser.GetString(filter, "minNotional")
					rules.MinNotional, _ = strconv.ParseFloat(minNotionalStr, 64)
				}
			}, "filters")
		}
	}, "symbols")

	if !symbolFound {
		return SymbolRules{}, fmt.Errorf("symbol %s not found", symbol)
	}

	// Mettre en cache et retourner les règles
	c.symbolRules[symbol] = rules
	return rules, nil
}

// Ajuste la quantité pour respecter les règles de LOT_SIZE
func (c *Client) AdjustQuantity(symbol string, quantity float64) (float64, error) {
	rules, err := c.GetSymbolRules(symbol)
	if err != nil {
		return 0, err
	}

	// S'assurer que la quantité est >= minQty
	if quantity < rules.MinQty {
		return 0, fmt.Errorf("quantity %.8f is below minimum allowed %.8f", quantity, rules.MinQty)
	}

	// S'assurer que la quantité est <= maxQty
	if quantity > rules.MaxQty {
		return 0, fmt.Errorf("quantity %.8f is above maximum allowed %.8f", quantity, rules.MaxQty)
	}

	// Calculer le nombre de décimales pour le stepSize
	stepSizeStr := strconv.FormatFloat(rules.StepSize, 'f', -1, 64)
	decimals := 0
	if strings.Contains(stepSizeStr, ".") {
		decimals = len(stepSizeStr) - strings.IndexByte(stepSizeStr, '.') - 1
	}

	// Ajuster la quantité pour qu'elle soit un multiple du stepSize
	factor := math.Pow10(decimals)
	adjusted := math.Floor(quantity*factor/rules.StepSize/factor) * rules.StepSize

	// Formatage avec précision correcte
	adjustedStr := strconv.FormatFloat(adjusted, 'f', decimals, 64)
	result, _ := strconv.ParseFloat(adjustedStr, 64)

	return result, nil
}

// Calcule la quantité de BTC à acheter en fonction du montant USDC et du prix
func (c *Client) CalculateQuantity(usdcAmount, price float64) (float64, error) {
	rawQuantity := usdcAmount / price
	return c.AdjustQuantity("BTCUSDC", rawQuantity)
}

func (c *Client) CreateOrder(side string, price, quantity string) ([]byte, error) {
	// Convertir price et quantity en float pour pouvoir calculer et ajuster
	priceFloat, err := strconv.ParseFloat(price, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid price format: %v", err)
	}

	quantityFloat, err := strconv.ParseFloat(quantity, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid quantity format: %v", err)
	}

	// Récupérer les règles de symbole
	rules, err := c.GetSymbolRules("BTCUSDC")
	if err != nil {
		return nil, fmt.Errorf("error getting symbol rules: %v", err)
	}

	// Ajuster la quantité selon les règles
	adjustedQuantity, err := c.AdjustQuantity("BTCUSDC", quantityFloat)
	if err != nil {
		return nil, fmt.Errorf("quantity adjustment failed: %v", err)
	}

	// Vérifier la valeur notionnelle minimale (prix * quantité >= minNotional)
	notional := priceFloat * adjustedQuantity
	if notional < rules.MinNotional {
		return nil, fmt.Errorf("order value %.2f USDC is below minimum allowed %.2f USDC", notional, rules.MinNotional)
	}

	// Formatter la quantité avec la précision correcte
	stepSizeStr := strconv.FormatFloat(rules.StepSize, 'f', -1, 64)
	decimals := 0
	if strings.Contains(stepSizeStr, ".") {
		decimals = len(stepSizeStr) - strings.IndexByte(stepSizeStr, '.') - 1
	}
	adjustedQuantityStr := strconv.FormatFloat(adjustedQuantity, 'f', decimals, 64)

	// Créer la requête d'ordre
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	queryString := fmt.Sprintf(
		"symbol=BTCUSDC&side=%s&type=LIMIT&timeInForce=GTC&quantity=%s&price=%s&timestamp=%s",
		side, adjustedQuantityStr, price, timestamp,
	)

	signature := c.signRequest(queryString)
	signedQuery := fmt.Sprintf("%s&signature=%s", queryString, signature)

	// Envoyer la requête
	body, err := c.sendRequest("POST", "/api/v3/order", signedQuery)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %v", err)
	}

	return body, nil
}

func (c *Client) GetOrderById(id string) ([]byte, error) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)

	queryString := fmt.Sprintf("symbol=BTCUSDC&orderId=%s&timestamp=%s", id, timestamp)
	signature := c.signRequest(queryString)
	signedQuery := fmt.Sprintf("%s&signature=%s", queryString, signature)

	// Send request
	body, err := c.sendRequest("GET", "/api/v3/order", signedQuery)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %v", err)
	}

	return body, nil
}

func (c *Client) IsFilled(order string) bool {
	status, err := jsonparser.GetString([]byte(order), "status")
	if err != nil {
		log.Fatal(err)
	}

	return status == "FILLED"
}

func (c *Client) CancelOrder(orderID string) ([]byte, error) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)

	queryString := fmt.Sprintf("symbol=BTCUSDC&orderId=%s&timestamp=%s", orderID, timestamp)
	signature := c.signRequest(queryString)
	signedQuery := fmt.Sprintf("%s&signature=%s", queryString, signature)

	body, err := c.sendRequest("DELETE", "/api/v3/order", signedQuery)
	if err != nil {
		return nil, fmt.Errorf("error canceling order %s: %v", orderID, err)
	}

	color.Green("Order %s canceled successfully", orderID)
	return body, nil
}

func (c *Client) GetExchangeInfo() ([]byte, error) {
	body, err := c.sendRequest("GET", "/api/v3/exchangeInfo", "")
	if err != nil {
		return nil, fmt.Errorf("error getting exchange info: %v", err)
	}
	return body, nil
}

func (c *Client) GetAccountInfo() ([]byte, error) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	queryString := fmt.Sprintf("timestamp=%s", timestamp)
	signature := c.signRequest(queryString)
	signedQuery := fmt.Sprintf("%s&signature=%s", queryString, signature)

	body, err := c.sendRequest("GET", "/api/v3/account", signedQuery)
	if err != nil {
		return nil, fmt.Errorf("error getting account info: %v", err)
	}
	return body, nil
}

func (c *Client) ShowSymbolRules(symbol string) {
	rules, err := c.GetSymbolRules(symbol)
	if err != nil {
		color.Red("Error getting rules for %s: %v", symbol, err)
		return
	}

	color.Green("Symbol Rules for %s:", symbol)
	color.Green("  Minimum Quantity: %.8f", rules.MinQty)
	color.Green("  Maximum Quantity: %.8f", rules.MaxQty)
	color.Green("  Step Size: %.8f", rules.StepSize)
	color.Green("  Minimum Order Value: %.2f USDC", rules.MinNotional)
}

// Fonction utilitaire pour les tests
func (c *Client) TestOrder(usdcAmount float64) error {
	// Obtenir le prix actuel
	price := c.GetLastPriceBTC()

	// Calculer la quantité en respectant les règles
	quantity, err := c.CalculateQuantity(usdcAmount, price)
	if err != nil {
		return fmt.Errorf("failed to calculate quantity: %v", err)
	}

	priceStr := strconv.FormatFloat(price, 'f', 2, 64)
	quantityStr := strconv.FormatFloat(quantity, 'f', 8, 64)

	color.Blue("Test order parameters:")
	color.Blue("  USDC Amount: %.2f", usdcAmount)
	color.Blue("  BTC Price: %s", priceStr)
	color.Blue("  BTC Quantity: %s", quantityStr)
	color.Blue("  Total Value: %.2f USDC", price*quantity)

	// Ne pas exécuter réellement l'ordre pour un test
	return nil
}

func (c *Client) GetDetailedBalances() (map[string]common.DetailedBalance, error) {
	// Récupérer les soldes d'origine
	originalBalances, err := c.getOriginalDetailedBalances()
	if err != nil {
		return nil, err
	}

	// Convertir les soldes au format commun
	balances := make(map[string]common.DetailedBalance)
	for asset, originalBalance := range originalBalances {
		balances[asset] = common.DetailedBalance{
			Free:   originalBalance.Free,
			Locked: originalBalance.Locked,
			Total:  originalBalance.Total,
		}
	}

	// S'assurer que BTC et USDC existent même si le solde est 0
	if _, exists := balances["BTC"]; !exists {
		balances["BTC"] = common.DetailedBalance{Free: 0, Locked: 0, Total: 0}
	}
	if _, exists := balances["USDC"]; !exists {
		balances["USDC"] = common.DetailedBalance{Free: 0, Locked: 0, Total: 0}
	}

	return balances, nil
}

// Méthode d'origine pour récupérer les soldes (renommée)
func (c *Client) getOriginalDetailedBalances() (map[string]DetailedBalance, error) {
	timestamp := time.Now().UnixMilli()
	queryString := fmt.Sprintf("timestamp=%d", timestamp)
	signature := c.signRequest(queryString)
	signedQuery := fmt.Sprintf("%s&signature=%s", queryString, signature)

	body, err := c.sendRequest("GET", "/api/v3/account", signedQuery)
	if err != nil {
		return nil, fmt.Errorf("error fetching balances: %v", err)
	}

	balances := make(map[string]DetailedBalance)

	// Extraire les soldes de la réponse JSON
	_, err = jsonparser.ArrayEach(body, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		asset, _ := jsonparser.GetString(value, "asset")
		if asset == "USDC" || asset == "BTC" {
			freeStr, _ := jsonparser.GetString(value, "free")
			lockedStr, _ := jsonparser.GetString(value, "locked")

			free, _ := strconv.ParseFloat(freeStr, 64)
			locked, _ := strconv.ParseFloat(lockedStr, 64)

			balances[asset] = DetailedBalance{
				Free:   free,
				Locked: locked,
				Total:  free + locked,
			}
		}
	}, "balances")

	// S'assurer que BTC et USDC existent même si le solde est 0
	if _, exists := balances["BTC"]; !exists {
		balances["BTC"] = DetailedBalance{Free: 0, Locked: 0, Total: 0}
	}
	if _, exists := balances["USDC"]; !exists {
		balances["USDC"] = DetailedBalance{Free: 0, Locked: 0, Total: 0}
	}

	return balances, nil
}

func (c *Client) CreateMakerOrder(side string, price float64, quantity string) ([]byte, error) {
	// Ajuster le prix pour s'assurer d'être maker
	adjustedPrice := price
	if side == "BUY" {
		// Pour un achat, placer l'ordre légèrement en dessous du marché
		adjustedPrice = price * 0.998 // 0.2% en dessous
	} else {
		// Pour une vente, placer l'ordre légèrement au-dessus du marché
		adjustedPrice = price * 1.002 // 0.2% au-dessus
	}

	adjustedPriceStr := strconv.FormatFloat(adjustedPrice, 'f', 2, 64)

	return c.CreateOrder(side, adjustedPriceStr, quantity)
}
