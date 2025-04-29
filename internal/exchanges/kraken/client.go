// internal/exchanges/kraken/client.go
package kraken

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"main/internal/exchanges/common"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
)

// Constantes pour l'API Kraken
const (
	apiURL     = "https://api.kraken.com"
	apiVersion = "0"
)

// Client représente un client API pour l'exchange Kraken
type Client struct {
	APIKey    string
	APISecret string
	BaseURL   string
	Debug     bool
}

// Structure de réponse standardisée de Kraken
type krakenResponse struct {
	Error  []string        `json:"error"`
	Result json.RawMessage `json:"result"`
}

// NewClient crée une nouvelle instance de client Kraken
func NewClient(apiKey, apiSecret string) *Client {
	return &Client{
		APIKey:    apiKey,
		APISecret: apiSecret,
		BaseURL:   apiURL,
		Debug:     false,
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

// logDebug affiche un message de debug si le mode debug est activé
func (c *Client) logDebug(format string, args ...interface{}) {
	if c.Debug {
		color.Blue("[DEBUG KRAKEN] "+format, args...)
	}
}

// signature crée une signature HMAC pour authentifier les requêtes à l'API Kraken
func (c *Client) signature(endpoint string, values url.Values) string {
	// Concaténer les données à signer : nonce + données POST
	payload := values.Encode()

	// Calculer correctement le SHA256 du nonce + payload
	h := sha256.New()
	h.Write([]byte(values.Get("nonce") + payload))
	shaSum := h.Sum(nil)

	// Créer le message à signer : endpoint + SHA256(nonce + payload)
	message := endpoint + string(shaSum)

	// Décoder la clé secrète de base64
	secret, err := base64.StdEncoding.DecodeString(c.APISecret)
	if err != nil {
		c.logDebug("Erreur lors du décodage de la clé secrète: %v", err)
		return ""
	}

	// Calculer la signature HMAC-SHA512
	h2 := hmac.New(sha512.New, secret)
	h2.Write([]byte(message))

	// Encoder la signature en base64
	return base64.StdEncoding.EncodeToString(h2.Sum(nil))
}

// sendPublicRequest envoie une requête publique (non-authentifiée) à l'API Kraken
func (c *Client) sendPublicRequest(method, endpoint string, params url.Values) ([]byte, error) {
	fullURL := fmt.Sprintf("%s/%s/public/%s", c.BaseURL, apiVersion, endpoint)

	var req *http.Request
	var err error

	if method == "GET" {
		if params != nil {
			fullURL = fmt.Sprintf("%s?%s", fullURL, params.Encode())
		}
		req, err = http.NewRequest(method, fullURL, nil)
	} else {
		req, err = http.NewRequest(method, fullURL, strings.NewReader(params.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	if err != nil {
		return nil, fmt.Errorf("erreur lors de la création de la requête: %w", err)
	}

	c.logDebug("%s %s", method, fullURL)

	// Exécuter la requête
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de l'envoi de la requête: %w", err)
	}
	defer resp.Body.Close()

	// Lire la réponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la lecture de la réponse: %w", err)
	}

	c.logDebug("Réponse: %s", string(body))

	// Vérifier le code de statut HTTP
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erreur HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Parser la réponse Kraken standard
	var response krakenResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("erreur lors du parsing de la réponse: %w", err)
	}

	// Vérifier si Kraken a retourné des erreurs
	if len(response.Error) > 0 {
		return nil, fmt.Errorf("erreur API Kraken: %s", strings.Join(response.Error, ", "))
	}

	return response.Result, nil
}

// sendPrivateRequest envoie une requête privée (authentifiée) à l'API Kraken
func (c *Client) sendPrivateRequest(endpoint string, params url.Values) ([]byte, error) {
	if params == nil {
		params = url.Values{}
	}

	// Ajouter le nonce
	params.Set("nonce", fmt.Sprintf("%d", time.Now().UnixNano()))

	// Préparer la requête
	fullURL := fmt.Sprintf("%s/%s/private/%s", c.BaseURL, apiVersion, endpoint)
	req, err := http.NewRequest("POST", fullURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la création de la requête: %w", err)
	}

	// Ajouter les en-têtes d'authentification
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("API-Key", c.APIKey)
	req.Header.Set("API-Sign", c.signature("/"+apiVersion+"/private/"+endpoint, params))

	c.logDebug("POST %s", fullURL)
	c.logDebug("Payload: %s", params.Encode())

	// Exécuter la requête
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de l'envoi de la requête: %w", err)
	}
	defer resp.Body.Close()

	// Lire la réponse
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la lecture de la réponse: %w", err)
	}

	c.logDebug("Réponse: %s", string(body))

	// Vérifier le code de statut HTTP
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erreur HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Parser la réponse Kraken standard
	var response krakenResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("erreur lors du parsing de la réponse: %w", err)
	}

	// Vérifier si Kraken a retourné des erreurs
	if len(response.Error) > 0 {
		return nil, fmt.Errorf("erreur API Kraken: %s", strings.Join(response.Error, ", "))
	}

	return response.Result, nil
}

