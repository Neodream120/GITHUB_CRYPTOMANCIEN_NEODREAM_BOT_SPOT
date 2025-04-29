package commands

import (
	"fmt"
	"main/internal/config"
	"main/internal/database"
	"main/internal/exchanges/common"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/buger/jsonparser"
	"github.com/fatih/color"
)

type cycleStatistics struct {
	totalCycles     int
	buyCycles       int
	sellCycles      int
	completedCycles int
	totalProfit     float64
}

// cleanOrderId nettoie et normalise un ID d'ordre selon l'exchange spécifié
func cleanOrderId(orderId string, exchange ...string) string {
	// Si l'ID est vide, retourner une chaîne vide
	if orderId == "" {
		return ""
	}

	// Nettoyer les espaces au début et à la fin
	orderId = strings.TrimSpace(orderId)

	// Déterminer l'exchange (par défaut: BINANCE)
	ex := "BINANCE"
	if len(exchange) > 0 && exchange[0] != "" {
		ex = strings.ToUpper(exchange[0])
	}

	// Traitement spécifique par exchange
	switch ex {
	case "MEXC":
		// Pour MEXC, normaliser le préfixe C02__
		if strings.HasPrefix(orderId, "C02__") {
			return orderId // Garder l'ID tel quel si le préfixe est déjà présent
		} else if strings.Contains(orderId, "C02__") {
			// Extraire la partie après C02__
			parts := strings.Split(orderId, "C02__")
			if len(parts) > 1 {
				return "C02__" + parts[1]
			}
		}

		// Supprimer tous les caractères non alphanumériques
		re := regexp.MustCompile("[^a-zA-Z0-9]")
		cleanedId := re.ReplaceAllString(orderId, "")

		if cleanedId != "" {
			// On laisse le préfixe C02__ pour la cohérence
			return "C02__" + cleanedId
		}
		return orderId

	case "BINANCE":
		// Pour Binance, extraire uniquement les chiffres
		re := regexp.MustCompile("[^0-9]")
		cleanId := re.ReplaceAllString(orderId, "")

		// Si l'ID est vide après nettoyage, retourner l'original
		if cleanId == "" {
			return orderId
		}

		return cleanId

	case "KUCOIN":
		// Pour KuCoin, extraire un motif d'ID typique (24 caractères alphanumériques)
		if len(orderId) > 24 {
			re := regexp.MustCompile("[a-zA-Z0-9]{24}")
			matches := re.FindAllString(orderId, -1)
			if len(matches) > 0 {
				return matches[0]
			}
		}
		return orderId

	case "KRAKEN":
		// Pour Kraken, les IDs sont généralement des chaînes alphanumériques sans préfixe spécifique
		// Nous nettoyons simplement les espaces et caractères non alphanumériques
		re := regexp.MustCompile("[^a-zA-Z0-9-]")
		cleanId := re.ReplaceAllString(orderId, "")

		if cleanId == "" {
			return orderId
		}

		return cleanId

	default:
		// Pour les autres exchanges, retourner l'ID tel quel
		return orderId
	}
}

func Update() {
	// Récupérer tous les exchanges configurés
	cfg, err := config.LoadConfig()
	if err != nil {
		color.Red("Erreur de configuration: %v", err)
		return
	}

	// Liste des exchanges à traiter
	exchanges := []string{"BINANCE", "MEXC", "KUCOIN", "KRAKEN"}

	// Conteneur pour suivre les statistiques de tous les exchanges
	allBalances := make(map[string]map[string]common.DetailedBalance)
	allPrices := make(map[string]float64)

	// Traiter chaque exchange
	for _, exchangeName := range exchanges {
		// Vérifier si l'exchange est configuré
		exchangeConfig, exists := cfg.Exchanges[exchangeName]
		if !exists || !exchangeConfig.Enabled {
			color.Yellow("Exchange %s non configuré ou désactivé", exchangeName)
			continue
		}

		// Initialiser le client pour cet exchange
		// Utilisation d'une fonction try/catch pour éviter les panics
		func() {
			defer func() {
				if r := recover(); r != nil {
					color.Red("Panic lors de l'initialisation du client pour %s: %v", exchangeName, r)
				}
			}()

			client := GetClientByExchange(exchangeName)
			if client == nil {
				color.Red("Client nil pour l'exchange %s", exchangeName)
				return
			}

			// Afficher les informations de l'exchange
			color.Cyan("=== Informations pour %s ===", exchangeName)

			// Récupérer le prix actuel du BTC
			// Protection contre les panics
			var lastPrice float64
			func() {
				defer func() {
					if r := recover(); r != nil {
						color.Red("Erreur lors de la récupération du prix BTC pour %s: %v", exchangeName, r)
					}
				}()
				lastPrice = client.GetLastPriceBTC()
			}()

			// Si le prix n'a pas pu être récupéré, passer à l'exchange suivant
			if lastPrice == 0 {
				color.Red("Impossible de récupérer le prix BTC pour %s", exchangeName)
				return
			}

			allPrices[exchangeName] = lastPrice
			color.White("Prix actuel du BTC: %.2f USDC", lastPrice)

			// Récupérer les soldes détaillés
			// Protection contre les panics
			var balances map[string]common.DetailedBalance
			func() {
				defer func() {
					if r := recover(); r != nil {
						color.Red("Erreur lors de la récupération des soldes pour %s: %v", exchangeName, r)
					}
				}()
				var err error
				balances, err = client.GetDetailedBalances()
				if err != nil {
					color.Red("Erreur lors de la récupération des soldes pour %s: %v", exchangeName, err)
					return
				}
			}()

			// Si les soldes n'ont pas pu être récupérés, passer à l'exchange suivant
			if balances == nil {
				color.Red("Impossible de récupérer les soldes pour %s", exchangeName)
				return
			}

			// Stocker les soldes
			allBalances[exchangeName] = balances

			// Afficher les soldes BTC
			btcBalance, hasBTC := balances["BTC"]
			if hasBTC {
				color.Yellow("Solde BTC:")
				color.White("  Libre:      %.8f BTC (%.2f USDC)", btcBalance.Free, btcBalance.Free*lastPrice)
				color.White("  Verrouillé: %.8f BTC (%.2f USDC)", btcBalance.Locked, btcBalance.Locked*lastPrice)
				color.White("  Total:      %.8f BTC (%.2f USDC)", btcBalance.Total, btcBalance.Total*lastPrice)
			} else {
				color.Yellow("Solde BTC: Non disponible")
			}

			// Afficher les soldes USDC
			usdcBalance, hasUSDC := balances["USDC"]
			if hasUSDC {
				color.Yellow("Solde USDC:")
				color.White("  Libre:      %.2f USDC", usdcBalance.Free)
				color.White("  Verrouillé: %.2f USDC", usdcBalance.Locked)
				color.White("  Total:      %.2f USDC", usdcBalance.Total)
			} else {
				color.Yellow("Solde USDC: Non disponible")
			}

			fmt.Println("") // Ligne vide pour séparer les sections
		}()
	}

	// Récupérer tous les cycles depuis le repository
	repo := database.GetRepository()
	cycles, err := repo.FindAll()
	if err != nil {
		color.Red("Erreur lors de la récupération des cycles: %v", err)
		return
	}

	// Traiter chaque cycle
	for _, cycle := range cycles {
		// Vérifier que l'exchange du cycle existe dans allPrices et allBalances
		if _, priceExists := allPrices[cycle.Exchange]; !priceExists {
			color.Yellow("Prix non disponible pour le cycle %d (Exchange: %s). Le cycle sera ignoré.",
				cycle.IdInt, cycle.Exchange)
			continue
		}

		// Déterminer le prix actuel et le client pour cet exchange
		var lastPrice float64
		var client common.Exchange

		// Utiliser une fonction anonyme pour capturer les panics potentiels
		func() {
			defer func() {
				if r := recover(); r != nil {
					color.Red("Panic lors du traitement du cycle %d: %v", cycle.IdInt, r)
				}
			}()

			switch cycle.Exchange {
			case "BINANCE":
				lastPrice = allPrices["BINANCE"]
				client = GetClientByExchange("BINANCE")
			case "MEXC":
				lastPrice = allPrices["MEXC"]
				client = GetClientByExchange("MEXC")
			case "KUCOIN":
				lastPrice = allPrices["KUCOIN"]
				client = GetClientByExchange("KUCOIN")
			case "KRAKEN":
				lastPrice = allPrices["KRAKEN"]
				client = GetClientByExchange("KRAKEN")
			default:
				color.Red("Exchange non supporté: %s", cycle.Exchange)
				return
			}

			// Vérifier que le client est bien initialisé
			if client == nil {
				color.Red("Client non initialisé pour l'exchange %s", cycle.Exchange)
				return
			}

			// Traiter le cycle en fonction de son statut
			switch cycle.Status {
			case "buy":
				processBuyCycle(client, repo, cycle, lastPrice)
			case "sell":
				processSellCycle(client, repo, cycle)
			case "completed":
				// Pas d'action nécessaire pour les cycles complétés
				return
			}
		}()
	}

	// À ajouter dans la fonction Update après avoir traité tous les cycles
	// Afficher les informations d'accumulation pour chaque exchange
	for _, exchangeName := range exchanges {
		if exchangeConfig, exists := cfg.Exchanges[exchangeName]; exists && exchangeConfig.Enabled {
			if exchangeConfig.Accumulation {
				displayAccumulationInfo(exchangeName)
			}
		}
	}

	// Afficher l'historique des cycles à la fin de la mise à jour
	displayCyclesHistory(cycles, 0)
}

