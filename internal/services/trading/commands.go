// internal/services/trading/commands.go
package commands

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"main/internal/config"
	"main/internal/database"
	"main/internal/exchanges/binance"
	"main/internal/exchanges/common"
	"main/internal/exchanges/kraken"
	"main/internal/exchanges/kucoin"
	"main/internal/exchanges/mexc"

	"github.com/buger/jsonparser"
	"github.com/fatih/color"
)

// Configuration globale pour les commandes
var cfg *config.Config

// GetAllArgs retourne tous les arguments de la ligne de commande
func GetAllArgs() []string {
	return os.Args[1:] // Retourne tous les arguments sauf le nom du programme
}

// SetConfig permet de définir la configuration pour toutes les commandes
func SetConfig(config *config.Config) {
	cfg = config
}

// GetLastArg retourne le dernier argument de la ligne de commande
func GetLastArg() string {
	args := os.Args
	argsLen := len(args)
	if argsLen <= 1 {
		return ""
	}
	return args[argsLen-1]
}

// GetClientByExchange retourne un client pour l'échange spécifié
func GetClientByExchange(exchangeArg ...string) common.Exchange {
	// Récupérer le nom de l'exchange
	var ex string
	if len(exchangeArg) > 0 {
		ex = exchangeArg[0]
	} else {
		ex = cfg.Exchange()
	}
	ex = strings.ToUpper(ex)

	// Vérifier les clés API
	if cfg.Exchanges[ex].APIKey == "" || cfg.Exchanges[ex].SecretKey == "" {
		color.Red(fmt.Sprintf("%s_API_KEY and %s_SECRET_KEY must be set in bot.conf", ex, ex))
		os.Exit(0)
	}

	var client common.Exchange

	// Sélectionner dynamiquement le client en fonction de l'exchange
	switch ex {
	case "BINANCE":
		client = binance.NewClient(cfg.Exchanges[ex].APIKey, cfg.Exchanges[ex].SecretKey)
	case "MEXC":
		client = mexc.NewClient(cfg.Exchanges[ex].APIKey, cfg.Exchanges[ex].SecretKey)
	case "KUCOIN": // Ajout du cas pour KuCoin
		client = kucoin.NewClient(cfg.Exchanges[ex].APIKey, cfg.Exchanges[ex].SecretKey)
	case "KRAKEN": // Ajouter ce cas
		client = kraken.NewClient(cfg.Exchanges[ex].APIKey, cfg.Exchanges[ex].SecretKey)
	default:
		color.Red("Unsupported exchange: %s. Defaulting to Binance.", ex)
		client = binance.NewClient(cfg.APIKey(), cfg.SecretKey())
	}
	return client
}

func CancelAll() {
	color.Yellow("Annulation de tous les ordres d'achat en cours...")

	// Récupérer le repository
	repo := database.GetRepository()

	// Récupérer tous les cycles
	cycles, err := repo.FindAll()
	if err != nil {
		color.Red("Erreur lors de la récupération des cycles: %v", err)
		os.Exit(1)
	}

	// Obtenir le client d'échange
	client := GetClientByExchange()

	// Compteurs pour le suivi
	countCancelled := 0
	countFailed := 0

	// Traiter chaque cycle
	for _, cycle := range cycles {
		// Ne traiter que les cycles avec le statut "buy"
		if cycle.Status != "buy" {
			continue
		}

		// Nettoyer l'ID d'ordre avec l'exchange spécifique
		cleanOrderId := cleanOrderId(cycle.BuyId, cycle.Exchange)
		if cleanOrderId == "" {
			color.Red("ID d'ordre invalide pour le cycle %d: %s", cycle.IdInt, cycle.BuyId)
			countFailed++
			continue
		}

		// Annuler l'ordre d'achat
		color.White("Annulation de l'ordre d'achat %s pour le cycle %d...", cleanOrderId, cycle.IdInt)
		_, err := client.CancelOrder(cleanOrderId)
		if err != nil {
			color.Red("Échec de l'annulation de l'ordre pour le cycle %d: %v", cycle.IdInt, err)
			countFailed++
			continue
		}

		// Supprimer le cycle de la base de données
		err = repo.DeleteByIdInt(cycle.IdInt)
		if err != nil {
			color.Red("Erreur lors de la suppression du cycle %d: %v", cycle.IdInt, err)
			countFailed++
			continue
		}

		color.Green("Cycle %d supprimé avec succès", cycle.IdInt)
		countCancelled++
	}

	// Afficher le résumé des opérations
	fmt.Println("")
	if countCancelled == 0 && countFailed == 0 {
		color.Yellow("Aucun ordre d'achat en cours trouvé.")
	} else {
		color.Cyan("Résumé des opérations:")
		color.Green("  %d ordre(s) annulé(s) avec succès", countCancelled)
		if countFailed > 0 {
			color.Red("  %d ordre(s) n'ont pas pu être annulé(s)", countFailed)
		}
	}
}