// CheckConnection vérifie la connexion à l'API Kraken
func (c *Client) CheckConnection() error {
	// Utiliser une requête publique simple pour vérifier la connexion
	_, err := c.sendPublicRequest("GET", "Time", nil)
	if err != nil {
		color.Red("Échec de connexion à Kraken: %v", err)
		return err
	}

	// Vérifier également que les clés API fonctionnent en faisant une requête privée simple
	if c.APIKey != "" && c.APISecret != "" {
		_, err = c.sendPrivateRequest("Balance", nil)
		if err != nil {
			color.Red("Échec de l'authentification à Kraken: %v", err)
			return err
		}
	}

	color.Green("Connexion à l'API KRAKEN réussie")
	return nil
}

// GetLastPriceBTC récupère le prix actuel du BTC
func (c *Client) GetLastPriceBTC() float64 {
	// Créer les paramètres pour la requête
	params := url.Values{}
	params.Set("pair", "XBTUSDC") // XBT est le code de Kraken pour BTC

	// Envoyer la requête
	data, err := c.sendPublicRequest("GET", "Ticker", params)
	if err != nil {
		color.Red("Erreur lors de la récupération du prix BTC: %v", err)
		return 0
	}

	// Analyser la réponse
	var ticker map[string]map[string]json.RawMessage
	if err := json.Unmarshal(data, &ticker); err != nil {
		color.Red("Erreur lors du parsing du ticker: %v", err)
		return 0
	}

	// Extraction du prix
	for _, v := range ticker {
		var price []string
		if err := json.Unmarshal(v["c"], &price); err != nil {
			color.Red("Erreur lors de l'extraction du prix: %v", err)
			return 0
		}

		if len(price) > 0 {
			p, err := strconv.ParseFloat(price[0], 64)
			if err != nil {
				color.Red("Erreur lors de la conversion du prix: %v", err)
				return 0
			}
			return p
		}
	}

	color.Red("Prix BTC non trouvé dans la réponse")
	return 0
}