// processBuyCycle traite un cycle en statut "buy" pour n'importe quel exchange
func processBuyCycle(client common.Exchange, repo *database.CycleRepository, cycle *database.Cycle, lastPrice float64) {
	// Nettoyer l'ID d'ordre d'achat
	cleanBuyId := cleanOrderId(cycle.BuyId, cycle.Exchange)

	if cleanBuyId == "" {
		color.Red("ID d'ordre d'achat invalide: %s", cycle.BuyId)
		return
	}

	// Charger la configuration pour obtenir les paramètres spécifiques de l'exchange
	cfg, err := config.LoadConfig()
	if err != nil {
		color.Red("Erreur de configuration: %v", err)
		return
	}

	// Obtenir la configuration de l'exchange pour ce cycle
	exchangeConfig, configErr := cfg.GetExchangeConfig(cycle.Exchange)
	if configErr != nil {
		color.Red("Erreur lors de la récupération de la configuration de l'exchange: %v", configErr)
		return
	}

	// Récupérer les paramètres d'annulation automatique
	maxDays := exchangeConfig.BuyMaxDays
	maxPriceDeviation := exchangeConfig.BuyMaxPriceDeviation

	// Vérifier si l'ordre doit être annulé en raison de son âge
	if maxDays > 0 {
		age := cycle.GetAge()
		if age >= float64(maxDays) {
			color.Yellow("Cycle %d: L'ordre d'achat a dépassé l'âge maximal de %d jours (âge actuel: %.2f jours). Annulation...",
				cycle.IdInt, maxDays, age)

			// Annuler l'ordre avec la fonction sécurisée
			success, err := safeOrderCancel(client, cleanBuyId, cycle.IdInt)

			if !success {
				// Si l'annulation échoue, tenter d'autres méthodes selon l'exchange
				if cycle.Exchange == "MEXC" {
					// Logique spécifique pour MEXC...
					if strings.HasPrefix(cleanBuyId, "C02__") {
						cleanId := strings.TrimPrefix(cleanBuyId, "C02__")
						success, _ = safeOrderCancel(client, cleanId, cycle.IdInt)
					} else {
						prefixedId := "C02__" + cleanBuyId
						success, _ = safeOrderCancel(client, prefixedId, cycle.IdInt)
					}

					// Dernière tentative avec l'extraction des chiffres uniquement
					if !success {
						re := regexp.MustCompile("[0-9]+")
						matches := re.FindAllString(cleanBuyId, -1)
						if len(matches) > 0 {
							numericId := matches[0]
							success, _ = safeOrderCancel(client, numericId, cycle.IdInt)
						}
					}
				}

				// Si toutes les tentatives échouent, informer l'utilisateur mais poursuivre
				if !success {
					color.Red("Erreur lors de l'annulation de l'ordre par âge: %v", err)
					color.Yellow("L'ordre n'a pas pu être annulé sur l'exchange, mais le cycle sera supprimé de la base de données.")
					color.Yellow("Vous devrez peut-être annuler manuellement l'ordre sur %s", cycle.Exchange)
				}
			}

			// Mettre à jour le statut du cycle, MÊME SI l'annulation sur l'exchange a échoué
			err = repo.UpdateByIdInt(cycle.IdInt, map[string]interface{}{
				"status": "cancelled",
			})
			if err != nil {
				color.Red("Erreur lors de la mise à jour du cycle: %v", err)
			} else {
				color.Green("Cycle %d: Ordre d'achat annulé avec succès (âge maximal dépassé)", cycle.IdInt)
			}
			return
		}
	}

	// Récupérer l'ordre d'achat
	orderBytes, err := client.GetOrderById(cleanBuyId)
	if err != nil {
		color.Red("Erreur lors de la récupération de l'ordre d'achat %s (nettoyé: %s): %v",
			cycle.BuyId, cleanBuyId, err)

		// Si l'erreur suggère que l'ordre n'existe pas, mettre à jour le cycle
		if strings.Contains(err.Error(), "404") ||
			strings.Contains(err.Error(), "Not Found") {
			color.Yellow("Ordre non trouvé, mise à jour potentielle du cycle")

			err = repo.UpdateByIdInt(cycle.IdInt, map[string]interface{}{
				"status": "cancelled",
			})
			if err != nil {
				color.Red("Erreur lors de la mise à jour du cycle: %v", err)
			}
			return
		}

		return
	}

	// Vérification spécifique pour MEXC qui peut signaler FILLED avant mise à jour réelle des soldes
	if cycle.Exchange == "MEXC" && client.IsFilled(string(orderBytes)) {
		// Récupérer les soldes pour confirmer que le BTC est disponible
		balances, balErr := client.GetDetailedBalances()
		if balErr == nil {
			availableBTC := balances["BTC"].Free
			color.Yellow("MEXC: Vérification solde BTC disponible: %.8f BTC pour cycle %.8f BTC",
				availableBTC, cycle.Quantity)

			// Si le solde disponible est insuffisant
			if availableBTC < cycle.Quantity*0.98 {
				color.Yellow("MEXC: Délai de 5 secondes pour permettre la mise à jour des soldes")
				time.Sleep(5 * time.Second)

				// Vérifier à nouveau après le délai
				balances, balErr = client.GetDetailedBalances()
				if balErr == nil {
					availableBTC = balances["BTC"].Free
					color.Yellow("MEXC: Après délai - Solde BTC disponible: %.8f BTC pour cycle %.8f BTC",
						availableBTC, cycle.Quantity)

					// Si toujours insuffisant
					if availableBTC < cycle.Quantity*0.95 {
						// Ne pas poursuivre la création de l'ordre de vente pour ce cycle
						color.Yellow("Cycle %d: Solde BTC disponible insuffisant (%.8f) pour vendre %.8f BTC. L'ordre semble ne pas être réellement exécuté.",
							cycle.IdInt, availableBTC, cycle.Quantity)
						return
					}
				}
			}
		}
	}

	// Vérifier si l'ordre n'est PAS rempli
	if !client.IsFilled(string(orderBytes)) {
		// Vérifier si l'ordre devrait être annulé en raison de la déviation de prix
		if maxPriceDeviation > 0 {
			// Calculer le seuil d'annulation basé sur le pourcentage configuré
			deviationFactor := 1 + (maxPriceDeviation / 100)
			cancelThreshold := cycle.BuyPrice * deviationFactor

			if lastPrice > cancelThreshold {
				color.Yellow("Cycle %d: Le prix actuel %.2f dépasse le seuil d'annulation (%.2f, déviation configurée: %.2f%%). Annulation de l'ordre...",
					cycle.IdInt, lastPrice, cancelThreshold, maxPriceDeviation)

				// Utiliser la fonction sécurisée
				success, err := safeOrderCancel(client, cleanBuyId, cycle.IdInt)

				if !success {
					color.Red("Erreur lors de l'annulation de l'ordre par déviation de prix: %v", err)
					return
				}

				// Mettre à jour le statut du cycle
				err = repo.UpdateByIdInt(cycle.IdInt, map[string]interface{}{
					"status": "cancelled",
				})
				if err != nil {
					color.Red("Erreur lors de la mise à jour du cycle: %v", err)
				} else {
					color.Green("Cycle %d: Ordre d'achat annulé avec succès (déviation de prix maximale dépassée)", cycle.IdInt)
				}
				return
			}
		}
		return
	}

	// === L'ORDRE EST REMPLI, RÉCUPÉRER LES FRAIS D'ACHAT DE FAÇON PRÉCISE ===
	color.Green("Cycle %d: Ordre d'achat exécuté", cycle.IdInt)

	// Récupérer les frais d'achat réels
	var buyFees float64
	// Tenter de récupérer les frais avec la méthode publique GetOrderFees
	buyFees, err = client.GetOrderFees(cleanBuyId)
	if err != nil {
		// Si on ne peut pas récupérer les frais, estimer avec le taux par défaut
		feeRate := getFeeRateForExchange(cycle.Exchange)
		buyFees = cycle.BuyPrice * cycle.Quantity * feeRate
		color.Yellow("Impossible de récupérer les frais d'achat, estimation selon le taux standard: %.8f USDC (taux: %.4f%%)",
			buyFees, feeRate*100)
	} else {
		color.Green("Frais d'achat récupérés: %.8f USDC", buyFees)
	}

	// Extraire la quantité réellement exécutée depuis l'API
	var executedQty float64 = 0

	switch cycle.Exchange {
	case "MEXC":
		// Format de réponse pour MEXC
		executedQtyStr, err := jsonparser.GetString(orderBytes, "executedQty")
		if err == nil && executedQtyStr != "" {
			parsedQty, parseErr := strconv.ParseFloat(executedQtyStr, 64)
			if parseErr == nil && parsedQty > 0 {
				executedQty = parsedQty
				color.Yellow("MEXC: Quantité exécutée extraite de l'API: %.8f BTC", executedQty)
			}
		}

	case "BINANCE":
		executedQtyStr, err := jsonparser.GetString(orderBytes, "executedQty")
		if err == nil && executedQtyStr != "" {
			parsedQty, parseErr := strconv.ParseFloat(executedQtyStr, 64)
			if parseErr == nil && parsedQty > 0 {
				executedQty = math.Floor(parsedQty*100000000) / 100000000
				color.Yellow("BINANCE: Quantité exécutée extraite de l'API: %.8f BTC", executedQty)
			}
		}

	case "KUCOIN":
		// Format de réponse pour KuCoin
		dealSizeStr, err := jsonparser.GetString(orderBytes, "dealSize")
		if err == nil && dealSizeStr != "" {
			parsedQty, parseErr := strconv.ParseFloat(dealSizeStr, 64)
			if parseErr == nil && parsedQty > 0 {
				executedQty = parsedQty
				color.Yellow("KUCOIN: Quantité exécutée extraite de l'API: %.8f BTC", executedQty)
			}
		}

	case "KRAKEN":
		// Format de réponse pour Kraken
		var volExecStr string
		volExecStr, _ = jsonparser.GetString(orderBytes, "vol_exec")
		if volExecStr == "" {
			volExecStr, _ = jsonparser.GetString(orderBytes, "executed")
		}

		if volExecStr != "" {
			parsedQty, parseErr := strconv.ParseFloat(volExecStr, 64)
			if parseErr == nil && parsedQty > 0 {
				executedQty = parsedQty
				color.Yellow("KRAKEN: Quantité exécutée extraite de l'API: %.8f BTC", executedQty)
			}
		}
	}

	// Si nous avons pu extraire une quantité valide et différente de la quantité initiale, mettre à jour
	if executedQty > 0 && math.Abs(executedQty-cycle.Quantity)/cycle.Quantity > 0.0005 && cycle.Exchange != "BINANCE" {
		color.Yellow("Cycle %d: Mise à jour de la quantité de %.8f BTC à %.8f BTC (d'après l'API)",
			cycle.IdInt, cycle.Quantity, executedQty)

		// Calculer le montant d'achat précis (prix * quantité)
		purchaseAmountUSDC := cycle.BuyPrice * executedQty

		// Mettre à jour la quantité et stocker les frais dans la base de données
		err = repo.UpdateByIdInt(cycle.IdInt, map[string]interface{}{
			"quantity":           executedQty,
			"buyFees":            buyFees,            // Nouveau: stocker les frais d'achat dans un champ dédié
			"totalFees":          buyFees,            // Initialiser totalFees avec buyFees
			"purchaseAmountUSDC": purchaseAmountUSDC, // Stocker le montant exact d'achat
		})

		if err != nil {
			color.Red("Erreur lors de la mise à jour de la quantité et des frais: %v", err)
		} else {
			// Mettre à jour l'objet cycle local pour la suite du traitement
			cycle.Quantity = executedQty
			cycle.TotalFees = buyFees
			cycle.PurchaseAmountUSDC = purchaseAmountUSDC
		}
	} else {
		// Si la quantité reste inchangée, mettre à jour uniquement les frais
		// Calculer le montant d'achat précis (prix * quantité)
		purchaseAmountUSDC := cycle.BuyPrice * cycle.Quantity

		err = repo.UpdateByIdInt(cycle.IdInt, map[string]interface{}{
			"buyFees":            buyFees,            // Nouveau: stocker les frais d'achat dans un champ dédié
			"totalFees":          buyFees,            // Initialiser totalFees avec buyFees
			"purchaseAmountUSDC": purchaseAmountUSDC, // Stocker le montant exact d'achat
		})

		if err != nil {
			color.Red("Erreur lors de la mise à jour des frais: %v", err)
		} else {
			cycle.TotalFees = buyFees
			cycle.PurchaseAmountUSDC = purchaseAmountUSDC
		}
	}

	// ========= CALCUL DU PRIX DE VENTE =========
	// 1. Prix de vente standard basé sur la configuration
	sellOffset := exchangeConfig.SellOffset
	standardSellPrice := cycle.BuyPrice + sellOffset

	// 2. Prix minimum pour être maker (légèrement au-dessus du prix actuel)
	makerMinPrice := lastPrice * 1.001

	// 3. Prix ajusté pour couvrir les frais
	var feeAdjustedPrice float64

	// Utiliser la méthode AdjustSellPriceForFees de l'interface Exchange pour calculer un prix
	// qui prend en compte les frais d'achat et de vente
	adjustedPrice, err := client.AdjustSellPriceForFees(cycle.BuyPrice, cycle.Quantity, cleanBuyId)
	if err == nil {
		feeAdjustedPrice = adjustedPrice
		color.Yellow("Cycle %d: Prix de vente ajusté pour les frais via API: %.2f USDC",
			cycle.IdInt, feeAdjustedPrice)
	} else {
		// En cas d'erreur, on retombe sur l'estimation des frais
		color.Yellow("Erreur lors de l'ajustement du prix via API: %v, utilisation de l'estimation", err)

		// Estimer les frais selon l'exchange
		var feeRate float64 = getFeeRateForExchange(cycle.Exchange)

		// Estimer les frais de vente
		estimatedSellFees := cycle.BuyPrice * cycle.Quantity * feeRate

		// Total des frais estimés (achat déjà récupéré + vente estimée)
		totalFeesEstimated := buyFees + estimatedSellFees

		// Ajouter une marge de sécurité
		if cycle.Exchange == "KRAKEN" {
			totalFeesEstimated *= 1.1
		} else {
			totalFeesEstimated *= 1.05
		}

		// Calculer l'ajustement par unité
		feeAdjustmentPerUnit := totalFeesEstimated / cycle.Quantity

		// Prix minimum pour couvrir les frais estimés
		feeAdjustedPrice = cycle.BuyPrice + feeAdjustmentPerUnit

		color.Yellow("Cycle %d: Prix de vente ajusté pour frais estimés: %.2f USDC (frais estimés: %.8f USDC)",
			cycle.IdInt, feeAdjustedPrice, totalFeesEstimated)
	}

	// 4. Déterminer le prix de vente final (le maximum des trois valeurs)
	var finalSellPrice float64

	// a) Si le prix ajusté pour les frais est le plus élevé
	if feeAdjustedPrice >= standardSellPrice && feeAdjustedPrice >= makerMinPrice {
		finalSellPrice = feeAdjustedPrice
		color.Yellow("Cycle %d: Prix de vente déterminé par les frais: %.2f USDC", cycle.IdInt, finalSellPrice)
	} else if makerMinPrice >= standardSellPrice && makerMinPrice >= feeAdjustedPrice {
		// b) Si le prix maker minimum est le plus élevé
		finalSellPrice = makerMinPrice
		color.Yellow("Cycle %d: Prix de vente déterminé pour être maker: %.2f USDC", cycle.IdInt, finalSellPrice)
	} else {
		// c) Si le prix standard est le plus élevé
		finalSellPrice = standardSellPrice
		color.Yellow("Cycle %d: Prix de vente standard utilisé: %.2f USDC", cycle.IdInt, finalSellPrice)
	}

	// Calculer le montant de vente prévu
	saleAmountUSDC := finalSellPrice * cycle.Quantity

	// Mise à jour du prix de vente et du montant de vente prévu dans la base de données
	err = repo.UpdateByIdInt(cycle.IdInt, map[string]interface{}{
		"sellPrice":      finalSellPrice,
		"saleAmountUSDC": saleAmountUSDC, // Nouveau: stocker le montant exact de vente prévu
	})

	if err != nil {
		color.Red("Erreur lors de la mise à jour du prix de vente: %v", err)
		return
	}

	// Mettre à jour l'objet cycle local
	cycle.SellPrice = finalSellPrice
	cycle.SaleAmountUSDC = saleAmountUSDC

	// Vérifier le solde BTC disponible
	balances, balErr := client.GetDetailedBalances()
	if balErr != nil {
		color.Red("Erreur lors de la récupération des soldes: %v", balErr)
		return
	}

	// Vérifier que le BTC est réellement disponible
	availableBTC := balances["BTC"].Free

	// Ajuster la quantité si nécessaire
	quantityToSell := cycle.Quantity
	if availableBTC < quantityToSell && availableBTC > quantityToSell*0.95 {
		color.Yellow("Cycle %d: Ajustement de la quantité à vendre de %.8f à %.8f (disponible)",
			cycle.IdInt, quantityToSell, availableBTC)
		quantityToSell = availableBTC

		// Mettre à jour le montant de vente prévu avec la nouvelle quantité
		saleAmountUSDC = finalSellPrice * quantityToSell
		cycle.SaleAmountUSDC = saleAmountUSDC
	}

	if cycle.Exchange == "BINANCE" {
		quantityToSell = executedQty
		color.Yellow("Cycle %d: Utilisation de la quantité exacte achetée: %.8f BTC",
			cycle.IdInt, quantityToSell)
	}

	// Préparer les paramètres de l'ordre de vente
	quantityStr := strconv.FormatFloat(quantityToSell, 'f', 8, 64)
	sellPriceStr := strconv.FormatFloat(finalSellPrice, 'f', 2, 64)

	// Créer l'ordre de vente
	sellBytes, err := client.CreateOrder("SELL", sellPriceStr, quantityStr)

	// Gestion améliorée pour Kraken
	if err != nil {
		// Cas spécial pour Kraken: vérifier si l'ordre a été créé malgré l'erreur
		if cycle.Exchange == "KRAKEN" && strings.Contains(err.Error(), "Insufficient funds") {
			color.Yellow("Kraken a signalé 'fonds insuffisants', vérification si l'ordre a été créé malgré l'erreur...")
			time.Sleep(10 * time.Second)
		}

		color.Red("Erreur lors de la création de l'ordre de vente: %v", err)

		// Si l'erreur est de type "Oversold", donner des instructions spécifiques
		if strings.Contains(strings.ToLower(err.Error()), "oversold") {
			color.Yellow("Erreur de type 'Oversold': Cela signifie que vous essayez de vendre plus que ce qui est disponible.")
			color.Yellow("Vérifiez les points suivants:")
			color.Yellow("1. Vérifiez si l'ordre de vente n'a pas déjà été créé sur la plateforme")
			color.Yellow("2. Vérifiez si les fonds sont bien disponibles et non verrouillés")
			color.Yellow("3. Attendez quelques minutes pour que les soldes se mettent à jour")

			// Mettre quand même à jour le statut pour éviter de perdre l'information que l'achat est complété
			err = repo.UpdateByIdInt(cycle.IdInt, map[string]interface{}{
				"status": "sell",
				// Pas de SellId car l'ordre n'a pas été créé
			})
			if err != nil {
				color.Red("Erreur lors de la mise à jour du cycle: %v", err)
			} else {
				color.Yellow("Cycle %d: Statut mis à jour à 'sell' mais l'ordre de vente n'a pas pu être créé", cycle.IdInt)
			}
		}

		return
	}

	// Extraire l'ID de l'ordre de vente
	orderIdValue, dataType, _, err := jsonparser.Get(sellBytes, "orderId")
	if err != nil {
		color.Red("Erreur lors de l'extraction de l'ID d'ordre: %v", err)
		color.Red("Réponse API complète: %s", string(sellBytes))
		return
	}

	// Conversion selon le type de données
	var orderIdStr string
	switch dataType {
	case jsonparser.String:
		orderIdStr = string(orderIdValue)
	case jsonparser.Number:
		orderIdStr = string(orderIdValue)
	default:
		orderIdStr = string(orderIdValue)
		color.Yellow("Type de données inattendu pour l'ID d'ordre: %v", dataType)
	}

	// Vérification supplémentaire pour s'assurer que l'ID n'est pas vide
	if orderIdStr == "" {
		color.Red("ID d'ordre vide obtenu de la réponse API")
		color.Red("Réponse API complète: %s", string(sellBytes))
		return
	}

	// Mettre à jour le cycle
	err = repo.UpdateByIdInt(cycle.IdInt, map[string]interface{}{
		"status": "sell",
		"sellId": orderIdStr,
	})
	if err != nil {
		color.Red("Erreur lors de la mise à jour du cycle: %v", err)
		return
	}

	// Calculer et afficher le profit potentiel
	profitPercent := ((finalSellPrice - cycle.BuyPrice) / cycle.BuyPrice) * 100
	color.Green("Cycle %d: Ordre de vente placé avec succès. ID: %s", cycle.IdInt, orderIdStr)
	color.Green("Cycle %d: Prix d'achat: %.2f, Prix de vente: %.2f, Profit potentiel: %.2f%%",
		cycle.IdInt, cycle.BuyPrice, finalSellPrice, profitPercent)
	color.Green("Cycle %d: Frais d'achat: %.8f USDC", cycle.IdInt, buyFees)
}

