// internal/exchanges/kucoin/client.go
package kucoin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"main/internal/exchanges/common"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
)

// Ajout d'une structure pour stocker les règles de symbole
type SymbolRules struct {
	Symbol         string
	BaseCurrency   string
	QuoteCurrency  string
	BaseMinSize    float64
	BaseMaxSize    float64
	QuoteMinSize   float64
	QuoteMaxSize   float64
	BaseIncrement  float64
	QuoteIncrement float64
	PriceIncrement float64
	PriceLimitRate float64
}

var symbolRulesCache = make(map[string]SymbolRules)

// Client représente un client API pour l'échange KuCoin
type Client struct {
	APIKey     string
	APISecret  string
	Passphrase string
	BaseURL    string
	Debug      bool
}

// Réponse standardisée de KuCoin
type kuCoinResponse struct {
	Code    string          `json:"code"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"msg"`
}

// Détails sur le prix d'un actif retourné par KuCoin
type tickerResponse struct {
	Price       string `json:"price"`
	BestBid     string `json:"bestBid"`
	BestAsk     string `json:"bestAsk"`
	Size        string `json:"size"`
	Time        int64  `json:"time"`
	Sequence    string `json:"sequence"`
	BestBidSize string `json:"bestBidSize"`
	BestAskSize string `json:"bestAskSize"`
}

// Informations sur un compte
type accountInfo struct {
	ID        string `json:"id"`
	Currency  string `json:"currency"`
	Type      string `json:"type"`
	Balance   string `json:"balance"`
	Available string `json:"available"`
	Holds     string `json:"holds"`
}

// NewClient crée une nouvelle instance de client KuCoin
func NewClient(apiKey, apiSecret string) *Client {
	// Pour KuCoin, le passphrase est généralement stocké dans le même champ que APISecret
	// Format attendu: "secret:passphrase"
	var passphrase string
	parts := strings.Split(apiSecret, ":")
	if len(parts) > 1 {
		apiSecret = parts[0]
		passphrase = parts[1]
	}

	return &Client{
		APIKey:     apiKey,
		APISecret:  apiSecret,
		Passphrase: passphrase,
		BaseURL:    "https://api.kucoin.com",
		Debug:      false,
	}
}

// SetBaseURL permet de modifier l'URL de base de l'API
func (c *Client) SetBaseURL(url string) {
	c.BaseURL = url
}

// SetDebug active ou désactive le mode debug
func (c *Client) SetDebug(debug bool) {
	c.Debug = debug
}

// Logs un message de debug si le mode debug est activé
func (c *Client) logDebug(format string, args ...interface{}) {
	if c.Debug {
		color.Blue("[DEBUG] "+format, args...)
	}
}

// Génère la signature HMAC-SHA256 pour KuCoin
func (c *Client) signRequest(timestamp, method, endpoint, body string) string {
	message := timestamp + method + endpoint + body
	h := hmac.New(sha256.New, []byte(c.APISecret))
	h.Write([]byte(message))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// Génère le passphrase crypté pour l'API v2 de KuCoin
func (c *Client) signPassphrase() string {
	h := hmac.New(sha256.New, []byte(c.APISecret))
	h.Write([]byte(c.Passphrase))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// Envoie une requête HTTP à l'API KuCoin
func (c *Client) sendRequest(method, endpoint string, body string) ([]byte, error) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	signature := c.signRequest(timestamp, method, endpoint, body)

	// Construire l'URL complète
	fullURL := c.BaseURL + endpoint
	if method == "GET" && body != "" {
		fullURL += "?" + body
	}

	if c.Debug {
		c.logDebug("URL complète: %s", fullURL)
		c.logDebug("Body: %s", body)
	}

	// Créer la requête
	req, err := http.NewRequest(method, fullURL, strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la création de la requête: %w", err)
	}

	// Ajouter les en-têtes requis par KuCoin
	req.Header.Set("KC-API-KEY", c.APIKey)
	req.Header.Set("KC-API-SIGN", signature)
	req.Header.Set("KC-API-TIMESTAMP", timestamp)

	// En v2, le passphrase doit être crypté
	encryptedPassphrase := c.signPassphrase()
	req.Header.Set("KC-API-PASSPHRASE", encryptedPassphrase)
	req.Header.Set("KC-API-KEY-VERSION", "2")

	req.Header.Set("Content-Type", "application/json")

	// Envoyer la requête
	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	if c.Debug {
		c.logDebug("En-têtes:")
		for k, v := range req.Header {
			c.logDebug("  %s: %s", k, v)
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de l'envoi de la requête: %w", err)
	}
	defer resp.Body.Close()

	// Lire la réponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la lecture de la réponse: %w", err)
	}

	if c.Debug {
		c.logDebug("Réponse brute: %s", string(responseBody))
	}

	// Vérifier le code de statut HTTP
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erreur HTTP %d: %s", resp.StatusCode, string(responseBody))
	}

	// Décoder la réponse
	var response kuCoinResponse
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return nil, fmt.Errorf("erreur lors du décodage de la réponse: %w", err)
	}

	// Vérifier le code de la réponse
	if response.Code != "200000" {
		return nil, fmt.Errorf("erreur API KuCoin: %s - %s", response.Code, response.Message)
	}

	// Retourner les données
	return response.Data, nil
}