// GetDetailedBalances récupère les soldes détaillés du compte
func (c *Client) GetDetailedBalances() (map[string]common.DetailedBalance, error) {
	// Initialiser la map des soldes
	balances := make(map[string]common.DetailedBalance)

	// Récupérer les soldes
	data, err := c.sendPrivateRequest("Balance", nil)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération des soldes: %w", err)
	}

	// Analyser la réponse
	var balanceData map[string]string
	if err := json.Unmarshal(data, &balanceData); err != nil {
		return nil, fmt.Errorf("erreur lors du parsing des soldes: %w", err)
	}

	// Récupérer les informations sur les ordres ouverts pour calculer les montants bloqués
	openOrdersData, err := c.sendPrivateRequest("OpenOrders", nil)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération des ordres ouverts: %w", err)
	}

	var openOrders struct {
		Open map[string]struct {
			Status  string            `json:"status"`
			Vol     string            `json:"vol"`
			VolExec string            `json:"vol_exec"`
			Descr   map[string]string `json:"descr"`
		} `json:"open"`
	}

	if err := json.Unmarshal(openOrdersData, &openOrders); err != nil {
		return nil, fmt.Errorf("erreur lors du parsing des ordres ouverts: %w", err)
	}

	// Calculer les montants bloqués par devise
	lockedAmounts := make(map[string]float64)

	// Logique corrigée pour déterminer les montants bloqués
	for _, order := range openOrders.Open {
		if order.Status == "open" {
			pair := order.Descr["pair"]
			orderType := order.Descr["type"] // "buy" ou "sell"
			vol, _ := strconv.ParseFloat(order.Vol, 64)
			volExec, _ := strconv.ParseFloat(order.VolExec, 64)
			remainingVol := vol - volExec

			// Vérifier spécifiquement pour la paire BTC/USDC (XBTUSDC chez Kraken)
			if pair == "XBTUSDC" {
				price, _ := strconv.ParseFloat(order.Descr["price"], 64)

				if orderType == "buy" {
					// Pour un ordre d'achat de BTC, les USDC sont bloqués
					// Le montant bloqué est: prix * volume restant
					lockedAmount := price * remainingVol
					lockedAmounts["USDC"] += lockedAmount
				} else if orderType == "sell" {
					// Pour un ordre de vente de BTC, les BTC sont bloqués
					lockedAmounts["XBT"] += remainingVol
				}
			} else {
				// Pour les autres paires, essayer de déterminer logiquement
				if strings.HasPrefix(pair, "XBT") {
					// Paires commençant par XBT (BTC)
					if orderType == "sell" {
						lockedAmounts["XBT"] += remainingVol
					}
				} else if strings.HasSuffix(pair, "XBT") {
					// Paires se terminant par XBT
					if orderType == "buy" {
						lockedAmounts["XBT"] += remainingVol
					}
				} else if strings.HasPrefix(pair, "USDC") || strings.HasSuffix(pair, "USDC") {
					// Paires impliquant USDC
					if (strings.HasPrefix(pair, "USDC") && orderType == "sell") ||
						(strings.HasSuffix(pair, "USDC") && orderType == "buy") {
						price, _ := strconv.ParseFloat(order.Descr["price"], 64)
						lockedAmounts["USDC"] += price * remainingVol
					}
				}
			}
		}
	}

	// Traiter chaque solde pour le format commun
	for asset, balanceStr := range balanceData {
		// Convertir le code d'actif Kraken vers le format standard
		standardAsset := asset
		if asset == "XXBT" {
			standardAsset = "BTC"
		} else if asset == "USDC" {
			standardAsset = "USDC"
		} else {
			continue // On ignore les autres actifs
		}

		// Convertir le solde en float
		total, err := strconv.ParseFloat(balanceStr, 64)
		if err != nil {
			continue
		}

		// Déterminer les montants libres et bloqués
		// Pour XBT/BTC
		var locked float64
		if asset == "XXBT" {
			locked = lockedAmounts["XBT"]
		} else if asset == "USDC" {
			locked = lockedAmounts["USDC"]
		} else {
			locked = lockedAmounts[asset]
		}

		free := total - locked

		// S'assurer que les valeurs ne sont pas négatives
		if free < 0 {
			free = 0
		}
		if locked > total {
			locked = total
		}

		balances[standardAsset] = common.DetailedBalance{
			Free:   free,
			Locked: locked,
			Total:  total,
		}
	}

	// S'assurer que BTC et USDC existent dans la réponse
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
	color.Blue("Vérification du solde USDC sur KRAKEN...")

	balances, err := c.GetDetailedBalances()
	if err != nil {
		color.Red("Erreur lors de la récupération des soldes: %v", err)
		return 0
	}

	usdcBalance := balances["USDC"].Free

	color.Green("Solde USDC sur KRAKEN: %.2f", usdcBalance)
	return usdcBalance
}