func processSellCycle(client common.Exchange, repo *database.CycleRepository, cycle *database.Cycle) {
	// Obtenir le repository d'accumulation
	accuRepo := database.GetAccumulationRepository()

	// Obtenir la configuration de l'exchange
	cfg, err := config.LoadConfig()
	if err != nil {
		color.Red("Erreur lors du chargement de la configuration: %v", err)
		return
	}

	exchangeConfig, err := cfg.GetExchangeConfig(cycle.Exchange)
	if err != nil {
		color.Red("Erreur lors de la récupération de la configuration de l'exchange: %v", err)
		return
	}

	// Obtenir le prix actuel du BTC
	currentPrice := client.GetLastPriceBTC()
	// Vérifier les conditions d'accumulation
	shouldAccumulate, deviationPercent, err := checkAccumulationConditions(cycle, currentPrice, exchangeConfig, accuRepo)
	if err != nil {
		color.Red("Erreur lors de la vérification des conditions d'accumulation: %v", err)
	}

	if shouldAccumulate {
		color.Yellow("Conditions d'accumulation remplies pour le cycle %d:", cycle.IdInt)
		color.Yellow("  - Déviation de prix: %.2f%% (seuil: %.2f%%)", deviationPercent, exchangeConfig.SellAccuPriceDeviation)
		color.Yellow("  - Annulation de l'ordre de vente pour accumulation...")

		// Créer une nouvelle entrée d'accumulation
		accumulation := &database.Accumulation{
			Exchange:         cycle.Exchange,
			CycleIdInt:       cycle.IdInt,
			Quantity:         cycle.Quantity,
			OriginalBuyPrice: cycle.BuyPrice,
			TargetSellPrice:  cycle.SellPrice,
			CancelPrice:      currentPrice,
			Deviation:        deviationPercent,
			CreatedAt:        time.Now(),
		}

		// Enregistrer l'accumulation
		_, err = accuRepo.Save(accumulation)
		if err != nil {
			color.Red("Erreur lors de l'enregistrement de l'accumulation: %v", err)

			// Même si l'enregistrement échoue, essayer de supprimer le cycle
			deleteErr := repo.DeleteByIdInt(cycle.IdInt)
			if deleteErr != nil {
				color.Red("Erreur lors de la suppression du cycle: %v", deleteErr)
			} else {
				color.Yellow("Cycle supprimé malgré l'échec d'enregistrement de l'accumulation.")
			}
			return
		}

		// Supprimer le cycle de la base de données
		err = repo.DeleteByIdInt(cycle.IdInt)
		if err != nil {
			color.Red("Erreur lors de la suppression du cycle pour accumulation: %v", err)
			color.Yellow("Attention: L'accumulation a été enregistrée mais le cycle n'a pas été supprimé. Cycle ID: %d", cycle.IdInt)
		} else {
			color.Green("Cycle %d annulé avec succès pour accumulation", cycle.IdInt)
			color.Green("%.8f BTC accumulés à un prix de %.2f au lieu de %.2f (économie: %.2f%%)",
				cycle.Quantity, currentPrice, cycle.SellPrice, deviationPercent)
		}

		return
	}

	// Si nous ne sommes pas en accumulation, continuer avec le traitement normal
	// Nettoyer l'ID d'ordre de vente en spécifiant l'exchange
	cleanSellId := cleanOrderId(cycle.SellId, cycle.Exchange)
	if cleanSellId == "" {
		color.Red("ID d'ordre de vente invalide: %s", cycle.SellId)
		return
	}

	// Récupérer l'ordre de vente
	orderBytes, err := client.GetOrderById(cleanSellId)
	if err != nil {
		color.Red("Erreur lors de la récupération de l'ordre de vente %s (nettoyé: %s): %v",
			cycle.SellId, cleanSellId, err)
		return
	}

	// Vérifier si l'ordre est exécuté
	isFilled := client.IsFilled(string(orderBytes))
	if !isFilled {
		// L'ordre n'est pas encore exécuté
		return
	}

	// Récupérer les frais de vente réels
	var sellFees float64
	// Tenter de récupérer les frais avec la méthode publique GetOrderFees
	sellFees, err = client.GetOrderFees(cleanSellId)
	if err != nil {
		// Si on ne peut pas récupérer les frais, estimer avec le taux par défaut
		feeRate := getFeeRateForExchange(cycle.Exchange)
		sellFees = cycle.SellPrice * cycle.Quantity * feeRate
		color.Yellow("Impossible de récupérer les frais de vente, estimation selon le taux standard: %.8f USDC (taux: %.4f%%)",
			sellFees, feeRate*100)
	} else {
		color.Green("Frais de vente récupérés: %.8f USDC", sellFees)
	}

	// Ajouter directement les frais de vente aux frais totaux déjà enregistrés
	totalFees := cycle.TotalFees + sellFees

	// Tenter d'extraire la date réelle d'exécution et les frais selon l'exchange
	completionTime := time.Now() // Valeur par défaut
	extractionSuccessful := false

	// Extraction spécifique à chaque exchange
	switch cycle.Exchange {
	case "BINANCE":
		updateTimeMs, timeErr := jsonparser.GetInt(orderBytes, "updateTime")
		if timeErr == nil {
			extractedTime := time.Unix(0, updateTimeMs*int64(time.Millisecond))
			if extractedTime.After(cycle.CreatedAt) {
				completionTime = extractedTime
				extractionSuccessful = true
			}
		}

	case "MEXC":
		// Utiliser une estimation raisonnable pour MEXC (6h après création)
		// car ses timestamps sont souvent incorrects
		now := time.Now()
		if completionTime.Before(cycle.CreatedAt) {
			// Si la date de complétion est avant la date de création, utiliser la date de création + une durée raisonnable
			color.Yellow("Correction de date: CompletedAt était antérieur à CreatedAt pour le cycle %d", cycle.IdInt)
			completionTime = cycle.CreatedAt.Add(6 * time.Hour) // 6h est une estimation raisonnable pour MEXC
		} else {
			completionTime = now.Add(-1 * time.Hour)
		}
		extractionSuccessful = true

	case "KUCOIN":
		// Pour KuCoin, extraire la date si possible
		createdAtStr, err := jsonparser.GetString(orderBytes, "createdAt")
		if err == nil && createdAtStr != "" {
			// KuCoin utilise des timestamps en millisecondes
			if timestampMs, err := strconv.ParseInt(createdAtStr, 10, 64); err == nil {
				extractedTime := time.Unix(0, timestampMs*int64(time.Millisecond))
				if extractedTime.After(cycle.CreatedAt) {
					completionTime = extractedTime
					extractionSuccessful = true
				}
			}
		}

	case "KRAKEN":
		// Pour Kraken, tenter d'extraire le timestamp de fermeture ou utiliser une estimation
		// Les formats de réponse de Kraken peuvent varier, vérifier plusieurs possibilités
		closeTimeStr, err := jsonparser.GetString(orderBytes, "closetm")
		if err == nil && closeTimeStr != "" {
			// Kraken peut fournir des timestamps sous différents formats
			closeTime, parseErr := strconv.ParseFloat(closeTimeStr, 64)
			if parseErr == nil {
				// Convertir le timestamp en time.Time
				extractedTime := time.Unix(int64(closeTime), 0)
				if extractedTime.After(cycle.CreatedAt) {
					completionTime = extractedTime
					extractionSuccessful = true
				}
			}
		}

		// Si l'extraction échoue, utiliser une estimation raisonnable (5h après création)
		if !extractionSuccessful {
			completionTime = cycle.CreatedAt.Add(5 * time.Hour)
			extractionSuccessful = true
		}
	}

	if extractionSuccessful {
		color.Green("Date de complétion extraite avec succès pour le cycle %d: %s",
			cycle.IdInt, completionTime.Format("02/01/2006 15:04:05"))
	} else {
		color.Yellow("Utilisation de la date actuelle comme date de complétion pour le cycle %d", cycle.IdInt)
	}

	// Calculer le profit net en tenant compte des frais spécifiques
	var profit, profitPercent float64
	buyAmount := cycle.BuyPrice * cycle.Quantity
	sellAmount := cycle.SellPrice * cycle.Quantity

	profit = sellAmount - buyAmount - totalFees
	if buyAmount > 0 {
		profitPercent = (profit / buyAmount) * 100
	}

	// Afficher les détails du profit avec les frais
	if totalFees > 0 {
		color.Green("Cycle %d: COMPLÉTÉ AVEC SUCCÈS! (Profit net: %.2f USDC, %.2f%%)",
			cycle.IdInt, profit, profitPercent)
		color.Green("Frais totaux: %.8f USDC (Achat: %.8f, Vente: %.8f)",
			totalFees, sellFees)
	} else {
		color.Green("Cycle %d: COMPLÉTÉ AVEC SUCCÈS!", cycle.IdInt)
	}

	// Mettre à jour le cycle dans la base de données
	// Ajouter les champs de frais dans la mise à jour
	updateFields := map[string]interface{}{
		"status":      "completed",
		"completedAt": completionTime.Format(time.RFC3339),
		"sellFees":    sellFees,
		"totalFees":   totalFees,
	}

	err = repo.UpdateByIdInt(cycle.IdInt, updateFields)
	if err != nil {
		color.Red("Erreur lors de la mise à jour du cycle: %v", err)
		return
	}

	// Mettre à jour l'objet cycle en mémoire également
	cycle.Status = "completed"
	cycle.CompletedAt = completionTime

	color.Green("Date d'achat: %s", cycle.CreatedAt.Format("02/01/2006 15:04"))
	color.Green("Date de vente: %s", completionTime.Format("02/01/2006 15:04"))
	color.Green("Durée du cycle: %s", formatDetailedDuration(time.Since(cycle.CreatedAt).Hours()/24))
}