// Si aucun exchange n'est spécifié, il utilisera la méthode standard
// Si aucun exchange n'est spécifié, il utilisera la méthode standard
func NewWithExchange(exchange string) {
	// Si aucun exchange n'est spécifié, utiliser la méthode standard
	if exchange == "" {
		New()
		return
	}

	// Récupérer les paramètres de configuration pour l'exchange spécifié en utilisant
	// les fonctions existantes qui lisent depuis bot.conf
	percent := getExchangePercent(exchange)

	buyOffsetStr := getExchangeParam(exchange, "BUY_OFFSET", "-700")
	buyOffset, _ := strconv.ParseFloat(buyOffsetStr, 64)
	buyOffset = math.Abs(buyOffset) // Convertir en valeur positive pour le calcul

	sellOffsetStr := getExchangeParam(exchange, "SELL_OFFSET", "700")
	sellOffset, _ := strconv.ParseFloat(sellOffsetStr, 64)
	sellOffset = math.Abs(sellOffset) // Convertir en valeur positive

	// Ces valeurs peuvent être utilisées plus tard dans le code si nécessaire
	// buyMaxDays, _ := strconv.Atoi(buyMaxDaysStr)
	// buyMaxDeviation, _ := strconv.ParseFloat(buyMaxDeviationStr, 64)

	// Initialiser le client d'échange spécifique
	client := GetClientByExchange(exchange)
	client.CheckConnection()

	// Récupérer le solde disponible
	freeBalance := client.GetBalanceUSD()
	color.White("Solde USD disponible sur %s: %.2f", exchange, freeBalance)
	if freeBalance < 10 {
		color.Red("Un minimum de 10$ est nécessaire sur %s", exchange)
		return // Continuer avec les autres exchanges en cas d'échec
	}

	// Récupérer le prix actuel du BTC
	btcPrice := client.GetLastPriceBTC()
	fmt.Printf("%s %s\n",
		color.CyanString("Prix BTC actuel sur %s:", exchange),
		color.YellowString("%.2f", btcPrice),
	)

	// Calculer le montant pour le nouveau cycle
	newCycleUSDC := CalcAmountUSD(freeBalance, percent)
	fmt.Printf("%s %s\n",
		color.CyanString("USD pour ce nouveau cycle:"),
		color.YellowString("%.2f", newCycleUSDC),
	)

	// Calculer la quantité de BTC à acheter
	newCycleBTC := CalcAmountBTC(newCycleUSDC, btcPrice)
	newCycleBTCFormated := FormatSmallFloat(newCycleBTC)
	fmt.Printf("%s %s\n",
		color.CyanString("BTC pour ce nouveau cycle:"),
		color.YellowString(newCycleBTCFormated),
	)

	// Calculer les prix d'achat et de vente en utilisant les offsets
	// Comme BUY_OFFSET est généralement négatif dans le fichier bot.conf,
	// on le soustrait au prix actuel (on a converti en valeur positive précédemment)
	buyPrice := btcPrice - buyOffset
	fmt.Printf("%s %s\n",
		color.CyanString("Prix d'achat:"),
		color.YellowString("%.2f", buyPrice),
	)

	// SELL_OFFSET est généralement positif, on l'ajoute au prix actuel
	sellPrice := btcPrice + sellOffset
	fmt.Printf("%s %s\n",
		color.CyanString("Prix de vente:"),
		color.YellowString("%.2f", sellPrice),
	)

	// Préparer l'ordre d'achat
	buyPriceStr := fmt.Sprintf("%.2f", buyPrice)

	// Créer l'ordre d'achat
	body, err := client.CreateOrder("BUY", buyPriceStr, newCycleBTCFormated)
	if err != nil {
		color.Red("Échec de l'ordre sur %s: %v", exchange, err)
		return // Continuer avec les autres exchanges en cas d'échec
	}

	// Extraire l'ID de l'ordre
	orderIdValue, dataType, _, err := jsonparser.Get(body, "orderId")
	if err != nil {
		color.Red("Erreur lors de l'extraction de l'ID d'ordre: %v", err)
		return
	}

	// Extraction et nettoyage cohérent de l'ID
	var orderIdStr string
	switch dataType {
	case jsonparser.String:
		orderIdStr = strings.TrimSpace(string(orderIdValue))
	case jsonparser.Number:
		orderIdStr = strings.TrimSpace(string(orderIdValue))
	default:
		color.Yellow("Type d'ID d'ordre inattendu: %v", dataType)
		orderIdStr = strings.TrimSpace(string(orderIdValue))
	}

	if exchange == "MEXC" {
		// Supprimer les préfixes spécifiques
		orderIdStr = strings.TrimPrefix(orderIdStr, "C02__")
	}

	// Créer un objet Cycle
	cycle := &database.Cycle{
		Exchange:  exchange,
		Status:    string(database.Status("buy")),
		Quantity:  newCycleBTC,
		BuyPrice:  buyPrice,
		BuyId:     orderIdStr,
		SellPrice: sellPrice,
		SellId:    "",
		CreatedAt: time.Now(),
	}

	// Enregistrer le cycle dans la base de données
	repo := database.GetRepository()
	_, err = repo.Save(cycle)
	if err != nil {
		color.Red("Erreur lors de l'enregistrement du cycle sur %s: %v", exchange, err)
		// Tenter d'annuler l'ordre si l'enregistrement échoue
		_, cancelErr := client.CancelOrder(orderIdStr)
		if cancelErr != nil {
			color.Red("Erreur lors de l'annulation de l'ordre après échec de sauvegarde: %v", cancelErr)
		}
		return
	}

	color.Green("Nouveau cycle créé avec succès sur %s", exchange)
}