// CreateOrder crée un nouvel ordre sur Kraken
func (c *Client) CreateOrder(side, price, quantity string) ([]byte, error) {
	// Convertir la quantité en float pour manipulation précise
	quantityFloat, err := strconv.ParseFloat(quantity, 64)
	if err != nil {
		return nil, fmt.Errorf("quantité invalide: %w", err)
	}

	// Récupérer les soldes pour vérification précise
	balances, err := c.GetDetailedBalances()
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération des soldes: %w", err)
	}

	// Vérifier le solde disponible
	var availableBalance float64
	if side == "SELL" {
		availableBalance = balances["BTC"].Free
	} else if side == "BUY" {
		availableBalance = balances["USDC"].Free
	} else {
		return nil, fmt.Errorf("côté de l'ordre non supporté: %s (doit être BUY ou SELL)", side)
	}

	// Ajuster la quantité si nécessaire
	const tolerancePercent = 0.99 // Tolérance de 1% pour gérer les imprécisions
	if quantityFloat > availableBalance*tolerancePercent {
		// Ajuster la quantité
		adjustedQuantity := availableBalance * tolerancePercent
		quantity = strconv.FormatFloat(adjustedQuantity, 'f', 8, 64)

		color.Yellow("Ajustement de la quantité: %.8f → %.8f (solde disponible)", quantityFloat, adjustedQuantity)
	}

	// Adapter le side pour Kraken (buy/sell)
	krakenSide := strings.ToLower(side)

	// Créer les paramètres pour la requête
	params := url.Values{}
	params.Set("pair", "XBTUSDC")
	params.Set("type", krakenSide)
	params.Set("ordertype", "limit")
	params.Set("price", price)
	params.Set("volume", quantity)

	// Pour s'assurer d'être maker, on ajoute le paramètre post-only
	params.Set("oflags", "post")

	// Envoyer la requête
	data, err := c.sendPrivateRequest("AddOrder", params)
	if err != nil {
		// Gérer spécifiquement les erreurs de fonds insuffisants
		if strings.Contains(err.Error(), "Insufficient funds") {
			return nil, fmt.Errorf("fonds insuffisants: vérifiez votre solde disponible (err: %v)", err)
		}
		return nil, fmt.Errorf("erreur lors de la création de l'ordre: %w", err)
	}

	// Convertir la réponse au format attendu par le système
	var addOrderResponse struct {
		TxID  []string          `json:"txid"`
		Descr map[string]string `json:"descr"`
	}

	if err := json.Unmarshal(data, &addOrderResponse); err != nil {
		return nil, fmt.Errorf("erreur lors du parsing de la réponse: %w", err)
	}

	// Créer une réponse standardisée avec l'ID de l'ordre
	if len(addOrderResponse.TxID) > 0 {
		standardResponse := map[string]interface{}{
			"orderId": addOrderResponse.TxID[0],
			"status":  "created",
		}

		jsonResponse, err := json.Marshal(standardResponse)
		if err != nil {
			return nil, fmt.Errorf("erreur lors de la création de la réponse: %w", err)
		}

		return jsonResponse, nil
	}

	return nil, fmt.Errorf("aucun ID d'ordre retourné par Kraken")
}

// GetOrderById récupère les informations d'un ordre spécifique
func (c *Client) GetOrderById(id string) ([]byte, error) {
	// Créer les paramètres pour la requête
	params := url.Values{}
	params.Set("txid", id)

	// Essayer d'abord avec les ordres ouverts
	data, err := c.sendPrivateRequest("QueryOrders", params)
	if err != nil {
		// Si l'ordre n'est pas trouvé dans les ordres ouverts, vérifier les ordres fermés
		closedParams := url.Values{}
		closedParams.Set("txid", id)
		closedParams.Set("trades", "true")

		closedData, closedErr := c.sendPrivateRequest("QueryOrders", closedParams)
		if closedErr != nil {
			return nil, fmt.Errorf("erreur lors de la récupération de l'ordre %s: %w", id, err)
		}

		data = closedData
	}

	// Convertir la réponse pour qu'elle soit conforme au format attendu par le système
	var orderData map[string]map[string]interface{}
	if err := json.Unmarshal(data, &orderData); err != nil {
		return nil, fmt.Errorf("erreur lors du parsing de l'ordre: %w", err)
	}

	// Créer une réponse standardisée qui fonctionne avec le reste du système
	for txid, orderDetails := range orderData {
		status := orderDetails["status"].(string)

		// Convertir l'ordre Kraken en format standardisé
		standardOrder := map[string]interface{}{
			"orderId":  txid,
			"status":   status,
			"price":    orderDetails["price"],
			"quantity": orderDetails["vol"],
			"executed": orderDetails["vol_exec"],
		}

		jsonResponse, err := json.Marshal(standardOrder)
		if err != nil {
			return nil, fmt.Errorf("erreur lors de la création de la réponse: %w", err)
		}

		return jsonResponse, nil
	}

	return nil, fmt.Errorf("ordre %s non trouvé", id)
}