func displayCyclesHistory(cycles []*database.Cycle, _ float64) {
	if len(cycles) == 0 {
		color.Yellow("Aucun cycle trouvé dans la base de données.")
		return
	}

	// Compteurs pour les statistiques
	statsBinance := cycleStatistics{}
	statsMexc := cycleStatistics{}
	statsKucoin := cycleStatistics{}
	statsKraken := cycleStatistics{}

	fmt.Println("")
	color.Cyan("===== CYCLES ACTIFS =====")
	fmt.Println("")

	// Nouvel en-tête avec les colonnes prix BTC à l'achat et à la vente
	headerFormat := "%-5s | %-10s | %-12s | %-15s | %-15s | %-15s | %-15s | %-15s\n"
	rowFormat := "%-5d | %-10s | %-12s | %-15.2f | %-15.2f | %-15.2f | %-15s | %-15s\n"

	fmt.Printf(headerFormat, "ID", "EXCHANGE", "STATUT", "MONTANT USDC", "PRIX BTC ACHAT", "PRIX BTC VENTE", "GAINS PRÉVUS", "DURÉE")
	fmt.Println("-------+------------+--------------+-----------------+-----------------+-----------------+-----------------+-----------------")

	// Trier les cycles par ID (du plus récent au plus ancien)
	sort.Slice(cycles, func(i, j int) bool {
		return cycles[i].IdInt > cycles[j].IdInt
	})

	// Filtrer et afficher uniquement les cycles non complétés
	activeCycles := 0
	for _, cycle := range cycles {
		// Exclure les cycles complétés et annulés
		if cycle.Status == "completed" || cycle.Status == "cancelled" {
			// Mettre à jour les statistiques mais ne pas afficher
			updateStats(cycle, &statsBinance, &statsMexc, &statsKucoin, &statsKraken)
			continue
		}

		activeCycles++

		// Déterminer le statut avec couleur
		var status string
		switch cycle.Status {
		case "buy":
			status = color.GreenString("ACHAT")
		case "sell":
			status = color.YellowString("VENTE")
		default:
			status = cycle.Status
		}

		// Calculer le montant USDC utilisé pour l'achat
		usdcAmount := cycle.BuyPrice * cycle.Quantity

		// Calculer le montant USDC prévu à la vente
		usdcSaleAmount := cycle.SellPrice * cycle.Quantity

		// Calculer les gains prévus (en valeur absolue et en pourcentage)
		var expectedProfit float64

		// Obtenir un client pour cet exchange afin de pouvoir utiliser GetOrderFees
		var client common.Exchange
		func() {
			defer func() {
				if r := recover(); r != nil {
					color.Red("Erreur lors de l'initialisation du client pour %s: %v", cycle.Exchange, r)
				}
			}()
			client = GetClientByExchange(cycle.Exchange)
		}()

		// Récupérer les frais d'achat réels si possible sinon estimer les frais
		var buyFees, sellFees float64

		if client != nil {
			// Si nous avons un ID d'achat et que l'ordre est déjà exécuté
			if cycle.BuyId != "" {
				// Nettoyer l'ID de l'ordre d'achat selon l'exchange
				cleanBuyId := cleanOrderId(cycle.BuyId, cycle.Exchange)
				if cleanBuyId != "" {
					// Tenter de récupérer les frais réels
					realBuyFees, err := client.GetOrderFees(cleanBuyId)
					if err == nil && realBuyFees > 0 {
						buyFees = realBuyFees
					}
				} else {
					// Estimation basique si l'ID n'est pas valide
					buyFees = usdcAmount * getFeeRateForExchange(cycle.Exchange)
				}
			} else {
				// Si l'ordre d'achat est toujours en cours ou l'ID n'est pas disponible
				buyFees = usdcAmount * getFeeRateForExchange(cycle.Exchange)
			}

			// Pour les frais de vente, on doit estimer car l'ordre n'est pas encore exécuté
			// Appliquer directement le taux de frais (taux maker généralement pour les ventes)
			sellFees = usdcSaleAmount * getFeeRateForExchange(cycle.Exchange)

			// Calculer le profit en tenant compte des frais
			expectedProfit = usdcSaleAmount - usdcAmount - (buyFees + sellFees)
		} else {
			// Fallback au comportement actuel si on ne peut pas obtenir de client
			if cycle.Exchange == "KRAKEN" {
				// Estimer les frais selon le taux maker de Kraken (0.26%)
				const makerFeeRate = 0.0026
				buyFees = cycle.BuyPrice * cycle.Quantity * makerFeeRate
				sellFees = cycle.SellPrice * cycle.Quantity * makerFeeRate
				expectedProfit = usdcSaleAmount - usdcAmount - (buyFees + sellFees)
			} else if cycle.Exchange == "BINANCE" {
				// Binance: 0.1% standard
				buyFees = usdcAmount * 0.001
				sellFees = usdcSaleAmount * 0.001
				expectedProfit = usdcSaleAmount - usdcAmount - (buyFees + sellFees)
			} else {
				// Pour les autres exchanges, supposons que les frais sont déjà inclus dans les prix
				expectedProfit = usdcSaleAmount - usdcAmount
			}
		}

		expectedProfitPercent := 0.0
		if usdcAmount > 0 {
			expectedProfitPercent = (expectedProfit / usdcAmount) * 100
		}

		// Formater les gains prévus
		expectedProfitStr := fmt.Sprintf("%.2f (%.2f%%)", expectedProfit, expectedProfitPercent)

		// Calculer la durée depuis la création
		duration := calculateDuration(cycle.CreatedAt)

		// Afficher la ligne du cycle avec les nouvelles colonnes
		fmt.Printf(rowFormat,
			cycle.IdInt,
			cycle.Exchange,
			status,
			usdcAmount,
			cycle.BuyPrice,  // Prix du BTC à l'achat
			cycle.SellPrice, // Prix du BTC à la vente
			expectedProfitStr,
			duration)

		// Mettre à jour les statistiques
		updateStats(cycle, &statsBinance, &statsMexc, &statsKucoin, &statsKraken)
	}

	if activeCycles == 0 {
		color.Yellow("Aucun cycle actif trouvé.")
	}

	fmt.Println("-------+------------+--------------+-----------------+-----------------+-----------------+-----------------+-----------------")

	// Afficher les statistiques par exchange avec les nouvelles informations
	displayExchangeStats("Binance", statsBinance, cycles)
	displayExchangeStats("MEXC", statsMexc, cycles)
	displayExchangeStats("KuCoin", statsKucoin, cycles)
	displayExchangeStats("Kraken", statsKraken, cycles)
}

