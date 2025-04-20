package mexc

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"main/internal/database"
	"main/internal/exchanges/common"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/fatih/color"
)

// Client représente un client API pour l'exchange MEXC
type Client struct {
	APIKey    string
	APISecret string
	BaseURL   string
	Debug     bool // Mode debug pour afficher plus d'informations
}

// NewClient crée une nouvelle instance de client MEXC
func NewClient(apiKey, apiSecret string) *Client {
	return &Client{
		APIKey:    apiKey,
		APISecret: apiSecret,
		BaseURL:   "https://api.mexc.com",
		Debug:     false, // Activer le mode debug par défaut
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
		color.Blue("[DEBUG] "+format, args...)
	}
}

// Génère la signature HMAC SHA256 pour MEXC
func (c *Client) signRequest(queryString string) string {
	h := hmac.New(sha256.New, []byte(c.APISecret))
	h.Write([]byte(queryString))
	return hex.EncodeToString(h.Sum(nil))
}

// sendRequest envoie une requête HTTP à l'API MEXC
func (c *Client) sendRequest(method, endpoint, queryString string) ([]byte, error) {
	fullURL := fmt.Sprintf("%s%s?%s", c.BaseURL, endpoint, queryString)

	req, err := http.NewRequest(method, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la création de la requête: %w", err)
	}

	// CORRECTION: Selon la documentation officielle de MEXC, l'en-tête correct est "X-MEXC-APIKEY"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MEXC-APIKEY", c.APIKey)

	client := &http.Client{
		Timeout: 15 * time.Second, // Augmenter le timeout à 15 secondes
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de l'envoi de la requête: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la lecture de la réponse: %w", err)
	}

	// En cas d'erreur HTTP, inclure le corps de la réponse pour le diagnostic
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("erreur HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Vérifier si la réponse est une erreur de l'API
	if strings.Contains(string(body), "\"code\":") && strings.Contains(string(body), "\"msg\":") {
		var errorResp struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := json.Unmarshal(body, &errorResp); err == nil && errorResp.Code != 0 && errorResp.Code != 200 {
			return nil, fmt.Errorf("erreur API MEXC (code %d): %s", errorResp.Code, errorResp.Msg)
		}
	}

	return body, nil
}

// CheckConnection vérifie la connexion à l'API MEXC
func (c *Client) CheckConnection() error {
	_, err := c.sendRequest("GET", "/api/v3/ping", "")
	if err != nil {
		color.Red("Échec de connexion à MEXC: %v", err)
		return err
	}

	color.Green("Connexion à l'API MEXC réussie")

	return nil
}

// GetLastPriceBTC récupère le prix actuel du BTC
func (c *Client) GetLastPriceBTC() float64 {
	queryString := "symbol=BTCUSDC"
	body, err := c.sendRequest("GET", "/api/v3/ticker/price", queryString)
	if err != nil {
		log.Fatalf("Erreur lors de la récupération du prix BTC: %v", err)
	}

	priceStr, err := jsonparser.GetString(body, "price")
	if err != nil {
		log.Fatalf("Erreur lors de l'extraction du prix: %v", err)
	}

	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		log.Fatalf("Erreur lors de la conversion du prix: %v", err)
	}
	return price
}

// normalizeOrderId normalise un ID d'ordre MEXC
func (c *Client) normalizeOrderId(orderId string) string {
	// Si l'ID est vide, retourner une chaîne vide
	if orderId == "" {
		return ""
	}

	// Nettoyer l'ID en supprimant les espaces
	cleanedId := strings.TrimSpace(orderId)

	// Pour MEXC, les IDs peuvent avoir ou non le préfixe C02__
	// Si l'ID contient déjà ce préfixe, le conserver tel quel
	if strings.HasPrefix(cleanedId, "C02__") {
		return cleanedId
	}

	// Sinon, vérifier si c'est un nombre et ajouter le préfixe
	numericPattern := regexp.MustCompile("^[0-9]+$")
	if numericPattern.MatchString(cleanedId) {
		return "C02__" + cleanedId
	}

	// Si l'ID contient C02__ mais pas au début, corriger le format
	if strings.Contains(cleanedId, "C02__") {
		parts := strings.Split(cleanedId, "C02__")
		if len(parts) > 1 {
			return "C02__" + parts[1]
		}
	}

	// Dans le doute, retourner l'ID tel quel
	return cleanedId
}