// UpdateWithExchange exécute la commande Update avec un exchange spécifique
func UpdateWithExchange(exchange string) {
	// Si aucun exchange n'est spécifié, utiliser la méthode standard
	if exchange == "" {
		Update()
		return
	}

	// Initialiser le client pour cet exchange
	client := GetClientByExchange(exchange)

	// Afficher les informations de l'exchange
	color.Cyan("=== Informations pour %s ===", exchange)

	// Récupérer le prix actuel du BTC
	lastPrice := client.GetLastPriceBTC()
	color.White("Prix actuel du BTC: %.2f USDC", lastPrice)

	// Récupérer les soldes détaillés
	balances, err := client.GetDetailedBalances()
	if err != nil {
		color.Red("Erreur lors de la récupération des soldes pour %s: %v", exchange, err)
		return
	}

	// Afficher les soldes BTC
	btcBalance := balances["BTC"]
	color.Yellow("Solde BTC:")
	color.White("  Libre:      %.8f BTC (%.2f USDC)", btcBalance.Free, btcBalance.Free*lastPrice)
	color.White("  Verrouillé: %.8f BTC (%.2f USDC)", btcBalance.Locked, btcBalance.Locked*lastPrice)
	color.White("  Total:      %.8f BTC (%.2f USDC)", btcBalance.Total, btcBalance.Total*lastPrice)

	// Afficher les soldes USDC
	usdcBalance := balances["USDC"]
	color.Yellow("Solde USDC:")
	color.White("  Libre:      %.2f USDC", usdcBalance.Free)
	color.White("  Verrouillé: %.2f USDC", usdcBalance.Locked)
	color.White("  Total:      %.2f USDC", usdcBalance.Total)

	fmt.Println("") // Ligne vide pour séparer les sections

	// Récupérer tous les cycles depuis le repository
	repo := database.GetRepository()
	allCycles, err := repo.FindAll()
	if err != nil {
		color.Red("Erreur lors de la récupération des cycles: %v", err)
		return
	}

	// Filtrer les cycles pour l'exchange spécifié
	var cycles []*database.Cycle
	for _, cycle := range allCycles {
		if cycle.Exchange == exchange {
			cycles = append(cycles, cycle)
		}
	}

	if len(cycles) == 0 {
		color.Yellow("Aucun cycle trouvé pour l'exchange %s", exchange)
		return
	}

	// Traiter chaque cycle
	for _, cycle := range cycles {
		// Traiter le cycle en fonction de son statut
		switch cycle.Status {
		case "buy":
			processBuyCycle(client, repo, cycle, lastPrice)
		case "sell":
			processSellCycle(client, repo, cycle)
		case "completed":
			// Pas d'action nécessaire pour les cycles complétés
			continue
		}
	}

	// Afficher l'historique des cycles filtrés
	displayCyclesHistory(cycles, 0)
}