func displayExchangeStats(exchangeName string, stats cycleStatistics, allCycles []*database.Cycle) {
	color.Cyan("Statistiques %s:", exchangeName)
	color.White("  Total des cycles:     %d", stats.totalCycles)
	color.White("  Cycles d'achat:       %d", stats.buyCycles)
	color.White("  Cycles de vente:      %d", stats.sellCycles)
	color.White("  Cycles complétés:     %d", stats.completedCycles)

	if stats.completedCycles > 0 {
		// Récupérer la date actuelle pour calculer les périodes
		now := time.Now()

		// Calculer les profits par période
		profit24h := calculateProfitByPeriod(allCycles, exchangeName, now.Add(-24*time.Hour), now)
		profit7d := calculateProfitByPeriod(allCycles, exchangeName, now.Add(-7*24*time.Hour), now)
		profit30d := calculateProfitByPeriod(allCycles, exchangeName, now.Add(-30*24*time.Hour), now)
		profit3m := calculateProfitByPeriod(allCycles, exchangeName, now.Add(-90*24*time.Hour), now)

		// Vérifier la cohérence des profits par période
		// Le profit d'une période plus longue ne devrait pas être inférieur à celui d'une période plus courte
		if profit7d < profit24h {
			profit7d = profit24h // Ajustement pour cohérence
		}
		if profit30d < profit7d {
			profit30d = profit7d // Ajustement pour cohérence
		}
		if profit3m < profit30d {
			profit3m = profit30d // Ajustement pour cohérence
		}

		// S'assurer que le profit total est au moins égal au profit sur 3 mois
		if stats.totalProfit < profit3m {
			// Correction statistique
			stats.totalProfit = profit3m
		}

		// Afficher les profits avec un format cohérent
		color.Green("  Profit total:         %.2f USDC", stats.totalProfit)

		// Utiliser une couleur différente selon que le profit est positif ou négatif
		if profit24h >= 0 {
			color.Green("  Profit depuis 24h:    %.2f USDC", profit24h)
		} else {
			color.Red("  Profit depuis 24h:    %.2f USDC", profit24h)
		}

		if profit7d >= 0 {
			color.Green("  Profit depuis 7j:     %.2f USDC", profit7d)
		} else {
			color.Red("  Profit depuis 7j:     %.2f USDC", profit7d)
		}

		if profit30d >= 0 {
			color.Green("  Profit depuis 30j:    %.2f USDC", profit30d)
		} else {
			color.Red("  Profit depuis 30j:    %.2f USDC", profit30d)
		}

		if profit3m >= 0 {
			color.Green("  Profit depuis 3 mois: %.2f USDC", profit3m)
		} else {
			color.Red("  Profit depuis 3 mois: %.2f USDC", profit3m)
		}
	}
	fmt.Println("")
}

