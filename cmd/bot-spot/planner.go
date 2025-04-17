package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"main/internal/config"
	"main/internal/scheduler"
	commands "main/internal/services/trading"
	"main/internal/types" // Import du package types contenant TaskConfig
	"main/pkg/logger"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// plannerCmd gère la commande de planification interactive
func plannerCmd() {
	fmt.Println("=== Configuration du planificateur de tâches ===")

	// Initialiser la configuration et le logger
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("Erreur lors du chargement de la configuration: %v\n", err)
		return
	}

	log := logger.NewLogger(logger.LogConfig{
		Level:  "info",
		Format: "text",
	})

	// Créer le planificateur
	sched := scheduler.NewScheduler(cfg, log)

	// Charger les tâches existantes
	err = sched.LoadTasksFromConfig()
	if err != nil {
		fmt.Printf("Erreur lors du chargement des tâches: %v\n", err)
	}

	// Afficher les tâches existantes
	displayExistingTasks(sched)

	// Demander à l'utilisateur s'il veut ajouter une nouvelle tâche
	reader := bufio.NewReader(os.Stdin)
	fmt.Println("\nVoulez-vous configurer une nouvelle tâche planifiée ? (o/n)")
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response == "o" || response == "oui" || response == "y" || response == "yes" {
		addNewTaskInteractive(sched, reader)
	}

	//Demander à l'utilisateur s'il veut démarrer le planificateur
	/*fmt.Println("\nVoulez-vous démarrer le planificateur maintenant ? (o/n)")
	response, _ = reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response == "o" || response == "oui" || response == "y" || response == "yes" {
		startSchedulerInteractive(sched)
	} else {
		fmt.Println("\nConfiguration terminée. Pour démarrer le planificateur plus tard, utilisez:")
		fmt.Println("go run . -plan start")
	}*/
}

// displayExistingTasks affiche les tâches existantes
func displayExistingTasks(sched *scheduler.Scheduler) {
	tasks := sched.GetAllTasks()
	if len(tasks) == 0 {
		fmt.Println("\nAucune tâche planifiée n'est configurée actuellement.")
		return
	}

	fmt.Println("\nTâches planifiées existantes:")
	for i, task := range tasks {
		// Formater l'intervalle pour l'affichage
		intervalStr := ""
		if task.IntervalValue > 0 {
			var typesIntervalUnit types.TimeUnit
			switch task.IntervalUnit {
			case scheduler.Minutes:
				typesIntervalUnit = types.Minutes
			case scheduler.Hours:
				typesIntervalUnit = types.Hours
			case scheduler.Days:
				typesIntervalUnit = types.Days
			}
			intervalStr = formatIntervalToString(task.IntervalValue, typesIntervalUnit)
		} else {
			// Calculer à partir de l'Interval
			value, unit := durationToUserFriendly(task.Interval)
			intervalStr = formatIntervalToString(value, unit)
		}

		statusStr := "Activée"
		if !task.Enabled {
			statusStr = "Désactivée"
		}

		fmt.Printf("%d. %s - %s - Intervalle: %s - État: %s\n",
			i+1,
			task.Name,
			task.Type,
			intervalStr,
			statusStr)

		// Afficher des détails supplémentaires selon le type
		switch task.Type {
		case "update":
			if task.Exchange != "" {
				fmt.Printf("   Exchange spécifique: %s\n", task.Exchange)
			}
		case "new":
			if task.Exchange != "" {
				fmt.Printf("   Exchange: %s", task.Exchange)

				// Afficher les paramètres personnalisés
				customParams := []string{}
				if task.BuyOffset != 0 {
					customParams = append(customParams, fmt.Sprintf("BuyOffset: %.2f", task.BuyOffset))
				}
				if task.SellOffset != 0 {
					customParams = append(customParams, fmt.Sprintf("SellOffset: %.2f", task.SellOffset))
				}
				if task.Percent != 0 {
					customParams = append(customParams, fmt.Sprintf("Percent: %.2f", task.Percent))
				}

				if len(customParams) > 0 {
					fmt.Printf(" (%s)", strings.Join(customParams, ", "))
				}
				fmt.Println()
			}

			if task.SpecificTime != "" {
				fmt.Printf("   Heure d'exécution: %s\n", task.SpecificTime)
			}
		}

		// Afficher la prochaine exécution
		if !task.NextScheduledAt.IsZero() {
			fmt.Printf("   Prochaine exécution: %s\n",
				task.NextScheduledAt.Format("02/01/2006 15:04:05"))
		}
	}
}