// IsFilled vérifie si un ordre est complètement exécuté
func (c *Client) IsFilled(order string) bool {
	var orderData map[string]interface{}
	if err := json.Unmarshal([]byte(order), &orderData); err != nil {
		c.logDebug("Erreur lors du parsing de l'ordre: %v", err)
		return false
	}

	// Vérifier si l'ordre est rempli selon le format standardisé
	status, hasStatus := orderData["status"].(string)
	if hasStatus && (status == "closed" || status == "filled") {
		return true
	}

	// Vérifier si l'ordre est complètement exécuté en comparant les quantités
	executed, hasExecuted := orderData["executed"].(string)
	quantity, hasQuantity := orderData["quantity"].(string)

	if hasExecuted && hasQuantity {
		executedFloat, err1 := strconv.ParseFloat(executed, 64)
		quantityFloat, err2 := strconv.ParseFloat(quantity, 64)

		if err1 == nil && err2 == nil && quantityFloat > 0 {
			// Si la quantité exécutée est pratiquement égale à la quantité totale (marge d'erreur de 1%)
			if executedFloat >= quantityFloat*0.99 {
				return true
			}
		}
	}

	return false
}

// CancelOrder annule un ordre existant sur Kraken
func (c *Client) CancelOrder(orderID string) ([]byte, error) {
	// Créer les paramètres pour la requête
	params := url.Values{}
	params.Set("txid", orderID)

	// Envoyer la requête
	_, err := c.sendPrivateRequest("CancelOrder", params)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de l'annulation de l'ordre %s: %w", orderID, err)
	}

	color.Green("Ordre %s annulé avec succès", orderID)

	// Créer une réponse standardisée
	response := map[string]interface{}{
		"orderId": orderID,
		"status":  "cancelled",
	}

	jsonResponse, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la création de la réponse: %w", err)
	}

	return jsonResponse, nil
}

// GetExchangeInfo récupère les informations de l'exchange
func (c *Client) GetExchangeInfo() ([]byte, error) {
	// Créer les paramètres pour la requête
	params := url.Values{}
	params.Set("pair", "XBTUSDC")

	// Envoyer la requête
	data, err := c.sendPublicRequest("GET", "AssetPairs", params)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération des informations de l'exchange: %w", err)
	}

	return data, nil
}

// GetAccountInfo récupère les informations du compte
func (c *Client) GetAccountInfo() ([]byte, error) {
	// Cette fonction peut être utilisée pour récupérer diverses informations sur le compte
	data, err := c.sendPrivateRequest("Balance", nil)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération des informations du compte: %w", err)
	}

	return data, nil
}