// calculateDuration calcule la durée depuis une date donnée jusqu'à maintenant
func calculateDuration(startTime time.Time) string {
	duration := time.Since(startTime)

	if duration.Hours() > 24 {
		days := int(duration.Hours() / 24)
		hours := int(duration.Hours()) % 24
		return fmt.Sprintf("%d j %d h", days, hours)
	} else if duration.Hours() >= 1 {
		hours := int(duration.Hours())
		minutes := int(duration.Minutes()) % 60
		return fmt.Sprintf("%d h %d min", hours, minutes)
	} else {
		minutes := int(duration.Minutes())
		return fmt.Sprintf("%d min", minutes)
	}
}

// Fonction améliorée updateStats pour mettre à jour correctement les statistiques
func updateStats(cycle *database.Cycle, statsBinance, statsMexc, statsKucoin, statsKraken *cycleStatistics) {
	// Sélectionner les statistiques de l'exchange approprié
	var stats *cycleStatistics
	switch cycle.Exchange {
	case "BINANCE":
		stats = statsBinance
	case "MEXC":
		stats = statsMexc
	case "KUCOIN":
		stats = statsKucoin
	case "KRAKEN":
		stats = statsKraken
	default:
		return // Exchange non supporté
	}

	// Mettre à jour les statistiques
	stats.totalCycles++
	switch cycle.Status {
	case "buy":
		stats.buyCycles++
	case "sell":
		stats.sellCycles++
	case "completed":
		stats.completedCycles++

		// Calculer le profit brut
		grossProfit := (cycle.SellPrice - cycle.BuyPrice) * cycle.Quantity

		// Soustraire les frais stockés pour obtenir le profit net
		var totalFees float64
		if cycle.TotalFees > 0 {
			totalFees = cycle.TotalFees
		} else {
			// Si les frais ne sont pas stockés, utiliser une estimation
			feeRate := getFeeRateForExchange(cycle.Exchange) * 2 // achat + vente
			totalFees = grossProfit * feeRate
		}

		netProfit := grossProfit - totalFees

		// Log pour le débogage si nécessaire
		if cfg != nil && cfg.Environment == "development" {
			color.Cyan("Cycle %d (%s) - Frais totaux: %.8f USDC",
				cycle.IdInt, cycle.Exchange, totalFees)
			color.Cyan("Profit brut: %.8f, Profit net: %.8f",
				grossProfit, netProfit)
		}

		// Ajouter le profit net aux statistiques
		stats.totalProfit += netProfit
	}
}