// checkPlannerSubCommand vérifie les sous-commandes du planificateur
func checkPlannerSubCommand() bool {
	args := commands.GetAllArgs()
	for i, arg := range args {
		if arg == "--plan" || arg == "-plan" {
			// S'il y a un argument suivant, vérifier s'il s'agit d'une sous-commande
			if i+1 < len(args) {
				subCommand := args[i+1]

				// Vérifier si la sous-commande existe
				switch subCommand {
				case "start":
					startPlannerDaemon()
					return true
				case "stop":
					stopPlannerDaemon()
					return true
				case "status":
					checkPlannerStatus()
					return true
				case "-rt":
					removeTaskCmd()
					return true
				case "-ra":
					removeAllTasksCmd()
					return true
				case "daemon":
					// Cette option est utilisée en interne pour le mode daemon
					runPlannerDaemon()
					return true
				}
			}

			// Si aucune sous-commande spécifique, lancer la configuration interactive
			plannerCmd()
			return true
		} else if arg == "-plan-daemon" {
			// Option spéciale pour exécuter le daemon directement
			runPlannerDaemon()
			return true
		}
	}

	return false
}

// startPlannerDaemon démarre le planificateur en tant que daemon
func startPlannerDaemon() {
	fmt.Println("Démarrage du planificateur en tant que daemon...")

	// Sous Windows, créer un exécutable dédié au lieu d'utiliser go run
	var cmd *exec.Cmd

	// Détecter le chemin de l'exécutable actuel
	exePath, err := os.Executable()
	if err != nil {
		fmt.Printf("Erreur lors de la détection du chemin de l'exécutable: %v\n", err)
		// Fallback au go run standard si on ne peut pas déterminer le chemin
		cmd = exec.Command("go", "run", ".", "-plan-daemon")
	} else {
		// Utiliser l'exécutable lui-même avec le flag daemon
		// Cette approche garantit que le PID sera celui du processus qui continue à s'exécuter
		cmd = exec.Command(exePath, "-plan-daemon")
	}

	// Configuration pour Windows - utiliser CREATE_NEW_PROCESS_GROUP
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		}
	}

	// Rediriger la sortie vers un fichier log
	logFile, err := os.OpenFile("planner.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Printf("Erreur lors de la création du fichier log: %v\n", err)
		return
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	// Démarrer en arrière-plan
	err = cmd.Start()
	if err != nil {
		fmt.Printf("Erreur lors du démarrage du daemon: %v\n", err)
		return
	}

	// Enregistrer le PID dans le fichier
	pidFile, err := os.Create("planner.pid")
	if err != nil {
		fmt.Printf("Erreur lors de la création du fichier PID: %v\n", err)
		return
	}

	_, err = fmt.Fprintf(pidFile, "%d", cmd.Process.Pid)
	pidFile.Close()
	if err != nil {
		fmt.Printf("Erreur lors de l'écriture du PID: %v\n", err)
		return
	}

	// Astuce supplémentaire: enregistrer également le nom de l'exécutable
	// pour faciliter la recherche du processus lors de l'arrêt
	exeInfoFile, _ := os.Create("planner_exe.info")
	if exeInfoFile != nil {
		fmt.Fprintf(exeInfoFile, "%s\n%d", filepath.Base(exePath), cmd.Process.Pid)
		exeInfoFile.Close()
	}

	fmt.Println("Planificateur démarré avec succès (PID:", cmd.Process.Pid, ")")
}