// CreateMakerOrder crée un ordre en mode maker
func (c *Client) CreateMakerOrder(side string, price float64, quantity string) ([]byte, error) {
	// Convertir la quantité en float pour les calculs
	quantityFloat, err := strconv.ParseFloat(quantity, 64)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la conversion de la quantité: %w", err)
	}

	var adjustedPrice float64
	if strings.ToUpper(side) == "BUY" {
		// Pour un achat, placer l'ordre légèrement en dessous du marché
		adjustedPrice = price * 0.998 // 0.2% en dessous
	} else {
		// Pour une vente, nous devons prendre en compte les frais

		// Taux de frais maker de Kraken (0.26% pour les niveaux de base)
		const makerFeeRate = 0.0026

		// Estimer les frais d'achat déjà payés
		buyFees := price * quantityFloat * makerFeeRate

		// Estimer les frais de vente à venir
		sellFees := price * quantityFloat * makerFeeRate

		// Total des frais à couvrir
		totalFeesToCover := buyFees + sellFees

		// Ajouter une marge de sécurité de 10%
		totalFeesToCover *= 1.1

		// Calculer l'ajustement de prix nécessaire par unité
		feeAdjustmentPerUnit := totalFeesToCover / quantityFloat

		// Prix minimum pour couvrir les frais
		minProfitablePrice := price + feeAdjustmentPerUnit

		// Prix maker standard (0.2% au-dessus)
		standardPrice := price * 1.002

		// Obtenir le prix actuel du marché
		currentPrice := c.GetLastPriceBTC()

		// Prix maker basé sur le prix actuel du marché
		marketBasedPrice := currentPrice * 1.001 // 0.1% au-dessus du prix actuel

		// Logique pour choisir le prix final:
		// 1. Le prix doit au moins couvrir les frais (minProfitablePrice)
		// 2. Il doit être suffisant pour être un ordre maker (marketBasedPrice)
		// 3. Il doit respecter l'offset standard s'il est plus élevé (standardPrice)

		// Prendre le maximum des trois prix
		adjustedPrice = math.Max(minProfitablePrice, math.Max(marketBasedPrice, standardPrice))

		c.logDebug("Calcul du prix de vente Kraken:")
		c.logDebug("Prix d'achat: %.2f USDC", price)
		c.logDebug("Prix actuel du marché: %.2f USDC", currentPrice)
		c.logDebug("Frais d'achat estimés: %.8f USDC", buyFees)
		c.logDebug("Frais de vente estimés: %.8f USDC", sellFees)
		c.logDebug("Ajustement pour frais: %.8f USDC", feeAdjustmentPerUnit)
		c.logDebug("Prix minimum rentable: %.2f USDC", minProfitablePrice)
		c.logDebug("Prix maker standard: %.2f USDC", standardPrice)
		c.logDebug("Prix basé sur le marché: %.2f USDC", marketBasedPrice)
		c.logDebug("Prix final ajusté: %.2f USDC", adjustedPrice)
	}

	// Formater le prix avec précision
	adjustedPriceStr := c.formatPrice(adjustedPrice)

	// Créer l'ordre avec le prix ajusté
	return c.CreateOrder(side, adjustedPriceStr, quantity)
}

// formatPrice formate un prix avec la précision appropriée pour Kraken
func (c *Client) formatPrice(price float64) string {
	// Kraken utilise généralement une précision de 1 décimale pour les prix BTC/USDC
	// mais cela peut varier, donc nous utilisons 2 décimales pour être sûrs
	return strconv.FormatFloat(math.Floor(price*100)/100, 'f', 2, 64)
}

// GetOrderFees récupère les frais appliqués à un ordre spécifique
func (c *Client) GetOrderFees(orderId string) (float64, error) {
	// Créer les paramètres pour la requête
	params := url.Values{}
	params.Set("txid", orderId)
	params.Set("trades", "true") // Inclure les trades associés pour obtenir les frais

	// Envoyer la requête pour obtenir les détails de l'ordre
	data, err := c.sendPrivateRequest("QueryOrders", params)
	if err != nil {
		return 0, fmt.Errorf("erreur lors de la récupération des détails de l'ordre: %w", err)
	}

	// Extraire les frais de la réponse
	// La réponse de Kraken contient les ordres sous forme de map avec l'ID comme clé
	var orderFees float64

	err = json.Unmarshal(data, &map[string]json.RawMessage{})
	if err != nil {
		return 0, fmt.Errorf("erreur lors du parsing des données d'ordre: %w", err)
	}

	// Comme la réponse est une map, nous devons itérer
	for _, orderDetails := range map[string]json.RawMessage{} {
		// Extraire les frais
		var order struct {
			Fee string `json:"fee"`
		}

		if err := json.Unmarshal(orderDetails, &order); err == nil && order.Fee != "" {
			orderFees, _ = strconv.ParseFloat(order.Fee, 64)
			if orderFees > 0 {
				return orderFees, nil
			}
		}
	}

	// Si les frais n'ont pas été trouvés dans les détails de l'ordre,
	// essayer d'obtenir l'historique des trades
	params = url.Values{}
	params.Set("txid", orderId)

	tradesData, err := c.sendPrivateRequest("TradesHistory", params)
	if err != nil {
		// Si nous ne pouvons pas obtenir les trades, estimer les frais
		return c.estimateOrderFees(orderId)
	}

	// Analyser les trades pour obtenir les frais
	var trades struct {
		Trades map[string]struct {
			Fee       string `json:"fee"`
			OrderTxid string `json:"ordertxid"`
		} `json:"trades"`
	}

	if err := json.Unmarshal(tradesData, &trades); err == nil {
		var totalFees float64

		for _, trade := range trades.Trades {
			if trade.OrderTxid == orderId {
				if fee, err := strconv.ParseFloat(trade.Fee, 64); err == nil {
					totalFees += fee
				}
			}
		}

		if totalFees > 0 {
			return totalFees, nil
		}
	}

	// En dernier recours, estimer les frais
	return c.estimateOrderFees(orderId)
}