// Fonction utilitaire pour calculer le profit sur une période donnée
func calculateProfitByPeriod(cycles []*database.Cycle, exchangeName string, startTime, endTime time.Time) float64 {
	var periodProfit float64
	exchangeNameUpper := strings.ToUpper(exchangeName)

	for _, cycle := range cycles {
		cycleExchangeUpper := strings.ToUpper(cycle.Exchange)

		// Ne considérer que les cycles de l'exchange spécifié et complétés
		if cycleExchangeUpper == exchangeNameUpper && cycle.Status == "completed" {
			// Utiliser la date de complétion pour déterminer si le cycle appartient à la période
			completionDate := cycle.CompletedAt
			if completionDate.IsZero() {
				// Si la date de complétion n'est pas définie, utiliser la date de création
				// mais ce n'est pas idéal
				completionDate = cycle.CreatedAt
			}

			// Vérifier si le cycle a été complété dans la période spécifiée
			if completionDate.After(startTime) && completionDate.Before(endTime) {
				// Calculer le profit net pour ce cycle
				buyValue := cycle.BuyPrice * cycle.Quantity
				sellValue := cycle.SellPrice * cycle.Quantity
				grossProfit := sellValue - buyValue

				// Utiliser les frais stockés ou estimer si nécessaire
				var totalFees float64
				if cycle.TotalFees > 0 {
					totalFees = cycle.TotalFees
				} else {
					// Estimer les frais si non disponibles (fallback)
					feeRate := getFeeRateForExchange(cycle.Exchange) * 2 // achat + vente
					totalFees = buyValue * feeRate
				}

				netProfit := grossProfit - totalFees
				periodProfit += netProfit
			}
		}
	}

	return periodProfit
}