// stopPlannerDaemon arrête le daemon du planificateur
func stopPlannerDaemon() {
	fmt.Println("Arrêt du planificateur...")

	// Stratégie en plusieurs phases pour trouver et arrêter le processus

	// 1. Essayer d'utiliser le fichier PID standard
	pidFound := false
	var pid int

	if pidData, err := os.ReadFile("planner.pid"); err == nil {
		if tmpPid, err := strconv.Atoi(strings.TrimSpace(string(pidData))); err == nil {
			pid = tmpPid
			pidFound = true
		}
	}

	// 2. Si le PID est trouvé, essayer de l'arrêter
	if pidFound {
		fmt.Printf("Tentative d'arrêt du processus avec PID %d...\n", pid)

		if runtime.GOOS == "windows" {
			cmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
			if err := cmd.Run(); err == nil {
				fmt.Println("Planificateur arrêté avec succès.")
				cleanupPlannerFiles()
				return
			} else {
				fmt.Printf("Impossible d'arrêter le processus %d: %v\n", pid, err)
				// Continuer avec les autres méthodes
			}
		} else {
			// Pour les systèmes Unix
			process, err := os.FindProcess(pid)
			if err == nil {
				if err := process.Signal(syscall.SIGTERM); err == nil {
					fmt.Println("Planificateur arrêté avec succès.")
					cleanupPlannerFiles()
					return
				}
			}
		}
	}

	// 3. Rechercher par nom de processus (bot-spot ou similaire)
	fmt.Println("Recherche du processus planificateur par nom...")

	if runtime.GOOS == "windows" {
		// Lire le nom de l'exécutable dans le fichier info si disponible
		var processName string
		if infoData, err := os.ReadFile("planner_exe.info"); err == nil {
			lines := strings.Split(string(infoData), "\n")
			if len(lines) > 0 {
				processName = strings.TrimSpace(lines[0])
			}
		}

		// Si on n'a pas pu lire le nom, utiliser des noms par défaut
		if processName == "" {
			processName = "bot-spot.exe"
		}

		// Tenter de trouver le processus par nom
		cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("IMAGENAME eq %s", processName), "/FO", "CSV")
		output, err := cmd.Output()
		if err == nil {
			// Analyser la sortie CSV
			lines := strings.Split(string(output), "\n")
			if len(lines) > 1 { // La première ligne est l'en-tête
				for _, line := range lines[1:] {
					if line == "" {
						continue
					}
					// Supprimer les guillemets et diviser par les virgules
					line = strings.ReplaceAll(line, "\"", "")
					parts := strings.Split(line, ",")
					if len(parts) >= 2 {
						pidStr := strings.TrimSpace(parts[1])
						if pidStr != "" {
							if _, err := strconv.Atoi(pidStr); err == nil {
								// Essayer de tuer ce processus
								killCmd := exec.Command("taskkill", "/F", "/PID", pidStr)
								if err := killCmd.Run(); err == nil {
									fmt.Printf("Processus %s avec PID %s arrêté avec succès.\n", processName, pidStr)
									pidFound = true
								}
							}
						}
					}
				}
			}
		}

		// Si on n'a toujours pas trouvé le processus, essayer avec "go.exe"
		if !pidFound {
			cmd = exec.Command("tasklist", "/FI", "IMAGENAME eq go.exe", "/FO", "CSV")
			output, err = cmd.Output()
			if err == nil {
				lines := strings.Split(string(output), "\n")
				if len(lines) > 1 {
					fmt.Println("Processus go.exe trouvés. Vous devrez peut-être les arrêter manuellement:")
					for i, line := range lines {
						if i == 0 || line == "" { // Ignorer l'en-tête et les lignes vides
							continue
						}
						fmt.Println(line)
					}
				}
			}
		}
	} else {
		// Pour les systèmes Unix
		cmd := exec.Command("pkill", "-f", "bot-spot")
		if err := cmd.Run(); err == nil {
			fmt.Println("Processus planificateur arrêté avec succès.")
			pidFound = true
		}
	}

	if pidFound {
		cleanupPlannerFiles()
		fmt.Println("Planificateur arrêté avec succès.")
	} else {
		fmt.Println("Impossible de trouver ou d'arrêter le planificateur.")
		fmt.Println("Vous devrez peut-être l'arrêter manuellement via le Gestionnaire des tâches.")
	}
}