// CreateOrder crée un nouvel ordre sur MEXC
func (c *Client) CreateOrder(side, price, quantity string) ([]byte, error) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)

	// Construire le query string avec tous les paramètres requis
	queryString := fmt.Sprintf(
		"symbol=BTCUSDC&side=%s&type=LIMIT&timeInForce=GTC&quantity=%s&price=%s&timestamp=%s",
		side, quantity, price, timestamp,
	)

	// Signer la requête
	signature := c.signRequest(queryString)
	signedQuery := fmt.Sprintf("%s&signature=%s", queryString, signature)

	// Envoyer la requête
	body, err := c.sendRequest("POST", "/api/v3/order", signedQuery)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de l'envoi de l'ordre: %w", err)
	}

	return body, nil
}

// GetOrderById récupère les informations d'un ordre spécifique
func (c *Client) GetOrderById(id string) ([]byte, error) {
	// Normaliser l'ID d'ordre
	normalizedId := c.normalizeOrderId(id)
	if normalizedId == "" {
		return nil, fmt.Errorf("ID d'ordre invalide: %s", id)
	}

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)

	// CHANGEMENT IMPORTANT: Pour les ordres de vente, vérifier d'abord l'historique des ordres
	// car les ordres complétés disparaissent des ordres actifs

	// 1. Vérifier d'abord l'historique des ordres (ordres complétés)
	queryString := fmt.Sprintf("symbol=BTCUSDC&timestamp=%s", timestamp)
	signature := c.signRequest(queryString)
	signedQuery := fmt.Sprintf("%s&signature=%s", queryString, signature)

	history, histErr := c.sendRequest("GET", "/api/v3/allOrders", signedQuery)
	if histErr == nil {
		var foundOrder []byte
		jsonparser.ArrayEach(history, func(order []byte, dataType jsonparser.ValueType, offset int, err error) {
			if err != nil {
				return
			}

			orderIdVal, _ := jsonparser.GetString(order, "orderId")
			if strings.Contains(orderIdVal, normalizedId) || strings.Contains(normalizedId, orderIdVal) ||
				strings.Contains(id, orderIdVal) || strings.Contains(orderIdVal, id) {
				foundOrder = order

				// Ajouter du debug pour voir les ordres trouvés
				status, _ := jsonparser.GetString(order, "status")
				c.logDebug("Ordre trouvé dans l'historique - ID: %s, Status: %s", orderIdVal, status)

				return
			}
		})

		if foundOrder != nil {
			// Modifier l'état si c'est un ordre complété dans l'historique
			status, err := jsonparser.GetString(foundOrder, "status")
			if err == nil && status != "FILLED" && status != "CANCELED" {
				c.logDebug("Ordre trouvé dans l'historique mais avec statut: %s, vérification supplémentaire", status)

				// Vérifier si l'ordre est potentiellement complété
				executedQty, err1 := jsonparser.GetString(foundOrder, "executedQty")
				origQty, err2 := jsonparser.GetString(foundOrder, "origQty")

				if err1 == nil && err2 == nil {
					executedQtyFloat, _ := strconv.ParseFloat(executedQty, 64)
					origQtyFloat, _ := strconv.ParseFloat(origQty, 64)

					if executedQtyFloat > 0 && executedQtyFloat >= origQtyFloat*0.99 {
						// L'ordre est effectivement exécuté, mais pas marqué comme FILLED
						// Créer une copie de l'ordre avec un statut FILLED
						var orderMap map[string]interface{}
						json.Unmarshal(foundOrder, &orderMap)
						orderMap["status"] = "FILLED" // Forcer le statut à FILLED

						modifiedOrder, _ := json.Marshal(orderMap)
						c.logDebug("Ordre modifié avec statut FILLED: %s", string(modifiedOrder))

						return modifiedOrder, nil
					}
				}
			}

			return foundOrder, nil
		}
	}

	// 2. Ensuite, vérifier les ordres actifs (comme avant)
	queryString = fmt.Sprintf("symbol=BTCUSDC&orderId=%s&timestamp=%s", normalizedId, timestamp)
	signature = c.signRequest(queryString)
	signedQuery = fmt.Sprintf("%s&signature=%s", queryString, signature)

	body, err := c.sendRequest("GET", "/api/v3/order", signedQuery)
	if err == nil {
		return body, nil
	}

	// 3. Si l'erreur est de type "Bad Request", essayer avec les ordres ouverts
	if strings.Contains(err.Error(), "400") {
		queryString = fmt.Sprintf("symbol=BTCUSDC&timestamp=%s", timestamp)
		signature = c.signRequest(queryString)
		signedQuery = fmt.Sprintf("%s&signature=%s", queryString, signature)

		allOrders, allErr := c.sendRequest("GET", "/api/v3/openOrders", signedQuery)
		if allErr == nil {
			var foundOrder []byte
			jsonparser.ArrayEach(allOrders, func(order []byte, dataType jsonparser.ValueType, offset int, err error) {
				if err != nil {
					return
				}

				orderIdVal, _ := jsonparser.GetString(order, "orderId")
				if strings.Contains(orderIdVal, normalizedId) || strings.Contains(normalizedId, orderIdVal) ||
					strings.Contains(id, orderIdVal) || strings.Contains(orderIdVal, id) {
					foundOrder = order
					return
				}
			})

			if foundOrder != nil {
				return foundOrder, nil
			}
		}
	}

	return nil, fmt.Errorf("impossible de trouver l'ordre avec ID %s: %w", id, err)
}