// CheckConnection vérifie la connexion à l'API KuCoin
func (c *Client) CheckConnection() error {
	_, err := c.sendRequest("GET", "/api/v1/timestamp", "")
	if err != nil {
		color.Red("Échec de connexion à KuCoin: %v", err)
		return err
	}

	color.Green("Connexion à l'API KUCOIN réussie")
	return nil
}

// GetLastPriceBTC récupère le prix actuel du BTC
func (c *Client) GetLastPriceBTC() float64 {
	endpoint := "/api/v1/market/orderbook/level1"
	queryString := "symbol=BTC-USDC"

	data, err := c.sendRequest("GET", endpoint, queryString)
	if err != nil {
		log.Fatalf("Erreur lors de la récupération du prix BTC: %v", err)
	}

	var ticker tickerResponse
	if err := json.Unmarshal(data, &ticker); err != nil {
		log.Fatalf("Erreur lors du décodage des données du ticker: %v", err)
	}

	price, err := strconv.ParseFloat(ticker.Price, 64)
	if err != nil {
		log.Fatalf("Erreur lors de la conversion du prix: %v", err)
	}
	return price
}

// normalizeOrderId normalise un ID d'ordre KuCoin
func (c *Client) normalizeOrderId(orderId string) string {
	// Si l'ID est vide, retourner une chaîne vide
	if orderId == "" {
		return ""
	}

	// Nettoyer l'ID en supprimant les espaces
	cleanedId := strings.TrimSpace(orderId)

	// Pour KuCoin, les IDs sont généralement de longues chaînes alphanumériques
	// mais certaines réponses peuvent contenir des préfixes ou des suffixes
	if len(cleanedId) > 24 {
		// Extraire un motif d'ID KuCoin typique (24 caractères alphanumériques)
		re := regexp.MustCompile("[a-zA-Z0-9]{24}")
		matches := re.FindAllString(cleanedId, -1)
		if len(matches) > 0 {
			return matches[0]
		}
	}

	// Si aucun motif spécifique n'est trouvé, retourner l'ID nettoyé
	return cleanedId
}

// CreateOrder crée un nouvel ordre sur KuCoin
// Modification de la méthode CreateOrder pour utiliser FormatPrice
func (c *Client) CreateOrder(side, price, quantity string) ([]byte, error) {
	endpoint := "/api/v1/orders"

	// Adapter le side pour KuCoin (buy/sell au lieu de BUY/SELL)
	kuSide := strings.ToLower(side)

	// Vérifier si le prix est déjà formaté correctement
	if _, err := strconv.ParseFloat(price, 64); err == nil {
		// Le prix est fourni en tant que chaîne, vérifier s'il est correctement formaté
		priceValue, _ := strconv.ParseFloat(price, 64)
		formattedPrice, err := c.FormatPrice("BTC-USDC", priceValue)
		if err == nil && formattedPrice != price {
			c.logDebug("Reformatage du prix: %s -> %s", price, formattedPrice)
			price = formattedPrice
		}
	}

	// Créer le corps de la requête
	orderData := map[string]string{
		"clientOid":   fmt.Sprintf("bot-%d", time.Now().UnixNano()), // ID unique généré côté client
		"side":        kuSide,
		"symbol":      "BTC-USDC",
		"type":        "limit",
		"price":       price,
		"size":        quantity,
		"timeInForce": "GTC", // Good Till Canceled
	}

	jsonData, err := json.Marshal(orderData)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la création du JSON pour l'ordre: %w", err)
	}

	// Envoyer la requête
	data, err := c.sendRequest("POST", endpoint, string(jsonData))
	if err != nil {
		return nil, fmt.Errorf("erreur lors de l'envoi de l'ordre: %w", err)
	}

	return data, nil
}

// GetOrderById récupère les informations d'un ordre spécifique
func (c *Client) GetOrderById(id string) ([]byte, error) {
	// Normaliser l'ID d'ordre
	normalizedId := c.normalizeOrderId(id)
	if normalizedId == "" {
		return nil, fmt.Errorf("ID d'ordre invalide: %s", id)
	}

	endpoint := fmt.Sprintf("/api/v1/orders/%s", normalizedId)

	// Envoyer la requête
	data, err := c.sendRequest("GET", endpoint, "")
	if err != nil {
		// Si l'ordre n'est pas trouvé, essayer de chercher dans l'historique
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Order does not exist") {
			return c.findOrderInHistory(normalizedId)
		}
		return nil, err
	}

	return data, nil
}