func cleanupPlannerFiles() {
	// Supprimer les fichiers de suivi
	os.Remove("planner.pid")
	os.Remove("planner_exe.info")
}

// checkPlannerStatus vérifie si le planificateur est en cours d'exécution
func checkPlannerStatus() {
	pidData, err := os.ReadFile("planner.pid")
	if err != nil {
		fmt.Println("Statut: Le planificateur n'est pas en cours d'exécution.")
		return
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidData)))
	if err != nil {
		fmt.Printf("Erreur lors de la lecture du PID: %v\n", err)
		return
	}

	// Vérifier si le processus existe toujours (dépend de l'OS)
	exists := false
	if runtime.GOOS == "windows" {
		// Sous Windows, FindProcess retourne toujours un process non-nil,
		// donc on utilise OpenProcess pour vérifier
		h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(pid))
		if err == nil {
			syscall.CloseHandle(h)
			exists = true
		}
	} else {
		// Sous Unix, on peut envoyer un signal 0 pour vérifier
		process, err := os.FindProcess(pid)
		if err == nil {
			err = process.Signal(syscall.Signal(0))
			exists = (err == nil)
		}
	}

	if exists {
		fmt.Printf("Statut: Le planificateur est en cours d'exécution (PID: %d)\n", pid)
	} else {
		fmt.Println("Statut: Le planificateur n'est pas en cours d'exécution (PID périmé).")
		os.Remove("planner.pid") // Nettoyer le fichier PID obsolète
	}
}

// runPlannerDaemon démarre le planificateur en mode daemon
func runPlannerDaemon() {
	// Configurer la journalisation
	logFile, err := os.OpenFile("planner_daemon.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return // En mode daemon, on ne peut pas afficher d'erreur
	}
	log.SetOutput(logFile)
	log.Println("Démarrage du planificateur en mode daemon")

	// Initialiser la configuration et le logger
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("Erreur lors du chargement de la configuration: %v\n", err)
		return
	}

	logger := logger.NewLogger(logger.LogConfig{
		Level:  "info",
		Format: "text",
	})

	// Créer le planificateur
	sched := scheduler.NewScheduler(cfg, logger)

	// Charger les tâches existantes
	err = sched.LoadTasksFromConfig()
	if err != nil {
		log.Printf("Erreur lors du chargement des tâches: %v\n", err)
	}

	// Démarrer le planificateur
	sched.Start()
	log.Println("Planificateur démarré avec succès")

	// Créer un canal pour capturer les signaux d'interruption
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Attendre le signal d'interruption
	<-sigChan

	// Arrêter le planificateur
	sched.Stop()
	log.Println("Planificateur arrêté")
}

// Convertit une durée en valeur et unité lisibles par l'utilisateur
func durationToUserFriendly(d time.Duration) (int, types.TimeUnit) {
	minutes := int(d.Minutes())

	if minutes < 60 {
		return minutes, types.Minutes
	}

	hours := int(d.Hours())
	if hours < 24 {
		return hours, types.Hours
	}

	days := int(hours / 24)
	return days, types.Days
}

// Convertit une valeur et une unité en chaîne lisible
func formatIntervalToString(value int, unit types.TimeUnit) string {
	switch unit {
	case types.Minutes:
		if value == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", value)
	case types.Hours:
		if value == 1 {
			return "1 heure"
		}
		return fmt.Sprintf("%d heures", value)
	case types.Days:
		if value == 1 {
			return "1 jour"
		}
		return fmt.Sprintf("%d jours", value)
	default:
		return fmt.Sprintf("%d %s", value, unit)
	}
}