func (c *Client) IsFilled(order string) bool {
	// Activer temporairement le débogage
	debugState := c.Debug
	c.Debug = false
	defer func() { c.Debug = debugState }()

	c.logDebug("Vérification si l'ordre est rempli: %s", order)

	// 1. Vérifier le statut standard
	status, err := jsonparser.GetString([]byte(order), "status")
	if err == nil {
		c.logDebug("Statut trouvé: %s", status)
		if status == "FILLED" {
			// Pour MEXC, même si le statut est FILLED, nous devons vérifier que le solde est réellement disponible
			// Vérifier les quantités exécutées
			executedQty, err1 := jsonparser.GetString([]byte(order), "executedQty")
			origQty, err2 := jsonparser.GetString([]byte(order), "origQty")

			if err1 == nil && err2 == nil {
				c.logDebug("Quantités trouvées - exécutée: %s, originale: %s", executedQty, origQty)
				executedQtyFloat, err1 := strconv.ParseFloat(executedQty, 64)
				origQtyFloat, err2 := strconv.ParseFloat(origQty, 64)

				// Si l'exécution est complète selon les données de l'API
				if err1 == nil && err2 == nil && executedQtyFloat > 0 && executedQtyFloat >= origQtyFloat*0.99 {
					// Vérifier également le solde disponible
					balances, err := c.GetDetailedBalances()
					if err == nil {
						side, sideErr := jsonparser.GetString([]byte(order), "side")
						if sideErr == nil && side == "BUY" {
							// Pour un ordre d'achat, vérifier si le BTC est disponible
							availableBTC := balances["BTC"].Free
							c.logDebug("BTC disponible: %.8f - Ordre d'achat reporté comme complété", availableBTC)

							// Si le solde disponible est d'au moins 95% de la quantité d'origine
							if availableBTC >= origQtyFloat*0.95 {
								c.logDebug("Solde BTC suffisant, confirmation d'ordre FILLED")
								return true
							} else {
								c.logDebug("Solde BTC insuffisant (%.8f) pour %.8f BTC. Attente de mise à jour des soldes.",
									availableBTC, origQtyFloat)
								return false
							}
						} else if sideErr == nil && side == "SELL" {
							// Pour un ordre de vente, vérifier les USDC
							availableUSDC := balances["USDC"].Free
							expectedUSDC := origQtyFloat * 0.95 * c.GetLastPriceBTC()
							c.logDebug("USDC disponible: %.2f - Attendu environ: %.2f", availableUSDC, expectedUSDC)

							// Si le solde USDC a augmenté, l'ordre est probablement complété
							if availableUSDC > expectedUSDC {
								c.logDebug("Solde USDC suffisant, confirmation d'ordre FILLED")
								return true
							}

							// Si le solde n'a pas augmenté, l'ordre n'est probablement pas réellement exécuté
							c.logDebug("Solde USDC insuffisant, l'ordre n'est probablement pas réellement exécuté")
							return false
						}
					}

					// Si on ne peut pas vérifier les soldes, être conservateur
					c.logDebug("Impossible de vérifier les soldes, considérons l'ordre comme non rempli")
					return false
				}
			}

			// Par défaut pour MEXC, ne pas faire confiance au statut FILLED
			// à moins de confirmer avec les soldes
			c.logDebug("ATTENTION: Ordre marqué FILLED mais vérification impossible, considéré comme non rempli")
			return false
		}
	}

	// Autres vérifications inchangées...
	return false
}