func CancelWithExchange(exchange string, cancelArg string) {
	// Si aucun exchange n'est spécifié, utiliser la méthode standard
	if exchange == "" {
		Cancel(cancelArg)
		return
	}

	// Extraire l'ID du cycle à annuler
	var idStr string

	// Vérifier si l'argument est de la forme "-c=ID" ou "--cancel=ID"
	if strings.HasPrefix(cancelArg, "-c=") || strings.HasPrefix(cancelArg, "--cancel=") {
		parts := strings.Split(cancelArg, "=")
		if len(parts) != 2 {
			color.Red("Format d'ID invalide. Utilisez -c=NOMBRE")
			os.Exit(1)
		}
		idStr = parts[1]
	} else {
		// Gérer le cas où l'ID pourrait être dans l'argument suivant
		// Cela n'est pas utilisé actuellement mais pourrait être ajouté si nécessaire
		color.Red("Format d'ID invalide. Utilisez -c=NOMBRE")
		os.Exit(1)
	}

	// Convertir l'ID en nombre entier
	idInt, err := strconv.Atoi(idStr)
	if err != nil {
		color.Red("ID invalide: %s", idStr)
		os.Exit(1)
	}

	color.White("Annulation du cycle %d sur %s...", idInt, exchange)

	// Récupérer le cycle depuis le repository
	repo := database.GetRepository()
	cycle, err := repo.FindByIdInt(int32(idInt))
	if err != nil {
		color.Red("Erreur lors de la récupération du cycle: %v", err)
		os.Exit(1)
	}

	if cycle == nil {
		color.Red("Cycle avec ID %d introuvable", idInt)
		os.Exit(1)
	}

	// Vérifier si le cycle appartient à l'exchange spécifié
	// Si un exchange est spécifié mais que le cycle appartient à un autre exchange
	if exchange != "" && cycle.Exchange != exchange {
		color.Red("Le cycle %d appartient à l'exchange %s, pas à %s", idInt, cycle.Exchange, exchange)
		os.Exit(1)
	}

	// Récupérer les informations du cycle
	status := cycle.Status

	// Obtenir le client de l'échange approprié pour le cycle
	client := GetClientByExchange(cycle.Exchange)

	// Annuler l'ordre uniquement si le statut est "buy" ou "sell"
	if status == "buy" || status == "sell" {
		var orderIdToCancel string
		if status == "buy" {
			orderIdToCancel = cycle.BuyId
			color.Yellow("Annulation de l'ordre d'achat %s", orderIdToCancel)
		} else {
			orderIdToCancel = cycle.SellId
			color.Yellow("Annulation de l'ordre de vente %s", orderIdToCancel)
		}

		// Nettoyer l'ID de l'ordre avec l'exchange spécifique
		cleanOrderId := cleanOrderId(orderIdToCancel, cycle.Exchange)
		if cleanOrderId == "" {
			color.Red("ID d'ordre invalide: %s", orderIdToCancel)
		} else {
			// Annuler l'ordre
			res, err := client.CancelOrder(cleanOrderId)
			if err != nil {
				color.Red("Échec de l'annulation de l'ordre: %v", err)
				// Continuer malgré l'erreur pour supprimer le cycle de la base de données
			} else {
				color.Green("Ordre annulé avec succès:")
				fmt.Println(string(res))
			}
		}
	} else {
		color.Yellow("Le cycle a le statut '%s', aucun ordre à annuler, suppression de la base de données uniquement", status)
	}

	// Supprimer le cycle de la base de données
	err = repo.DeleteByIdInt(int32(idInt))
	if err != nil {
		color.Red("Erreur lors de la suppression du cycle: %v", err)
		os.Exit(1)
	}

	color.Green("Cycle %d supprimé avec succès", idInt)
}