func addNewTaskInteractive(sched *scheduler.Scheduler, reader *bufio.Reader) {
	// 1. Définir le type de tâche
	fmt.Println("\n=== Configuration d'une nouvelle tâche ===")
	fmt.Println("Types de tâches disponibles:")
	fmt.Println("1. Mise à jour des cycles (update)")
	fmt.Println("2. Création d'un nouveau cycle (new)")
	fmt.Print("Choisissez le type de tâche (1 ou 2): ")

	typeChoice, _ := reader.ReadString('\n')
	typeChoice = strings.TrimSpace(typeChoice)

	var taskType string
	switch typeChoice {
	case "1":
		taskType = "update"
	case "2":
		taskType = "new"
	default:
		fmt.Println("Choix invalide. Configuration annulée.")
		return
	}

	// 2. Définir le nom de la tâche
	fmt.Print("\nNom de la tâche: ")
	taskName, _ := reader.ReadString('\n')
	taskName = strings.TrimSpace(taskName)

	if taskName == "" {
		// Utiliser un nom par défaut basé sur le type
		if taskType == "update" {
			taskName = "update-cycles-auto"
		} else {
			taskName = "new-cycle-auto"
		}
	}

	// 3. Définir l'intervalle
	var intervalValue int
	var intervalUnit types.TimeUnit

	fmt.Println("\nDéfinir l'intervalle d'exécution:")
	fmt.Println("1. Minutes")
	fmt.Println("2. Heures")
	fmt.Println("3. Jours")
	fmt.Print("Choisissez l'unité (1-3): ")

	unitChoice, _ := reader.ReadString('\n')
	unitChoice = strings.TrimSpace(unitChoice)

	switch unitChoice {
	case "1":
		intervalUnit = types.Minutes
		fmt.Print("Intervalle en minutes: ")
	case "2":
		intervalUnit = types.Hours
		fmt.Print("Intervalle en heures: ")
	case "3":
		intervalUnit = types.Days
		fmt.Print("Intervalle en jours: ")
	default:
		fmt.Println("Unité invalide, utilisation des minutes par défaut.")
		intervalUnit = types.Minutes
		fmt.Print("Intervalle en minutes: ")
	}

	intervalStr, _ := reader.ReadString('\n')
	intervalStr = strings.TrimSpace(intervalStr)

	if val, err := strconv.Atoi(intervalStr); err == nil {
		intervalValue = val
	} else {
		fmt.Println("Valeur invalide, utilisation de 5 par défaut.")
		intervalValue = 5
	}

	// 4. Définir une heure spécifique (optionnel)
	var specificTime string
	if intervalUnit == types.Days {
		fmt.Print("\nVoulez-vous définir une heure spécifique pour l'exécution? (o/n): ")
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))

		if response == "o" || response == "oui" || response == "y" || response == "yes" {
			fmt.Print("Entrez l'heure au format HH:MM (ex: 09:30): ")
			specificTime, _ = reader.ReadString('\n')
			specificTime = strings.TrimSpace(specificTime)

			// Valider le format de l'heure
			matched, _ := regexp.MatchString(`^([01]?[0-9]|2[0-3]):[0-5][0-9]$`, specificTime)
			if !matched {
				fmt.Println("Format d'heure invalide, aucune heure spécifique ne sera définie.")
				specificTime = ""
			}
		}
	}

	// 5. Choisir l'exchange et les paramètres personnalisés
	var exchangeName string
	var buyOffset, sellOffset, percent float64

	fmt.Print("\nSpécifier un exchange particulier? (o/n): ")
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))

	if response == "o" || response == "oui" || response == "y" || response == "yes" {
		fmt.Println("\nExchanges disponibles:")
		fmt.Println("1. BINANCE")
		fmt.Println("2. MEXC")
		fmt.Println("3. KUCOIN")
		fmt.Println("4. KRAKEN")
		fmt.Print("Choisissez un exchange (1-4): ")

		exchangeChoice, _ := reader.ReadString('\n')
		exchangeChoice = strings.TrimSpace(exchangeChoice)

		switch exchangeChoice {
		case "1":
			exchangeName = "BINANCE"
		case "2":
			exchangeName = "MEXC"
		case "3":
			exchangeName = "KUCOIN"
		case "4":
			exchangeName = "KRAKEN"
		default:
			fmt.Println("Choix invalide, aucun exchange spécifique ne sera défini.")
			exchangeName = ""
		}

		// Si un exchange est spécifié et que le type est "new", proposer de personnaliser les paramètres
		if exchangeName != "" && taskType == "new" {
			fmt.Print("\nVoulez-vous personnaliser les paramètres de trading (BUY_OFFSET, SELL_OFFSET, PERCENT)? (o/n): ")
			response, _ := reader.ReadString('\n')
			response = strings.TrimSpace(strings.ToLower(response))

			if response == "o" || response == "oui" || response == "y" || response == "yes" {
				// BUY_OFFSET
				fmt.Print("BUY_OFFSET (laissez vide pour utiliser la valeur par défaut): ")
				buyOffsetStr, _ := reader.ReadString('\n')
				buyOffsetStr = strings.TrimSpace(buyOffsetStr)

				if buyOffsetStr != "" {
					if val, err := strconv.ParseFloat(buyOffsetStr, 64); err == nil {
						buyOffset = val
					} else {
						fmt.Println("Valeur invalide, utilisation de la valeur par défaut.")
					}
				}

				// SELL_OFFSET
				fmt.Print("SELL_OFFSET (laissez vide pour utiliser la valeur par défaut): ")
				sellOffsetStr, _ := reader.ReadString('\n')
				sellOffsetStr = strings.TrimSpace(sellOffsetStr)

				if sellOffsetStr != "" {
					if val, err := strconv.ParseFloat(sellOffsetStr, 64); err == nil {
						sellOffset = val
					} else {
						fmt.Println("Valeur invalide, utilisation de la valeur par défaut.")
					}
				}

				// PERCENT
				fmt.Print("PERCENT (laissez vide pour utiliser la valeur par défaut): ")
				percentStr, _ := reader.ReadString('\n')
				percentStr = strings.TrimSpace(percentStr)

				if percentStr != "" {
					if val, err := strconv.ParseFloat(percentStr, 64); err == nil {
						percent = val
					} else {
						fmt.Println("Valeur invalide, utilisation de la valeur par défaut.")
					}
				}
			}
		}
	}

	// Créer la configuration de la tâche
	// Convertir types.TimeUnit vers scheduler.TimeUnit
	var schedIntervalUnit types.TimeUnit
	switch intervalUnit {
	case types.Minutes:
		schedIntervalUnit = scheduler.Minutes
	case types.Hours:
		schedIntervalUnit = scheduler.Hours
	case types.Days:
		schedIntervalUnit = scheduler.Days
	}

	taskConfig := types.TaskConfig{
		Name:          taskName,
		Type:          taskType,
		IntervalValue: intervalValue,
		IntervalUnit:  schedIntervalUnit,
		SpecificTime:  specificTime,
		Exchange:      exchangeName,
		BuyOffset:     buyOffset,
		SellOffset:    sellOffset,
		Percent:       percent,
		Enabled:       true,
	}

	// Créer la fonction appropriée pour la tâche
	var taskFn func(ctx context.Context, config types.TaskConfig) error
	switch taskConfig.Type {
	case "update":
		taskFn = sched.CreateUpdateTask()
	case "new":
		taskFn = sched.CreateNewCycleTask()
	}

	// Ajouter la tâche
	sched.AddTask(taskConfig, taskFn)

	// Sauvegarder la tâche dans la configuration (persistance)
	err := sched.SaveTasksToConfig()
	if err != nil {
		fmt.Printf("Erreur lors de la sauvegarde de la tâche: %v\n", err)
	}

	fmt.Printf("\nTâche '%s' ajoutée avec succès.\n", taskConfig.Name)
	fmt.Printf("La tâche sera exécutée tous les %s.\n",
		formatIntervalToString(taskConfig.IntervalValue, intervalUnit))

	if taskConfig.SpecificTime != "" {
		fmt.Printf("Exécution à %s tous les jours.\n", taskConfig.SpecificTime)
	}

	// Afficher un résumé des paramètres personnalisés si définis
	if taskConfig.Type == "new" {
		fmt.Println("\nParamètres de trading définis:")
		if taskConfig.BuyOffset != 0 {
			fmt.Printf("- BuyOffset: %.2f\n", taskConfig.BuyOffset)
		}
		if taskConfig.SellOffset != 0 {
			fmt.Printf("- SellOffset: %.2f\n", taskConfig.SellOffset)
		}
		if taskConfig.Percent != 0 {
			fmt.Printf("- Pourcentage USDC: %.2f%%\n", taskConfig.Percent)
		}
	}
}