// findOrderInHistory cherche un ordre dans l'historique des ordres
func (c *Client) findOrderInHistory(orderId string) ([]byte, error) {
	endpoint := "/api/v1/orders"
	queryString := "status=done"

	data, err := c.sendRequest("GET", endpoint, queryString)
	if err != nil {
		return nil, err
	}

	// Décoder la réponse
	var orders []map[string]interface{}
	if err := json.Unmarshal(data, &orders); err != nil {
		return nil, fmt.Errorf("erreur lors du décodage des ordres: %w", err)
	}

	// Chercher l'ordre dans la liste
	for _, order := range orders {
		if id, ok := order["id"].(string); ok && id == orderId {
			orderData, err := json.Marshal(order)
			if err != nil {
				return nil, fmt.Errorf("erreur lors de l'encodage de l'ordre: %w", err)
			}
			return orderData, nil
		}
	}

	return nil, fmt.Errorf("ordre non trouvé dans l'historique: %s", orderId)
}

// IsFilled vérifie si un ordre est complètement exécuté
func (c *Client) IsFilled(order string) bool {
	var orderData map[string]interface{}
	if err := json.Unmarshal([]byte(order), &orderData); err != nil {
		c.logDebug("Erreur lors du décodage de l'ordre: %v", err)
		return false
	}

	// Vérifier si l'ordre est complété
	if isActive, ok := orderData["isActive"].(bool); ok && !isActive {
		// Vérifier si la quantité exécutée est égale à la quantité totale
		if dealSize, ok := orderData["dealSize"].(string); ok {
			if size, ok := orderData["size"].(string); ok {
				return dealSize == size
			}
		}
	}

	return false
}

// CancelOrder annule un ordre existant sur KuCoin
func (c *Client) CancelOrder(orderID string) ([]byte, error) {
	// Normaliser l'ID de l'ordre
	orderIDToUse := c.normalizeOrderId(orderID)
	if orderIDToUse == "" {
		return nil, fmt.Errorf("ID d'ordre invalide: %s", orderID)
	}

	endpoint := fmt.Sprintf("/api/v1/orders/%s", orderIDToUse)

	// Envoyer la requête d'annulation
	data, err := c.sendRequest("DELETE", endpoint, "")
	if err != nil {
		return nil, fmt.Errorf("erreur lors de l'annulation de l'ordre %s: %w", orderIDToUse, err)
	}

	color.Green("Ordre %s annulé avec succès", orderIDToUse)
	return data, nil
}

// GetExchangeInfo récupère les informations de l'échange
func (c *Client) GetExchangeInfo() ([]byte, error) {
	data, err := c.sendRequest("GET", "/api/v1/symbols", "")
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération des informations de l'échange: %w", err)
	}
	return data, nil
}

// GetAccountInfo récupère les informations du compte
func (c *Client) GetAccountInfo() ([]byte, error) {
	data, err := c.sendRequest("GET", "/api/v1/accounts", "")
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération des informations du compte: %w", err)
	}
	return data, nil
}