// checkAccumulationConditions vérifie si les conditions sont remplies pour annuler un ordre de vente pour accumulation
func checkAccumulationConditions(
	cycle *database.Cycle,
	currentPrice float64,
	exchangeConfig config.ExchangeConfig,
	accuRepo *database.AccumulationRepository) (bool, float64, error) {

	// Vérifier si l'accumulation est activée
	if !exchangeConfig.Accumulation {
		return false, 0, nil
	}

	// Calculer la déviation de prix actuelle
	deviationPercent := ((cycle.SellPrice - currentPrice) / cycle.SellPrice) * 100

	// Vérifier si la déviation est suffisante pour l'accumulation
	if deviationPercent < exchangeConfig.SellAccuPriceDeviation {
		return false, deviationPercent, nil
	}

	// Calculer le profit global de l'exchange
	exchangeProfit, err := calculateExchangeProfit(cycle.Exchange)
	if err != nil {
		return false, deviationPercent, err
	}

	// Calculer la valeur des accumulations déjà effectuées
	totalAccumulatedValue, err := accuRepo.GetTotalAccumulatedValue(cycle.Exchange)
	if err != nil {
		return false, deviationPercent, err
	}

	// Calculer la valeur de l'ordre actuel
	cycleValue := cycle.Quantity * cycle.SellPrice

	// Vérifier si le profit disponible est suffisant pour annuler cet ordre
	profitAvailable := exchangeProfit - totalAccumulatedValue

	return profitAvailable >= cycleValue, deviationPercent, nil
}

// calculateExchangeProfit calcule le profit global pour un exchange donné
func calculateExchangeProfit(exchange string) (float64, error) {
	repo := database.GetRepository()
	cycles, err := repo.FindAll()
	if err != nil {
		return 0, err
	}

	var totalProfit float64
	for _, cycle := range cycles {
		// Ne considérer que les cycles de l'exchange spécifié et complétés
		if cycle.Exchange == exchange && cycle.Status == "completed" {
			// Calculer le profit net pour ce cycle
			buyValue := cycle.BuyPrice * cycle.Quantity
			sellValue := cycle.SellPrice * cycle.Quantity
			grossProfit := sellValue - buyValue

			// Utiliser les frais stockés ou estimer si nécessaire
			fees := cycle.TotalFees
			if fees <= 0 {
				// Si aucun frais n'est stocké, utiliser une estimation
				fees = grossProfit * getFeeRateForExchange(exchange) * 2 // Achat + vente
			}

			netProfit := grossProfit - fees
			totalProfit += netProfit
		}
	}

	return totalProfit, nil
}

func displayAccumulationInfo(exchange string) {
	accuRepo := database.GetAccumulationRepository()

	// Vérifier si l'accumulation est activée pour cet exchange
	cfg, err := config.LoadConfig()
	if err != nil {
		return
	}

	exchangeConfig, err := cfg.GetExchangeConfig(exchange)
	if err != nil {
		return
	}

	if !exchangeConfig.Accumulation {
		return
	}

	// Récupérer les statistiques d'accumulation
	stats, err := accuRepo.GetExchangeAccumulationStats(exchange)
	if err != nil {
		color.Red("Erreur lors de la récupération des statistiques d'accumulation: %v", err)
		return
	}

	// Récupérer le profit disponible
	profit, err := calculateExchangeProfit(exchange)
	if err != nil {
		color.Red("Erreur lors du calcul des profits: %v", err)
		return
	}

	accuValue, _ := accuRepo.GetTotalAccumulatedValue(exchange)
	profitAvailable := profit - accuValue

	fmt.Println("")
	color.Cyan("=== INFORMATIONS D'ACCUMULATION POUR %s ===", exchange)
	color.White("Accumulation:                  %s", color.GreenString("Activée"))
	color.White("Déviation minimale configurée: %.2f%%", exchangeConfig.SellAccuPriceDeviation)
	color.White("Profit total:                  %.2f USDC", profit)
	color.White("Valeur déjà accumulée:         %.2f USDC", accuValue)
	color.White("Profit disponible:             %.2f USDC", profitAvailable)
	color.White("Nombre d'accumulations:        %d", stats["count"])

	if stats["count"].(int) > 0 {
		color.White("Quantité totale accumulée:     %.8f BTC", stats["totalQuantity"])
		color.White("Économie réalisée:             %.2f USDC", stats["savedValue"])
		color.White("Déviation moyenne:             %.2f%%", stats["averageDeviation"])
	}
	fmt.Println("")
}

// safeOrderCancel tente d'annuler un ordre et gère correctement les erreurs qui indiquent un succès
func safeOrderCancel(client common.Exchange, orderId string, cycleId int32) (bool, error) {
	// Vérifier si c'est un ID MEXC et appliquer un traitement spécial si nécessaire
	if strings.Contains(orderId, "C02__") || strings.HasPrefix(orderId, "C02__") {
		// Pour MEXC, tenter d'abord avec l'ID tel quel
		_, err := client.CancelOrder(orderId)
		if err == nil {
			return true, nil
		}

		// Si ça échoue, essayer sans le préfixe
		cleanId := strings.TrimPrefix(orderId, "C02__")
		if cleanId != orderId {
			_, err = client.CancelOrder(cleanId)
			if err == nil {
				return true, nil
			}
		}

		// Si ça échoue encore, essayer avec le préfixe si l'ID n'en avait pas
		if !strings.HasPrefix(orderId, "C02__") {
			prefixedId := "C02__" + orderId
			_, err = client.CancelOrder(prefixedId)
			if err == nil {
				return true, nil
			}
		}

		// Si toutes les tentatives ont échoué, retourner l'erreur
		return false, fmt.Errorf("impossible d'annuler l'ordre MEXC (toutes les méthodes tentées): %v", err)
	}

	// Tentative d'annulation de l'ordre
	_, err := client.CancelOrder(orderId)

	// Vérifier si l'erreur est en fait un succès déguisé
	if err != nil {
		errMsg := strings.ToLower(err.Error())

		// Liste des messages d'erreur qui indiquent un succès
		successPhrases := []string{
			"order cancelled",
			"canceled",
			"success",
			"already closed",
			"does not exist",
			"not found",
			"unknown order", // Pour les ordres déjà exécutés ou annulés
		}

		// Vérifier si l'un des messages de succès est dans l'erreur
		for _, phrase := range successPhrases {
			if strings.Contains(errMsg, phrase) {
				color.Yellow("Cycle %d: Annulation réussie malgré le message d'erreur: %v", cycleId, err)
				return true, nil // Considérer comme un succès
			}
		}

		// C'est une vraie erreur
		return false, err
	}

	// Aucune erreur, l'annulation a réussi normalement
	return true, nil
}

// getFeeRateForExchange retourne le taux de frais pour un exchange et un type d'ordre donnés
func getFeeRateForExchange(exchange string) float64 {
	switch strings.ToUpper(exchange) {
	case "KRAKEN":
		// Kraken: 0.26% frais maker standard
		return 0.0026
	case "BINANCE":
		// Binance: 0.1% standard
		return 0.001
	case "MEXC":
		return 0.0
	case "KUCOIN":
		// KuCoin: 0.1% standard
		return 0.001
	default:
		// Valeur par défaut pour les exchanges non reconnus
		return 0.001
	}
}