// removeTaskCmd gère la commande pour supprimer une tâche planifiée
func removeTaskCmd() {
	fmt.Println("=== Suppression d'une tâche planifiée ===")

	// Initialiser la configuration et le logger
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("Erreur lors du chargement de la configuration: %v\n", err)
		return
	}

	log := logger.NewLogger(logger.LogConfig{
		Level:  "info",
		Format: "text",
	})

	// Créer le planificateur
	sched := scheduler.NewScheduler(cfg, log)

	// Charger les tâches existantes
	err = sched.LoadTasksFromConfig()
	if err != nil {
		fmt.Printf("Erreur lors du chargement des tâches: %v\n", err)
	}

	// Afficher les tâches existantes
	tasks := sched.GetAllTasks()

	if len(tasks) == 0 {
		fmt.Println("Aucune tâche planifiée n'est configurée actuellement.")
		return
	}

	fmt.Println("\nTâches planifiées existantes:")
	for i, task := range tasks {
		statusStr := "Activée"
		if !task.Enabled {
			statusStr = "Désactivée"
		}

		// Afficher plus de détails sur l'intervalle
		intervalStr := ""
		if task.IntervalValue > 0 {
			// Utilisez une fonction auxiliaire pour formater l'intervalle
			intervalStr = formatIntervalToString(task.IntervalValue, task.IntervalUnit)
		}

		fmt.Printf("%d. %s - %s - Intervalle: %s - État: %s\n",
			i+1,
			task.Name,
			task.Type,
			intervalStr,
			statusStr)

		// Ajouter des détails supplémentaires selon le type
		if task.Type == "new" && task.Exchange != "" {
			fmt.Printf("   Exchange: %s", task.Exchange)

			// Afficher les paramètres personnalisés
			customParams := []string{}
			if task.BuyOffset != 0 {
				customParams = append(customParams, fmt.Sprintf("BuyOffset: %.2f", task.BuyOffset))
			}
			if task.SellOffset != 0 {
				customParams = append(customParams, fmt.Sprintf("SellOffset: %.2f", task.SellOffset))
			}
			if task.Percent != 0 {
				customParams = append(customParams, fmt.Sprintf("Percent: %.2f", task.Percent))
			}

			if len(customParams) > 0 {
				fmt.Printf(" (%s)", strings.Join(customParams, ", "))
			}
			fmt.Println()
		}

		if task.SpecificTime != "" {
			fmt.Printf("   Heure d'exécution: %s\n", task.SpecificTime)
		}
	}

	// Demander à l'utilisateur quelle tâche supprimer
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("\nEntrez le numéro de la tâche à supprimer (ou 0 pour annuler): ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	taskNum, err := strconv.Atoi(input)
	if err != nil || taskNum < 0 || taskNum > len(tasks) {
		fmt.Println("Numéro de tâche invalide.")
		return
	}

	if taskNum == 0 {
		fmt.Println("Opération annulée.")
		return
	}

	// Supprimer la tâche sélectionnée
	taskToRemove := tasks[taskNum-1]

	// Demander confirmation
	fmt.Printf("\nÊtes-vous sûr de vouloir supprimer la tâche '%s' ? (o/n): ", taskToRemove.Name)
	confirm, _ := reader.ReadString('\n')
	confirm = strings.TrimSpace(strings.ToLower(confirm))

	if confirm != "o" && confirm != "oui" && confirm != "y" && confirm != "yes" {
		fmt.Println("Suppression annulée.")
		return
	}

	err = sched.RemoveTask(taskToRemove.Name)
	if err != nil {
		fmt.Printf("Erreur lors de la suppression de la tâche: %v\n", err)
	} else {
		fmt.Printf("Tâche '%s' supprimée avec succès.\n", taskToRemove.Name)
	}
}