// estimateOrderFees estime les frais d'un ordre à partir de son ID
func (c *Client) estimateOrderFees(orderId string) (float64, error) {
	// Pour Kraken, le taux de frais maker standard est 0.26%
	const makerFeeRate = 0.0026

	// Récupérer les détails de l'ordre
	params := url.Values{}
	params.Set("txid", orderId)

	orderData, err := c.sendPrivateRequest("QueryOrders", params)
	if err != nil {
		return 0, fmt.Errorf("erreur lors de la récupération des détails de l'ordre: %w", err)
	}

	// Analyser les détails pour estimer les frais
	var orders map[string]struct {
		Price     string `json:"price"`
		Volume    string `json:"vol"`
		VolumeExe string `json:"vol_exec"`
	}

	if err := json.Unmarshal(orderData, &orders); err != nil {
		return 0, fmt.Errorf("erreur lors du parsing des détails de l'ordre: %w", err)
	}

	// Pour chaque ordre (normalement un seul)
	for _, order := range orders {
		price, err1 := strconv.ParseFloat(order.Price, 64)
		volume, err2 := strconv.ParseFloat(order.VolumeExe, 64)

		if err1 == nil && err2 == nil && price > 0 && volume > 0 {
			// Calculer les frais estimés
			return price * volume * makerFeeRate, nil
		}
	}

	return 0, fmt.Errorf("impossible d'estimer les frais d'ordre")
}

// AdjustSellPriceForFees ajuste le prix de vente pour prendre en compte les frais de Kraken
func (c *Client) AdjustSellPriceForFees(buyPrice float64, quantity float64, buyOrderId string) (float64, error) {
	// Récupérer les frais réels de l'ordre d'achat si possible
	buyFees, err := c.GetOrderFees(buyOrderId)

	// Si on ne peut pas récupérer les frais, estimer avec le taux standard
	if err != nil || buyFees <= 0 {
		// Taux de frais maker de Kraken (0.26%)
		const makerFeeRate = 0.0026
		buyFees = buyPrice * quantity * makerFeeRate
	}

	// Multiplier par 2 pour couvrir les frais de vente également
	totalFeesToCover := buyFees * 2

	// Ajouter une marge de sécurité de 10% pour Kraken qui a des frais plus élevés
	totalFeesToCover *= 1.1

	// Calculer l'ajustement de prix par unité
	feeAdjustmentPerUnit := totalFeesToCover / quantity

	// Calculer le prix minimum pour être rentable
	minProfitablePrice := buyPrice + feeAdjustmentPerUnit

	c.logDebug("Calcul du prix de vente pour couvrir les frais Kraken:")
	c.logDebug("Prix d'achat: %.2f USDC", buyPrice)
	c.logDebug("Frais d'achat: %.8f USDC", buyFees)
	c.logDebug("Frais totaux à couvrir: %.8f USDC", totalFeesToCover)
	c.logDebug("Ajustement par unité: %.8f USDC", feeAdjustmentPerUnit)
	c.logDebug("Prix minimal rentable: %.2f USDC", minProfitablePrice)

	return minProfitablePrice, nil
}

func (c *Client) GetOpenOrders() ([]byte, error) {
	// Créer la requête
	params := url.Values{}

	// Envoyer la requête
	data, err := c.sendPrivateRequest("OpenOrders", params)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération des ordres ouverts: %w", err)
	}

	return data, nil
}