// Ajout d'une nouvelle méthode pour attendre la mise à jour des soldes
func (c *Client) WaitForBalanceUpdate(cycle *database.Cycle, maxRetries int, delaySeconds int) bool {
	c.logDebug("Attente de la mise à jour des soldes pour le cycle %d", cycle.IdInt)

	for i := 0; i < maxRetries; i++ {
		balances, err := c.GetDetailedBalances()
		if err != nil {
			c.logDebug("Erreur lors de la récupération des soldes: %v", err)
			time.Sleep(time.Duration(delaySeconds) * time.Second)
			continue
		}

		availableBTC := balances["BTC"].Free
		c.logDebug("Tentative %d/%d - BTC disponible: %.8f pour cycle %.8f BTC",
			i+1, maxRetries, availableBTC, cycle.Quantity)

		if availableBTC >= cycle.Quantity*0.95 {
			c.logDebug("Soldes mis à jour avec succès!")
			return true
		}

		color.Yellow("MEXC: Solde toujours insuffisant, attente de %d secondes...", delaySeconds)
		time.Sleep(time.Duration(delaySeconds) * time.Second)
	}

	c.logDebug("Échec de mise à jour des soldes après %d tentatives", maxRetries)
	return false
}

// CancelOrder annule un ordre existant sur MEXC
func (c *Client) CancelOrder(orderID string) ([]byte, error) {

	// Pour MEXC, les IDs d'ordre doivent avoir le préfixe "C02__"
	// Vérifier si l'ID a déjà le préfixe
	orderIDToUse := orderID
	if !strings.HasPrefix(orderID, "C02__") {
		orderIDToUse = "C02__" + orderID
	}

	// Si l'ID contient "C02__" mais ce n'est pas au début, le corriger
	if strings.Contains(orderIDToUse, "C02__") && !strings.HasPrefix(orderIDToUse, "C02__") {
		parts := strings.Split(orderIDToUse, "C02__")
		if len(parts) > 1 {
			orderIDToUse = "C02__" + parts[1]
		}
	}

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)

	// Construction de la requête pour l'annulation
	queryString := fmt.Sprintf("symbol=BTCUSDC&orderId=%s&timestamp=%s", orderIDToUse, timestamp)
	signature := c.signRequest(queryString)
	signedQuery := fmt.Sprintf("%s&signature=%s", queryString, signature)

	// Envoyer la requête
	body, err := c.sendRequest("DELETE", "/api/v3/order", signedQuery)
	if err != nil {
		c.logDebug("Échec de l'annulation avec ID: %s - Erreur: %v", orderIDToUse, err)

		// Si l'erreur indique "Unknown order id", essayer sans le préfixe
		if strings.Contains(err.Error(), "Unknown order id") && strings.HasPrefix(orderIDToUse, "C02__") {
			orderIDWithoutPrefix := strings.TrimPrefix(orderIDToUse, "C02__")
			c.logDebug("Nouvel essai sans préfixe: %s", orderIDWithoutPrefix)

			queryString = fmt.Sprintf("symbol=BTCUSDC&orderId=%s&timestamp=%s", orderIDWithoutPrefix, timestamp)
			signature = c.signRequest(queryString)
			signedQuery = fmt.Sprintf("%s&signature=%s", queryString, signature)

			body, secondErr := c.sendRequest("DELETE", "/api/v3/order", signedQuery)
			if secondErr == nil {
				color.Green("Ordre %s annulé avec succès (sans préfixe)", orderIDWithoutPrefix)
				return body, nil
			}
			c.logDebug("Échec du second essai: %v", secondErr)
		}

		// Si toujours pas de succès, essayer avec juste les chiffres de l'ID
		re := regexp.MustCompile("[0-9]+")
		matches := re.FindAllString(orderID, -1)
		if len(matches) > 0 {
			numericID := matches[0]
			c.logDebug("Essai avec ID numérique uniquement: %s", numericID)

			queryString = fmt.Sprintf("symbol=BTCUSDC&orderId=%s&timestamp=%s", numericID, timestamp)
			signature = c.signRequest(queryString)
			signedQuery = fmt.Sprintf("%s&signature=%s", queryString, signature)

			body, thirdErr := c.sendRequest("DELETE", "/api/v3/order", signedQuery)
			if thirdErr == nil {
				color.Green("Ordre %s annulé avec succès (ID numérique)", numericID)
				return body, nil
			}
			c.logDebug("Échec du troisième essai: %v", thirdErr)
		}

		return nil, err
	}

	color.Green("Ordre %s annulé avec succès", orderIDToUse)
	return body, nil
}

// GetExchangeInfo récupère les informations de l'exchange
func (c *Client) GetExchangeInfo() ([]byte, error) {
	body, err := c.sendRequest("GET", "/api/v3/exchangeInfo", "")
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération des informations de l'exchange: %w", err)
	}
	return body, nil
}

