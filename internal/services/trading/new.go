package commands

import (
	"fmt"
	"log"
	"main/internal/config"
	"os"
	"strconv"

	"github.com/fatih/color"
)

func CalcAmountUSD(freeBalance float64, percentStr string) float64 {
	percent, err := strconv.ParseFloat(percentStr, 64)
	if err != nil {
		log.Fatal(err)
	}
	return percent * freeBalance / 100
}

func CalcAmountBTC(availableUSD, priceBTC float64) float64 {
	return availableUSD / priceBTC
}

func FormatSmallFloat(quantity float64) string {
	return fmt.Sprintf("%.6f", quantity)
}

// New crée des nouveaux cycles sur tous les exchanges configurés
func New() {
	// Récupérer la configuration globale
	cfg, err := config.LoadConfig()
	if err != nil {
		color.Red("Erreur de configuration: %v", err)
		os.Exit(1)
	}

	// Parcourir tous les exchanges configurés
	for exchangeName, exchangeConfig := range cfg.Exchanges {
		// Vérifier si l'exchange est configuré et activé
		if exchangeConfig.APIKey == "" || exchangeConfig.SecretKey == "" {
			color.Yellow("Exchange %s non configuré ou désactivé (clés API manquantes), ignoré", exchangeName)
			continue
		}

		// Créer un cycle pour cet exchange
		NewWithExchange(exchangeName)
	}
}
