package main

import (
	"fmt"
	"log"
	"strings"

	"main/internal/config"
	"main/internal/database"
	commands "main/internal/services/trading"
)

func menu() {
	fmt.Println("")
	fmt.Println("Cryptomancien - Neodream - BOT SPOT - v5.0.0 - alpha")
	fmt.Println("")
	fmt.Println("--new            -n      Start new cycle")
	fmt.Println("--update         -u      Update running cycles")
	fmt.Println("--server         -s      Start local server")
	fmt.Println("--server         -s -complete      Start server with completed cycles only")
	fmt.Println("--stats          -st     Start statistics server (visualization and comparison)")
	fmt.Println("--cancel         -c      Cancel cycle by id - Example: -c=123")
	fmt.Println("--plan                   Configure and manage scheduled tasks for WINDOWS")
	fmt.Println("--plan           -plan start   Start the scheduler daemon")
	fmt.Println("--plan           -plan stop    Stop the scheduler daemon")
	fmt.Println("--plan           -plan status  Check scheduler status")
	fmt.Println("--remove-task    -plan -rt     Supprimer une tâche planifiée")
	fmt.Println("--remove-all     -plan -ra     Supprimer toutes les tâches planifiées")
	fmt.Println("")
	fmt.Println("Options additionnelles:")
	fmt.Println("-exchangebinance        Utiliser Binance pour cette commande")
	fmt.Println("-exchangemexc           Utiliser MEXC pour cette commande")
	fmt.Println("-exchangekucoin         Utiliser KuCoin pour cette commande")
	fmt.Println("-exchangeokx            Utiliser OKX pour cette commande")
	fmt.Println("-exchangekraken         Utiliser Kraken pour cette commande")
	fmt.Println("")
	fmt.Println("Exemples:")
	fmt.Println("-n -exchangemexc        Démarrer un nouveau cycle sur MEXC")
	fmt.Println("-u -exchangebinance     Mettre à jour les cycles sur Binance")
	fmt.Println("-n -exchangekucoin      Démarrer un nouveau cycle sur KuCoin")
	fmt.Println("-n -exchangeokx         Démarrer un nouveau cycle sur OKX")
	fmt.Println("-n -exchangekraken      Démarrer un nouveau cycle sur Kraken")
	fmt.Println("-plan                   Configurer le planificateur de tâches")
	fmt.Println("")
}

func initialize() {
	// Charger la configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialiser la base de données
	database.InitDatabase()

	// Passer la configuration aux commandes
	commands.SetConfig(cfg)
}

func extractExchangeFromArgs() string {
	// Patterns pour reconnaître les exchanges en arguments
	exchangePatterns := map[string]string{
		"exchangebinance": "BINANCE",
		"exchangemexc":    "MEXC",
		"exchangekucoin":  "KUCOIN",
		"exchangekraken":  "KRAKEN",
	}

	// Parcourir tous les arguments
	for _, arg := range commands.GetAllArgs() {
		// Supprimer les tirets au début
		cleanArg := strings.TrimLeft(arg, "-")

		// Vérifier si l'argument correspond à un exchange
		for pattern, exchange := range exchangePatterns {
			if strings.EqualFold(cleanArg, pattern) {
				return exchange
			}
		}
	}

	// Aucun exchange spécifié, retourner une chaîne vide
	return ""
}

func main() {
	// Vérifier d'abord si c'est une commande liée au planificateur
	if checkPlannerSubCommand() {
		return
	}

	// Initialiser les ressources communes
	initialize()
	defer database.CloseDatabase()

	// Rechercher les commandes dans tous les arguments
	args := commands.GetAllArgs()

	// Variable pour indiquer si une commande a été trouvée et exécutée
	commandFound := false

	// Vérifier quelle commande est présente
	for _, arg := range args {
		// Vérifier d'abord les formes avec "=" comme "-c=4" ou "--cancel=4"
		if strings.HasPrefix(arg, "-c=") || strings.HasPrefix(arg, "--cancel=") {
			// Extraire l'exchange spécifié dans les arguments
			exchange := extractExchangeFromArgs()
			// Passer l'argument complet à CancelWithExchange
			commands.CancelWithExchange(exchange, arg)
			commandFound = true
			return
		}

		// Puis vérifier les commandes régulières
		switch arg {
		case "--new", "-n":
			// Extraire l'exchange spécifié dans les arguments (s'il y en a un)
			exchange := extractExchangeFromArgs()
			commands.NewWithExchange(exchange)
			commandFound = true
			return

		case "--update", "-u":
			exchange := extractExchangeFromArgs()
			commands.UpdateWithExchange(exchange)
			commandFound = true
			return

		case "--cancel", "-c":
			// Cette branche gère le cas où "-c" est un argument séparé
			// Ce qui est différent de "-c=4"
			exchange := extractExchangeFromArgs()
			// Passer l'argument complet à CancelWithExchange
			commands.CancelWithExchange(exchange, arg)
			commandFound = true
			return

		case "--server", "-s":
			commands.Server()
			commandFound = true
			return

		case "--stats", "-st":
			// Nouvelle commande pour lancer le serveur de statistiques
			commands.StatsServer()
			commandFound = true
			return
		}
	}

	// Si aucune commande reconnue n'est trouvée, afficher le menu
	if !commandFound {
		menu()
	}
}