// removeAllTasksCmd supprime toutes les tâches planifiées
func removeAllTasksCmd() {
	fmt.Println("=== Suppression de toutes les tâches planifiées ===")

	// Initialiser la configuration et le logger
	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Printf("Erreur lors du chargement de la configuration: %v\n", err)
		return
	}

	log := logger.NewLogger(logger.LogConfig{
		Level:  "info",
		Format: "text",
	})

	// Créer le planificateur
	sched := scheduler.NewScheduler(cfg, log)

	// Charger les tâches existantes
	err = sched.LoadTasksFromConfig()
	if err != nil {
		fmt.Printf("Erreur lors du chargement des tâches: %v\n", err)
	}

	// Afficher les tâches existantes
	tasks := sched.GetAllTasks()

	if len(tasks) == 0 {
		fmt.Println("Aucune tâche planifiée n'est configurée actuellement.")
		return
	}

	fmt.Println("\nTâches planifiées qui seront supprimées:")
	for i, task := range tasks {
		fmt.Printf("%d. %s - %s\n", i+1, task.Name, task.Type)
	}

	// Demander confirmation
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("\nVous êtes sur le point de supprimer toutes les tâches planifiées (%d). Êtes-vous sûr ? (o/n): ", len(tasks))
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input != "o" && input != "oui" && input != "y" && input != "yes" {
		fmt.Println("Opération annulée.")
		return
	}

	// Supprimer toutes les tâches
	taskNames := make([]string, len(tasks))
	for i, task := range tasks {
		taskNames[i] = task.Name
	}

	for _, name := range taskNames {
		err = sched.RemoveTask(name)
		if err != nil {
			fmt.Printf("Erreur lors de la suppression de la tâche '%s': %v\n", name, err)
		}
	}

	fmt.Println("Toutes les tâches planifiées ont été supprimées.")

	// Vérifier que le fichier tasks.conf est vide ou a bien la valeur TASKS_COUNT=0
	tasksConfigFile := "tasks.conf"
	content := "# Configuration des tâches planifiées\n# Format: TASK_[index]_[property]=[value]\n\nTASKS_COUNT=0\n"
	err = os.WriteFile(tasksConfigFile, []byte(content), 0644)
	if err != nil {
		fmt.Printf("Erreur lors de la mise à jour du fichier de configuration: %v\n", err)
	}
}

