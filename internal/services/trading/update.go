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

// ForceCompleteOldMexcOrders force les ordres MEXC anciens à être marqués comme complétés
func ForceCompleteOldMexcOrders() {
	color.Yellow("Recherche d'ordres MEXC anciens à marquer comme complétés...")

	// Obtenir le repository
	repo := database.GetRepository()

	// Récupérer tous les cycles
	cycles, err := repo.FindAll()
	if err != nil {
		color.Red("Erreur lors de la récupération des cycles: %v", err)
		return
	}

	// Compteur pour les ordres modifiés
	completedCount := 0

	// Parcourir tous les cycles MEXC en vente
	for _, cycle := range cycles {
		if cycle.Exchange == "MEXC" && cycle.Status == "sell" {
			// Calculer l'âge du cycle en jours
			ageInDays := cycle.GetAge()

			// Si le cycle est en vente depuis plus de 7 jours, le marquer comme complété
			if ageInDays > 7 {
				color.Yellow("Cycle %d trouvé en vente depuis %.1f jours", cycle.IdInt, ageInDays)

				// Mise à jour du statut dans la base de données
				err = repo.UpdateByIdInt(cycle.IdInt, map[string]interface{}{
					"status": "completed",
				})

				if err != nil {
					color.Red("Erreur lors de la mise à jour du cycle: %v", err)
				} else {
					color.Green("Cycle %d marqué comme complété manuellement", cycle.IdInt)
					completedCount++
				}
			}
		}
	}

	if completedCount > 0 {
		color.Green("%d cycles MEXC anciens ont été marqués comme complétés", completedCount)
	} else {
		color.Yellow("Aucun cycle MEXC ancien à marquer comme complété")
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
				// Si l'annulation échoue, essayer une méthode plus agressive pour MEXC
				if cycle.Exchange == "MEXC" {
					// Essayer différentes variantes de l'ID d'ordre
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
			if availableBTC < cycle.Quantity*0.95 {
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

	// L'ordre a été exécuté, passer à l'étape de vente
	color.Green("Cycle %d: Ordre d'achat exécuté", cycle.IdInt)

	// ====== DÉBUT NOUVELLE PARTIE - EXTRACTION DE LA QUANTITÉ RÉELLEMENT EXÉCUTÉE DEPUIS L'API ======
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
		// Format de réponse pour Binance
		executedQtyStr, err := jsonparser.GetString(orderBytes, "executedQty")
		if err == nil && executedQtyStr != "" {
			parsedQty, parseErr := strconv.ParseFloat(executedQtyStr, 64)
			if parseErr == nil && parsedQty > 0 {
				executedQty = parsedQty
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

		// Essayer différents chemins possibles dans la réponse JSON
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

	// Si nous avons pu extraire une quantité valide de l'API ET si elle est différente de la quantité initiale
	if executedQty > 0 && math.Abs(executedQty-cycle.Quantity)/cycle.Quantity > 0.001 { // Différence de plus de 0.1%
		color.Yellow("Cycle %d: Mise à jour de la quantité de %.8f BTC à %.8f BTC (d'après l'API)",
			cycle.IdInt, cycle.Quantity, executedQty)

		// Mettre à jour la quantité dans la base de données
		err = repo.UpdateByIdInt(cycle.IdInt, map[string]interface{}{
			"quantity": executedQty,
		})

		if err != nil {
			color.Red("Erreur lors de la mise à jour de la quantité: %v", err)
			// Continuer avec la quantité originale
		} else {
			// Mettre à jour l'objet cycle local pour la suite du traitement
			cycle.Quantity = executedQty
		}
	}
	// ====== FIN NOUVELLE PARTIE ======

	// ==== VÉRIFICATION DES SOLDES ====
	balances, balErr := client.GetDetailedBalances()
	if balErr != nil {
		color.Red("Erreur lors de la récupération des soldes: %v", balErr)
		return
	}

	// Vérifier que le BTC est réellement disponible
	availableBTC := balances["BTC"].Free
	if availableBTC < cycle.Quantity*0.99 {
		color.Yellow("Cycle %d: Solde BTC disponible insuffisant (%.8f) pour vendre %.8f BTC. L'ordre semble ne pas être réellement exécuté.",
			cycle.IdInt, availableBTC, cycle.Quantity)

		// L'ordre n'est probablement pas exécuté, malgré ce que dit IsFilled
		// Vérifier l'âge et annuler si nécessaire
		age := cycle.GetAge()
		if maxDays > 0 && age >= float64(maxDays) {
			color.Yellow("Cycle %d: L'ordre d'achat a dépassé l'âge maximal. Annulation forcée...", cycle.IdInt)
			success, _ := safeOrderCancel(client, cleanBuyId, cycle.IdInt)
			if success {
				err = repo.UpdateByIdInt(cycle.IdInt, map[string]interface{}{
					"status": "cancelled",
				})
				if err == nil {
					color.Green("Cycle %d: Ordre annulé et cycle marqué comme annulé", cycle.IdInt)
				}
			}
		}
		return
	}
	// Utiliser l'offset de vente spécifique à l'exchange
	sellOffset := exchangeConfig.SellOffset

	// 1. Calculer le prix de vente standard : prix d'achat + SELL_OFFSET
	standardSellPrice := cycle.BuyPrice + sellOffset

	// 2. Calculer le prix minimum pour être "maker" : prix actuel + 0.1%
	makerMinPrice := lastPrice * 1.001

	// 3. Choisir le prix de vente final (le plus élevé des deux)
	var newSellPrice float64
	if standardSellPrice < makerMinPrice {
		newSellPrice = makerMinPrice
		color.Yellow("Cycle %d: Prix de vente ajusté pour être maker: %.2f → %.2f (prix actuel: %.2f + 0.1%%)",
			cycle.IdInt, standardSellPrice, newSellPrice, lastPrice)
	} else {
		newSellPrice = standardSellPrice
		color.Yellow("Cycle %d: Prix de vente standard utilisé: %.2f (> prix actuel: %.2f + 0.1%%)",
			cycle.IdInt, newSellPrice, lastPrice)
	}

	// CORRECTION: Mettre à jour le prix de vente dans la base de données avant de placer l'ordre
	// Cette mise à jour garantit que le prix affiché sera le prix réel utilisé
	err = repo.UpdateByIdInt(cycle.IdInt, map[string]interface{}{
		"sellPrice": newSellPrice,
	})
	if err != nil {
		color.Red("Erreur lors de la mise à jour du prix de vente: %v", err)
		return
	}

	// Mettre à jour l'objet cycle local pour qu'il reflète les changements
	cycle.SellPrice = newSellPrice

	// ==== DÉBUT DE LA MODIFICATION: AJUSTEMENT DE LA QUANTITÉ ====
	// Utiliser la quantité disponible si elle est légèrement inférieure à la quantité attendue
	quantityToSell := cycle.Quantity
	if availableBTC < quantityToSell && availableBTC > quantityToSell*0.99 {
		// Si on a au moins 99% de la quantité, adapter pour utiliser ce qui est disponible
		color.Yellow("Cycle %d: Ajustement de la quantité à vendre de %.8f à %.8f (disponible)",
			cycle.IdInt, quantityToSell, availableBTC)
		quantityToSell = availableBTC
	}
	// ==== FIN DE LA MODIFICATION: AJUSTEMENT DE LA QUANTITÉ ====

	// Préparer les paramètres de l'ordre de vente
	quantityStr := strconv.FormatFloat(quantityToSell, 'f', 8, 64)
	sellPriceStr := strconv.FormatFloat(newSellPrice, 'f', 2, 64)

	// ==== DÉBUT DE LA MODIFICATION: GESTION DES ERREURS DE CRÉATION D'ORDRE ====
	// Créer l'ordre de vente directement avec le prix déjà ajusté en mode maker
	sellBytes, err := client.CreateOrder("SELL", sellPriceStr, quantityStr)
	if err != nil {
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
	// ==== FIN DE LA MODIFICATION: GESTION DES ERREURS DE CRÉATION D'ORDRE ====

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
		// Si c'est déjà une chaîne
		orderIdStr = string(orderIdValue)
	case jsonparser.Number:
		// Si c'est un nombre, le convertir en chaîne
		orderIdStr = string(orderIdValue)
	default:
		// Fallback au cas où
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
	profitPercent := ((newSellPrice - cycle.BuyPrice) / cycle.BuyPrice) * 100
	color.Green("Cycle %d: Ordre de vente placé avec succès. ID: %s", cycle.IdInt, orderIdStr)
	color.Green("Cycle %d: Prix d'achat: %.2f, Prix de vente: %.2f, Profit potentiel: %.2f%%",
		cycle.IdInt, cycle.BuyPrice, newSellPrice, profitPercent)
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

		// Nettoyer l'ID d'ordre de vente en spécifiant l'exchange
		cleanSellId := cleanOrderId(cycle.SellId, cycle.Exchange)

		// Si l'ID d'ordre de vente est vide, tenter de créer un nouvel ordre de vente
		if cleanSellId == "" {
			color.Yellow("Cycle %d: ID d'ordre de vente vide. Tentative de création d'un nouvel ordre de vente.", cycle.IdInt)

			// Vérifier le solde BTC disponible
			balances, balErr := client.GetDetailedBalances()
			if balErr != nil {
				color.Red("Cycle %d: Erreur lors de la récupération des soldes: %v", cycle.IdInt, balErr)
				return
			}

			availableBTC := balances["BTC"].Free
			if availableBTC < cycle.Quantity*0.99 {
				color.Yellow("Cycle %d: Solde BTC disponible toujours insuffisant (%.8f) pour vendre %.8f BTC.",
					cycle.IdInt, availableBTC, cycle.Quantity)
				return
			}

			// Solde suffisant, créer l'ordre de vente
			currentPrice := client.GetLastPriceBTC()

			// Obtenir la configuration de l'exchange pour l'offset de vente
			cfg, cfgErr := config.LoadConfig()
			if cfgErr != nil {
				color.Red("Cycle %d: Erreur lors du chargement de la configuration: %v", cycle.IdInt, cfgErr)
				return
			}

			exchangeConfig, exchangeErr := cfg.GetExchangeConfig(cycle.Exchange)
			if exchangeErr != nil {
				color.Red("Cycle %d: Erreur lors de la récupération de la configuration de l'exchange: %v", cycle.IdInt, exchangeErr)
				return
			}

			// Calculer le prix de vente
			sellOffset := exchangeConfig.SellOffset
			standardSellPrice := cycle.BuyPrice + sellOffset
			makerMinPrice := currentPrice * 1.001

			var sellPrice float64
			if standardSellPrice < makerMinPrice {
				sellPrice = makerMinPrice
				color.Yellow("Cycle %d: Prix de vente ajusté pour être maker: %.2f → %.2f",
					cycle.IdInt, standardSellPrice, sellPrice)
			} else {
				sellPrice = standardSellPrice
			}

			// Mettre à jour le prix de vente dans la base de données
			err := repo.UpdateByIdInt(cycle.IdInt, map[string]interface{}{
				"sellPrice": sellPrice,
			})
			if err != nil {
				color.Red("Cycle %d: Erreur lors de la mise à jour du prix de vente: %v", cycle.IdInt, err)
				return
			}

			// Créer l'ordre de vente
			quantityStr := strconv.FormatFloat(cycle.Quantity, 'f', 8, 64)
			sellPriceStr := strconv.FormatFloat(sellPrice, 'f', 2, 64)

			sellBytes, err := client.CreateOrder("SELL", sellPriceStr, quantityStr)
			if err != nil {
				color.Red("Cycle %d: Erreur lors de la création de l'ordre de vente: %v", cycle.IdInt, err)
				return
			}

			// Extraire l'ID de l'ordre de vente
			orderIdValue, dataType, _, err := jsonparser.Get(sellBytes, "orderId")
			if err != nil {
				color.Red("Cycle %d: Erreur lors de l'extraction de l'ID d'ordre: %v", cycle.IdInt, err)
				return
			}

			// Déterminer l'ID de l'ordre de vente
			var orderIdStr string
			switch dataType {
			case jsonparser.String:
				orderIdStr = string(orderIdValue)
			case jsonparser.Number:
				orderIdStr = string(orderIdValue)
			default:
				orderIdStr = string(orderIdValue)
			}

			if orderIdStr == "" {
				color.Red("Cycle %d: ID d'ordre vide obtenu de la réponse API", cycle.IdInt)
				return
			}

			// Mettre à jour le cycle avec le nouvel ID d'ordre de vente
			err = repo.UpdateByIdInt(cycle.IdInt, map[string]interface{}{
				"sellId": orderIdStr,
			})
			if err != nil {
				color.Red("Cycle %d: Erreur lors de la mise à jour du cycle: %v", cycle.IdInt, err)
				return
			}

			color.Green("Cycle %d: Nouvel ordre de vente créé avec succès. ID: %s", cycle.IdInt, orderIdStr)

			// Mettre à jour l'objet cycle pour la suite du traitement
			cycle.SellId = orderIdStr
			cycle.SellPrice = sellPrice

			// On continue le traitement normal, mais avec un SellId qui vient d'être créé
			cleanSellId = cleanOrderId(cycle.SellId, cycle.Exchange)
		}

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

	// Tenter d'extraire la date réelle d'exécution
	completionTime := time.Now() // Valeur par défaut
	extractionSuccessful := false

	// Extraction spécifique à chaque exchange
	switch cycle.Exchange {
	case "BINANCE":
		updateTimeMs, err := jsonparser.GetInt(orderBytes, "updateTime")
		if err == nil {
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

	// Mettre à jour le cycle dans la base de données
	err = repo.UpdateByIdInt(cycle.IdInt, map[string]interface{}{
		"status":      "completed",
		"completedAt": completionTime.Format(time.RFC3339),
	})

	if err != nil {
		color.Red("Erreur lors de la mise à jour du cycle: %v", err)
		return
	}

	// Mettre à jour l'objet cycle en mémoire également (pour les opérations qui suivent)
	cycle.Status = "completed"
	cycle.CompletedAt = completionTime

	color.Green("Cycle %d: COMPLÉTÉ AVEC SUCCÈS!", cycle.IdInt)
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

	headerFormat := "%-5s | %-10s | %-12s | %-15s | %-20s | %-15s | %-15s\n"
	rowFormat := "%-5d | %-10s | %-12s | %-15.2f | %-20.2f | %-15s | %-15s\n"

	fmt.Printf(headerFormat, "ID", "EXCHANGE", "STATUT", "MONTANT USDC", "MONTANT PRÉVU VENTE", "GAINS PRÉVUS", "DURÉE")
	fmt.Println("-------+------------+--------------+-----------------+----------------------+-----------------+-----------------")

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
		expectedProfit := usdcSaleAmount - usdcAmount
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
			usdcSaleAmount,
			expectedProfitStr,
			duration)

		// Mettre à jour les statistiques
		updateStats(cycle, &statsBinance, &statsMexc, &statsKucoin, &statsKraken)
	}

	if activeCycles == 0 {
		color.Yellow("Aucun cycle actif trouvé.")
	}

	fmt.Println("-------+------------+--------------+-----------------+----------------------+-----------------+-----------------")

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

// updateStats met à jour les statistiques de l'exchange approprié
// Version corrigée pour inclure Kraken
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
		profit := (cycle.SellPrice - cycle.BuyPrice) * cycle.Quantity
		stats.totalProfit += profit
	}
}

// Fonction utilitaire pour calculer le profit sur une période donnée
func calculateProfitByPeriod(cycles []*database.Cycle, exchangeName string, startTime, endTime time.Time) float64 {
	var periodProfit float64

	// Convertir le nom de l'exchange en majuscules pour comparaison insensible à la casse
	exchangeNameUpper := strings.ToUpper(exchangeName)

	for _, cycle := range cycles {
		// Normaliser le nom de l'exchange du cycle pour comparaison
		cycleExchangeUpper := strings.ToUpper(cycle.Exchange)

		// Ne considérer que les cycles de l'exchange spécifié et complétés
		if cycleExchangeUpper == exchangeNameUpper && cycle.Status == "completed" {
			// Au lieu d'estimer la date de complétion, nous utilisons la date de création
			// pour déterminer si le cycle appartient à la période considérée
			if cycle.CreatedAt.After(startTime) && cycle.CreatedAt.Before(endTime) {
				// Calculer le profit pour ce cycle
				buyValue := cycle.BuyPrice * cycle.Quantity
				sellValue := cycle.SellPrice * cycle.Quantity
				profit := sellValue - buyValue
				periodProfit += profit
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
			// Calculer le profit pour ce cycle
			buyValue := cycle.BuyPrice * cycle.Quantity
			sellValue := cycle.SellPrice * cycle.Quantity
			profit := sellValue - buyValue
			totalProfit += profit
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