// GetDetailedBalances récupère les soldes détaillés du compte
func (c *Client) GetDetailedBalances() (map[string]common.DetailedBalance, error) {
	balances := make(map[string]common.DetailedBalance)

	// Récupérer les comptes de l'utilisateur
	data, err := c.sendRequest("GET", "/api/v1/accounts", "")
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération des soldes: %w", err)
	}

	// Décoder la réponse
	var accounts []accountInfo
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil, fmt.Errorf("erreur lors du décodage des comptes: %w", err)
	}

	// Traiter chaque compte
	for _, account := range accounts {
		if account.Currency == "USDC" || account.Currency == "BTC" {
			// Ne considérer que les comptes de trading
			if account.Type != "trade" {
				continue
			}

			balance, err := strconv.ParseFloat(account.Balance, 64)
			if err != nil {
				continue
			}

			available, err := strconv.ParseFloat(account.Available, 64)
			if err != nil {
				continue
			}

			locked := balance - available

			// Si la devise existe déjà, ajouter les montants
			if existingBalance, exists := balances[account.Currency]; exists {
				balances[account.Currency] = common.DetailedBalance{
					Free:   existingBalance.Free + available,
					Locked: existingBalance.Locked + locked,
					Total:  existingBalance.Total + balance,
				}
			} else {
				balances[account.Currency] = common.DetailedBalance{
					Free:   available,
					Locked: locked,
					Total:  balance,
				}
			}
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

// GetBalanceUSD récupère le solde en USDC
func (c *Client) GetBalanceUSD() float64 {
	color.Blue("Vérification du solde USDC sur KUCOIN...")

	// Récupérer les soldes détaillés
	balances, err := c.GetDetailedBalances()
	if err != nil {
		log.Fatalf("Erreur lors de la récupération des soldes: %v", err)
	}

	// Extraire le solde USDC
	usdcBalance := balances["USDC"].Free

	color.Green("Solde USDC sur KUCOIN: %.2f", usdcBalance)
	return usdcBalance
}

// CreateMakerOrder crée un ordre en mode maker
func (c *Client) CreateMakerOrder(side string, price float64, quantity string) ([]byte, error) {
	// Ajuster le prix pour s'assurer d'être maker
	var adjustedPrice float64
	if strings.ToUpper(side) == "BUY" {
		// Pour un achat, placer l'ordre légèrement en dessous du marché
		adjustedPrice = price * 0.998 // 0.2% en dessous
	} else {
		// Pour une vente, placer l'ordre légèrement au-dessus du marché
		adjustedPrice = price * 1.002 // 0.2% au-dessus
	}

	// Formater le prix selon les règles de précision de KuCoin
	adjustedPriceStr, err := c.FormatPrice("BTC-USDC", adjustedPrice)
	if err != nil {
		return nil, fmt.Errorf("erreur lors du formatage du prix: %w", err)
	}

	// Pour debug, afficher le prix formaté
	c.logDebug("Prix ajusté pour maker: %f -> %s", adjustedPrice, adjustedPriceStr)

	return c.CreateOrder(side, adjustedPriceStr, quantity)
}

func (c *Client) GetSymbolRules(symbol string) (SymbolRules, error) {
	// Vérifier d'abord le cache
	if rules, ok := symbolRulesCache[symbol]; ok {
		return rules, nil
	}

	// Si les règles ne sont pas en cache, les récupérer depuis l'API
	endpoint := "/api/v1/symbols"
	data, err := c.sendRequest("GET", endpoint, "")
	if err != nil {
		return SymbolRules{}, fmt.Errorf("erreur lors de la récupération des informations de l'échange: %w", err)
	}

	// Décoder la réponse qui contient toutes les paires de trading
	var symbols []map[string]interface{}
	if err := json.Unmarshal(data, &symbols); err != nil {
		return SymbolRules{}, fmt.Errorf("erreur lors du décodage des informations de l'échange: %w", err)
	}

	// Chercher le symbole spécifique
	for _, s := range symbols {
		if s["symbol"].(string) == symbol {
			rules := SymbolRules{
				Symbol:         s["symbol"].(string),
				BaseCurrency:   s["baseCurrency"].(string),
				QuoteCurrency:  s["quoteCurrency"].(string),
				PriceIncrement: parseFloat(s["priceIncrement"].(string)),
				BaseIncrement:  parseFloat(s["baseIncrement"].(string)),
				BaseMinSize:    parseFloat(s["baseMinSize"].(string)),
				BaseMaxSize:    parseFloat(s["baseMaxSize"].(string)),
				QuoteIncrement: parseFloat(s["quoteIncrement"].(string)),
				QuoteMinSize:   parseFloat(s["quoteMinSize"].(string)),
			}

			// Stocker dans le cache pour les prochaines utilisations
			symbolRulesCache[symbol] = rules
			return rules, nil
		}
	}

	return SymbolRules{}, fmt.Errorf("symbole %s non trouvé", symbol)
}

// parseFloat convertit une chaîne en float64 de manière sécurisée
func parseFloat(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

// FormatPrice formate un prix selon les règles de précision d'une paire de trading
func (c *Client) FormatPrice(symbol string, price float64) (string, error) {
	rules, err := c.GetSymbolRules(symbol)
	if err != nil {
		return "", err
	}

	// Calculer le nombre de décimales à partir de l'incrément de prix
	increment := rules.PriceIncrement
	precision := 0

	// Si l'incrément est 0, utiliser une précision par défaut
	if increment == 0 {
		precision = 2
	} else {
		// Convertir l'incrément en chaîne pour compter les décimales
		incrementStr := strconv.FormatFloat(increment, 'f', -1, 64)
		if i := strings.IndexByte(incrementStr, '.'); i >= 0 {
			precision = len(incrementStr) - i - 1
		}
	}

	// Arrondir le prix à la précision correcte
	factor := math.Pow10(precision)
	roundedPrice := math.Floor(price*factor) / factor

	// Formater le prix avec la précision correcte
	return strconv.FormatFloat(roundedPrice, 'f', precision, 64), nil
}