// startSchedulerInteractive démarre le planificateur et attend l'interruption de l'utilisateur
/*func startSchedulerInteractive(sched *scheduler.Scheduler) {
	fmt.Println("\nDémarrage du planificateur de tâches...")
	fmt.Println("Le planificateur s'exécutera en arrière-plan.")
	fmt.Println("Les commandes seront exécutées aux intervalles configurés.")
	fmt.Println("Appuyez sur Ctrl+C pour arrêter le planificateur.")

	// Afficher les tâches qui vont être exécutées
	tasks := sched.GetAllTasks()
	enabledTasks := 0

	fmt.Println("\nTâches planifiées qui seront exécutées:")
	for _, task := range tasks {
		if task.Enabled {
			enabledTasks++
			nextRun := "inconnue"
			if !task.NextScheduledAt.IsZero() {
				nextRun = task.NextScheduledAt.Format("02/01/2006 15:04:05")
			}
			var typesIntervalUnit types.TimeUnit
			switch task.IntervalUnit {
			case scheduler.Minutes:
				typesIntervalUnit = types.Minutes
			case scheduler.Hours:
				typesIntervalUnit = types.Hours
			case scheduler.Days:
				typesIntervalUnit = types.Days
			}
			fmt.Printf("- %s (%s) - Prochaine exécution: %s\n",
				task.Name,
				formatIntervalToString(task.IntervalValue, typesIntervalUnit),
				nextRun)
		}
	}

	if enabledTasks == 0 {
		fmt.Println("Aucune tâche active. Utilisez 'go run . -plan' pour configurer des tâches.")
		return
	}

	// Démarrer le planificateur
	sched.Start()

	// Créer un canal pour capturer les signaux d'interruption
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Attendre le signal d'interruption
	<-sigChan

	fmt.Println("\nArrêt du planificateur...")
	sched.Stop()
	fmt.Println("Planificateur arrêté.")
}*/