// GetAccountInfo récupère les informations du compte
func (c *Client) GetAccountInfo() ([]byte, error) {
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	queryString := fmt.Sprintf("timestamp=%s", timestamp)
	signature := c.signRequest(queryString)
	signedQuery := fmt.Sprintf("%s&signature=%s", queryString, signature)

	body, err := c.sendRequest("GET", "/api/v3/account", signedQuery)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération des informations du compte: %w", err)
	}
	return body, nil
}

// GetDetailedBalances récupère les soldes détaillés du compte
func (c *Client) GetDetailedBalances() (map[string]common.DetailedBalance, error) {
	balances := make(map[string]common.DetailedBalance)

	timestamp := time.Now().UnixMilli()
	queryString := fmt.Sprintf("timestamp=%d", timestamp)
	signature := c.signRequest(queryString)
	signedQuery := fmt.Sprintf("%s&signature=%s", queryString, signature)

	body, err := c.sendRequest("GET", "/api/v3/account", signedQuery)
	if err != nil {
		return nil, fmt.Errorf("erreur lors de la récupération des soldes: %w", err)
	}

	// Extraire les soldes de la réponse JSON
	_, err = jsonparser.ArrayEach(body, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		if err != nil {
			c.logDebug("Erreur lors de l'analyse d'une entrée de solde: %v", err)
			return
		}

		asset, err := jsonparser.GetString(value, "asset")
		if err != nil {
			c.logDebug("Erreur lors de l'extraction du nom d'actif: %v", err)
			return
		}

		if asset == "USDC" || asset == "BTC" {
			freeStr, err1 := jsonparser.GetString(value, "free")
			lockedStr, err2 := jsonparser.GetString(value, "locked")

			if err1 != nil || err2 != nil {
				c.logDebug("Erreur lors de l'extraction des soldes pour %s: %v, %v", asset, err1, err2)
				return
			}

			free, err1 := strconv.ParseFloat(freeStr, 64)
			locked, err2 := strconv.ParseFloat(lockedStr, 64)

			if err1 != nil || err2 != nil {
				c.logDebug("Erreur lors de la conversion des soldes pour %s: %v, %v", asset, err1, err2)
				return
			}

			balances[asset] = common.DetailedBalance{
				Free:   free,
				Locked: locked,
				Total:  free + locked,
			}
		}
	}, "balances")

	if err != nil {
		return nil, fmt.Errorf("erreur lors de l'analyse des soldes: %w", err)
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
	color.Blue("Vérification du solde USDC sur MEXC...")

	timestamp := time.Now().UnixMilli()
	queryString := fmt.Sprintf("timestamp=%d", timestamp)
	signature := c.signRequest(queryString)
	signedQuery := fmt.Sprintf("%s&signature=%s", queryString, signature)

	body, err := c.sendRequest("GET", "/api/v3/account", signedQuery)
	if err != nil {
		log.Fatalf("Erreur lors de la récupération du solde: %v", err)
	}

	var freeFloat float64
	_, err = jsonparser.ArrayEach(body, func(value []byte, dataType jsonparser.ValueType, offset int, err2 error) {
		asset, err := jsonparser.GetString(value, "asset")
		if err != nil {
			return
		}

		if asset == "USDC" {
			freeStr, err := jsonparser.GetString(value, "free")
			if err != nil {
				return
			}

			free, err := strconv.ParseFloat(freeStr, 64)
			if err != nil {
				return
			}

			freeFloat = free
		}
	}, "balances")

	if err != nil {
		c.logDebug("Erreur lors de l'analyse des soldes USDC: %v", err)
	}

	color.Green("Solde USDC sur MEXC: %.2f", freeFloat)
	return freeFloat
}

// CreateMakerOrder crée un ordre en mode maker (prix ajusté pour s'assurer d'être dans le carnet d'ordres)
func (c *Client) CreateMakerOrder(side string, price float64, quantity string) ([]byte, error) {
	// Ajuster le prix pour s'assurer d'être maker
	var adjustedPrice float64
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

// DumpOrderInfo affiche les informations détaillées d'un ordre pour le débogage
func (c *Client) DumpOrderInfo(orderBytes []byte) {
	if c.Debug {

		// Tenter d'extraire et d'afficher le statut
		status, err := jsonparser.GetString(orderBytes, "status")
		if err == nil {
			color.Blue("Statut trouvé: %s", status)
		} else {
			color.Blue("Erreur lors de l'extraction du statut: %v", err)

			// Essayer de trouver où se trouve le statut réel
			var parsedOrder map[string]interface{}
			if json.Unmarshal(orderBytes, &parsedOrder) == nil {
				color.Blue("Structure de l'ordre:")
				for k, v := range parsedOrder {
					color.Blue("  %s: %v", k, v)
				}
			}
		}
		color.Blue("===========================")
	}
}