// CancelAllWithExchange annule tous les ordres d'achat d'un exchange spécifique
func CancelAllWithExchange(exchange string) {
	// Si aucun exchange n'est spécifié, utiliser la méthode standard
	if exchange == "" {
		CancelAll()
		return
	}

	color.Yellow("Annulation de tous les ordres d'achat en cours sur %s...", exchange)

	// Récupérer le repository
	repo := database.GetRepository()

	// Récupérer tous les cycles
	cycles, err := repo.FindAll()
	if err != nil {
		color.Red("Erreur lors de la récupération des cycles: %v", err)
		os.Exit(1)
	}

	// Filtrer les cycles pour l'exchange spécifié
	var exchangeCycles []*database.Cycle
	for _, cycle := range cycles {
		if cycle.Exchange == exchange {
			exchangeCycles = append(exchangeCycles, cycle)
		}
	}

	if len(exchangeCycles) == 0 {
		color.Yellow("Aucun cycle trouvé pour l'exchange %s", exchange)
		return
	}

	// Obtenir le client d'échange
	client := GetClientByExchange(exchange)

	// Compteurs pour le suivi
	countCancelled := 0
	countFailed := 0

	// DIAGNOSTIC: Afficher tous les cycles avec leurs IDs
	color.Cyan("=== INFORMATIONS DE DIAGNOSTIC ===")
	for _, cycle := range exchangeCycles {
		color.White("Cycle %d - Exchange: %s - Status: %s - BuyId: '%s' - SellId: '%s'",
			cycle.IdInt, cycle.Exchange, cycle.Status, cycle.BuyId, cycle.SellId)
	}
	color.Cyan("===============================")

	// Traiter chaque cycle
	for _, cycle := range exchangeCycles {
		// Ne traiter que les cycles avec le statut "buy"
		if cycle.Status != "buy" {
			continue
		}

		// DIAGNOSTIC: Afficher l'ID original
		color.Yellow("DIAGNOSTIC - Cycle %d - ID original: '%s'", cycle.IdInt, cycle.BuyId)

		// Essayer différentes façons de nettoyer l'ID pour comparer
		cleanId1 := cleanOrderId(cycle.BuyId)
		cleanId2 := cleanOrderId(cycle.BuyId, cycle.Exchange)
		rawNumericId := regexp.MustCompile(`[^0-9]`).ReplaceAllString(cycle.BuyId, "")

		color.Yellow("DIAGNOSTIC - Différentes versions de l'ID:")
		color.Yellow("  - cleanOrderId sans exchange: '%s'", cleanId1)
		color.Yellow("  - cleanOrderId avec exchange: '%s'", cleanId2)
		color.Yellow("  - Extraction numérique simple: '%s'", rawNumericId)

		// Essayer d'utiliser directement l'ID original
		color.White("Tentative d'annulation avec l'ID original: '%s'", cycle.BuyId)
		_, errOriginal := client.CancelOrder(cycle.BuyId)
		if errOriginal != nil {
			color.Red("Échec avec ID original: %v", errOriginal)
		} else {
			color.Green("Succès avec ID original!")
			// Si ça a fonctionné, supprimer le cycle et continuer
			repo.DeleteByIdInt(cycle.IdInt)
			countCancelled++
			continue
		}

		// Si l'ID original échoue, essayer avec la version nettoyée standard
		if cleanId2 != "" && cleanId2 != cycle.BuyId {
			color.White("Tentative avec ID nettoyé: '%s'", cleanId2)
			_, errClean := client.CancelOrder(cleanId2)
			if errClean != nil {
				color.Red("Échec avec ID nettoyé: %v", errClean)
			} else {
				color.Green("Succès avec ID nettoyé!")
				// Si ça a fonctionné, supprimer le cycle et continuer
				repo.DeleteByIdInt(cycle.IdInt)
				countCancelled++
				continue
			}
		}

		// Si nous sommes ici, aucune tentative n'a réussi
		color.Red("Toutes les tentatives d'annulation ont échoué pour le cycle %d", cycle.IdInt)
		countFailed++
	}

	// Afficher le résumé des opérations
	fmt.Println("")
	if countCancelled == 0 && countFailed == 0 {
		color.Yellow("Aucun ordre d'achat en cours trouvé pour %s.", exchange)
	} else {
		color.Cyan("Résumé des opérations pour %s:", exchange)
		color.Green("  %d ordre(s) annulé(s) avec succès", countCancelled)
		if countFailed > 0 {
			color.Red("  %d ordre(s) n'ont pas pu être annulé(s)", countFailed)
		}
	}
}

// Fonction utilitaire pour récupérer des paramètres spécifiques à un exchange
func getExchangeParam(exchange, param, defaultValue string) string {
	paramName := fmt.Sprintf("%s_%s", exchange, param)
	value := os.Getenv(paramName)
	if value == "" {
		defaultParamName := fmt.Sprintf("DEFAULT_%s", param)
		defaultFromConfig := os.Getenv(defaultParamName)
		if defaultFromConfig != "" {
			return defaultFromConfig
		}
		return defaultValue
	}
	return value

}

// Fonction pour récupérer le pourcentage spécifique à un exchange
func getExchangePercent(exchange string) string {
	percentStr := getExchangeParam(exchange, "PERCENT", "5")
	return percentStr
}

func MigrateCompletedCyclesDates() {
	color.Yellow("Migration des dates de complétion pour les cycles complétés...")

	repo := database.GetRepository()
	cycles, err := repo.FindAll()
	if err != nil {
		color.Red("Erreur lors de la récupération des cycles: %v", err)
		return
	}

	correctedCount := 0

	for _, cycle := range cycles {
		if cycle.Status == "completed" && cycle.CompletedAt.Before(cycle.CreatedAt) {
			// Calculer une nouvelle date de complétion raisonnable
			newCompletionTime := cycle.CreatedAt.Add(6 * time.Hour)

			// Mettre à jour le cycle
			err = repo.UpdateByIdInt(cycle.IdInt, map[string]interface{}{
				"completedAt": newCompletionTime.Format(time.RFC3339),
			})

			if err != nil {
				color.Red("Erreur lors de la mise à jour du cycle %d: %v", cycle.IdInt, err)
			} else {
				correctedCount++
				color.Green("Cycle %d corrigé: CompletedAt de %s à %s",
					cycle.IdInt,
					cycle.CompletedAt.Format("02/01/2006 15:04"),
					newCompletionTime.Format("02/01/2006 15:04"))
			}
		}
	}

	color.Green("%d cycles ont été corrigés", correctedCount)
}
