// internal/scheduler/scheduler.go
package scheduler

import (
	"context"
	"fmt"
	"main/internal/config"
	"main/internal/types"
	"main/pkg/logger"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Ces constantes sont nécessaires pour la compatibilité avec le code existant,
// mais nous utiliserons types.TimeUnit pour la définition réelle
const (
	Minutes = "minutes"
	Hours   = "hours"
	Days    = "days"
)

// Sémaphore pour limiter l'accès à la base de données
var dbSemaphore = make(chan struct{}, 1)

// Task représente une tâche planifiée en cours d'exécution
type Task struct {
	Config types.TaskConfig
	Fn     func(ctx context.Context, config types.TaskConfig) error
}

// Scheduler gère l'exécution des tâches planifiées
type Scheduler struct {
	tasks     []*Task
	logger    *logger.Logger
	config    *config.Config
	isRunning bool
	mu        sync.Mutex
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewScheduler crée un nouveau planificateur
func NewScheduler(config *config.Config, logger *logger.Logger) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		tasks:     make([]*Task, 0),
		logger:    logger,
		config:    config,
		isRunning: false,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// AddTask ajoute une nouvelle tâche au planificateur
func (s *Scheduler) AddTask(config types.TaskConfig, fn func(ctx context.Context, config types.TaskConfig) error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Définir l'intervalle basé sur l'unité si pas déjà défini
	if config.Interval == 0 && config.IntervalValue > 0 {
		switch config.IntervalUnit {
		case types.Minutes:
			config.Interval = time.Duration(config.IntervalValue) * time.Minute
		case types.Hours:
			config.Interval = time.Duration(config.IntervalValue) * time.Hour
		case types.Days:
			config.Interval = time.Duration(config.IntervalValue) * 24 * time.Hour
		}
	}

	task := &Task{
		Config: config,
		Fn:     fn,
	}

	// Calculer la prochaine exécution prévue
	task.Config.NextScheduledAt = s.calculateNextRun(config)

	s.tasks = append(s.tasks, task)
	s.logger.Info("Tâche ajoutée: %s (intervalle: %v %s, prochaine exécution: %s)",
		config.Name,
		config.IntervalValue,
		config.IntervalUnit,
		task.Config.NextScheduledAt.Format("2006-01-02 15:04:05"))
}

// DurationToUserFriendly convertit une durée en valeur et unité lisibles par l'utilisateur
func DurationToUserFriendly(d time.Duration) (int, types.TimeUnit) {
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

// calculateNextRun calcule la prochaine exécution d'une tâche
func (s *Scheduler) calculateNextRun(config types.TaskConfig) time.Time {
	now := time.Now()

	// Si une heure spécifique est définie
	if config.SpecificTime != "" {
		targetTime, err := time.Parse("15:04", config.SpecificTime)
		if err == nil {
			targetToday := time.Date(
				now.Year(), now.Month(), now.Day(),
				targetTime.Hour(), targetTime.Minute(), 0, 0, now.Location(),
			)

			// Si l'heure est déjà passée aujourd'hui, planifier pour demain
			if targetToday.Before(now) {
				return targetToday.Add(24 * time.Hour)
			}
			return targetToday
		}
	}

	// Si une prochaine exécution est déjà prévue et est dans le futur, la conserver
	if !config.NextScheduledAt.IsZero() && config.NextScheduledAt.After(now) {
		return config.NextScheduledAt
	}

	// Calculer la prochaine exécution basée sur l'intervalle
	interval := config.Interval
	if interval == 0 && config.IntervalValue > 0 {
		switch config.IntervalUnit {
		case types.Minutes:
			interval = time.Duration(config.IntervalValue) * time.Minute
		case types.Hours:
			interval = time.Duration(config.IntervalValue) * time.Hour
		case types.Days:
			interval = time.Duration(config.IntervalValue) * 24 * time.Hour
		}
	}

	// Si la dernière exécution est définie, calculer à partir de là
	if !config.LastRunTime.IsZero() {
		return config.LastRunTime.Add(interval)
	}

	// Sinon, ajouter l'intervalle à maintenant
	return now.Add(interval)
}

// Start démarre le planificateur
func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.isRunning {
		s.mu.Unlock()
		return
	}
	s.isRunning = true
	s.mu.Unlock()

	s.logger.Info("Démarrage du planificateur de tâches")

	go s.runScheduler()
}

// runScheduler est la boucle principale du planificateur
func (s *Scheduler) runScheduler() {
	ticker := time.NewTicker(1 * time.Minute) // Vérifier toutes les minutes
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkAndRunTasks()
		case <-s.ctx.Done():
			s.logger.Info("Arrêt du planificateur de tâches")
			return
		}
	}
}

// checkAndRunTasks vérifie et exécute les tâches dont l'heure est venue
func (s *Scheduler) checkAndRunTasks() {
	now := time.Now()

	s.mu.Lock()
	tasksToRun := make([]*Task, 0)
	for _, task := range s.tasks {
		if task.Config.Enabled && now.After(task.Config.NextScheduledAt) {
			tasksToRun = append(tasksToRun, task)
			// Mettre à jour la prochaine exécution
			task.Config.LastRunTime = now
			task.Config.NextScheduledAt = s.calculateNextRun(task.Config)

			// Log de la prochaine exécution
			interval := ""
			if task.Config.IntervalValue > 0 {
				interval = fmt.Sprintf("%d %s", task.Config.IntervalValue, task.Config.IntervalUnit)
			} else {
				value, unit := DurationToUserFriendly(task.Config.Interval)
				interval = fmt.Sprintf("%d %s", value, unit)
			}

			s.logger.Info("Tâche %s planifiée pour la prochaine exécution: %s (intervalle: %s)",
				task.Config.Name,
				task.Config.NextScheduledAt.Format("2006-01-02 15:04:05"),
				interval)
		}
	}
	s.mu.Unlock()

	// Exécuter les tâches en dehors du verrou, mais séquentiellement avec un délai
	// pour les tâches qui accèdent à la base de données
	for i, task := range tasksToRun {
		// On attend un peu entre les tâches pour éviter les conflits de base de données
		if i > 0 {
			time.Sleep(2 * time.Second)
		}
		go s.executeTask(task)
	}
}

// Stop arrête le planificateur
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning {
		return
	}

	s.cancel()
	s.isRunning = false
	s.logger.Info("Arrêt du planificateur de tâches")
}

// executeTask exécute une tâche et gère les erreurs
func (s *Scheduler) executeTask(task *Task) {
	taskCtx, taskCancel := context.WithTimeout(s.ctx, 10*time.Minute) // Timeout de 10 minutes par tâche
	defer taskCancel()

	s.logger.Debug("Exécution de la tâche: %s", task.Config.Name)

	startTime := time.Now()

	// Acquérir le sémaphore pour les opérations de base de données
	if task.Config.Type == "update" || task.Config.Type == "new" {
		s.logger.Debug("Acquisition du verrou de base de données pour la tâche: %s", task.Config.Name)
		select {
		case dbSemaphore <- struct{}{}:
			// Sémaphore acquis
			defer func() { <-dbSemaphore }() // Libérer le sémaphore quand on a fini
		case <-taskCtx.Done():
			// Timeout pendant l'attente du sémaphore
			s.logger.Error("Timeout pendant l'attente du verrou de base de données pour la tâche: %s", task.Config.Name)
			return
		}
	}

	err := task.Fn(taskCtx, task.Config)
	duration := time.Since(startTime)

	if err != nil {
		s.logger.Error("Erreur lors de l'exécution de la tâche %s: %v (durée: %s)",
			task.Config.Name, err, duration)
	} else {
		s.logger.Info("Tâche %s exécutée avec succès (durée: %s)",
			task.Config.Name, duration)
	}
}

// GetAllTasks retourne toutes les tâches configurées
func (s *Scheduler) GetAllTasks() []types.TaskConfig {
	s.mu.Lock()
	defer s.mu.Unlock()

	tasks := make([]types.TaskConfig, len(s.tasks))
	for i, task := range s.tasks {
		tasks[i] = task.Config
	}
	return tasks
}

// UpdateTask met à jour la configuration d'une tâche
func (s *Scheduler) UpdateTask(name string, newConfig types.TaskConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, task := range s.tasks {
		if task.Config.Name == name {
			// Conserver certaines valeurs de l'ancienne configuration
			lastRun := task.Config.LastRunTime
			newConfig.LastRunTime = lastRun

			// Recalculer l'intervalle si nécessaire
			if newConfig.IntervalValue > 0 {
				switch newConfig.IntervalUnit {
				case types.Minutes:
					newConfig.Interval = time.Duration(newConfig.IntervalValue) * time.Minute
				case types.Hours:
					newConfig.Interval = time.Duration(newConfig.IntervalValue) * time.Hour
				case types.Days:
					newConfig.Interval = time.Duration(newConfig.IntervalValue) * 24 * time.Hour
				}
			}

			newConfig.NextScheduledAt = s.calculateNextRun(newConfig)

			s.tasks[i].Config = newConfig
			return nil
		}
	}

	return fmt.Errorf("tâche non trouvée: %s", name)
}

// RemoveTask supprime une tâche du planificateur
func (s *Scheduler) RemoveTask(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, task := range s.tasks {
		if task.Config.Name == name {
			// Supprimer la tâche de la liste
			s.tasks = append(s.tasks[:i], s.tasks[i+1:]...)

			// Mettre à jour le fichier de configuration
			err := s.SaveTasksToConfig()
			if err != nil {
				return fmt.Errorf("erreur lors de la suppression de la tâche: %w", err)
			}

			return nil
		}
	}

	return fmt.Errorf("tâche non trouvée: %s", name)
}

// LoadTasksFromConfig charge les tâches définies dans le fichier de configuration
func (s *Scheduler) LoadTasksFromConfig() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Réinitialiser les tâches existantes
	s.tasks = make([]*Task, 0)

	// Charger les tâches depuis la configuration
	scheduledTasks := s.config.GetScheduledTasks()
	for _, taskConfig := range scheduledTasks {
		// Créer la fonction appropriée en fonction du type de tâche
		var taskFn func(ctx context.Context, config types.TaskConfig) error

		switch taskConfig.Type {
		case "update":
			taskFn = s.createUpdateTask()
		case "new":
			taskFn = s.createNewCycleTask()
		default:
			continue // Ignorer les types de tâches inconnus
		}

		// Ajouter la tâche au planificateur
		task := &Task{
			Config: taskConfig,
			Fn:     taskFn,
		}

		if task.Config.NextScheduledAt.IsZero() || task.Config.NextScheduledAt.Before(time.Now()) {
			task.Config.NextScheduledAt = s.calculateNextRun(taskConfig)
		}

		s.tasks = append(s.tasks, task)

	}

	return nil
}

// createUpdateTask crée une fonction pour la tâche de mise à jour des cycles
func (s *Scheduler) createUpdateTask() func(ctx context.Context, config types.TaskConfig) error {
	return func(ctx context.Context, config types.TaskConfig) error {
		var args []string

		// Détecter dynamiquement le chemin du projet
		projectDir, err := findProjectRoot()
		if err != nil {
			s.logger.Error("Impossible de trouver le répertoire du projet: %v", err)
			return err
		}

		// Ajouter l'option pour l'exchange spécifique si nécessaire
		if config.Exchange != "" {
			args = append(args, fmt.Sprintf("-exchange%s", strings.ToLower(config.Exchange)))
		}

		// Ajouter la commande de mise à jour
		args = append(args, "-u")

		// Exécuter la commande avec go run
		cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
		cmd.Dir = projectDir

		// Ajouter un timeout à la commande
		var cmdCtx context.Context
		var cmdCancel context.CancelFunc
		cmdCtx, cmdCancel = context.WithTimeout(ctx, 2*time.Minute)
		defer cmdCancel()
		cmd = exec.CommandContext(cmdCtx, cmd.Path, cmd.Args[1:]...)
		cmd.Dir = projectDir

		output, err := cmd.CombinedOutput()

		if err != nil {
			s.logger.Error("Erreur lors de l'exécution de la commande update: %v, output: %s", err, string(output))
			return err
		}

		s.logger.Info("Commande update exécutée avec succès: %s", string(output))
		return nil
	}
}

// createNewCycleTask crée une fonction pour la tâche de création de nouveaux cycles
func (s *Scheduler) createNewCycleTask() func(ctx context.Context, config types.TaskConfig) error {
	return func(ctx context.Context, config types.TaskConfig) error {
		var args []string
		var tempEnvVars []string

		// Détecter dynamiquement le chemin du projet
		projectDir, err := findProjectRoot()
		if err != nil {
			s.logger.Error("Impossible de trouver le répertoire du projet: %v", err)
			return err
		}

		// Ajouter l'option pour l'exchange spécifique si nécessaire
		if config.Exchange != "" {
			args = append(args, fmt.Sprintf("-exchange%s", strings.ToLower(config.Exchange)))

			// Si des paramètres personnalisés sont définis, les configurer temporairement via des variables d'environnement
			if config.BuyOffset != 0 || config.SellOffset != 0 || config.Percent != 0 {
				exchangeUpper := strings.ToUpper(config.Exchange)

				if config.BuyOffset != 0 {
					buyOffsetEnv := fmt.Sprintf("%s_BUY_OFFSET=%g", exchangeUpper, config.BuyOffset)
					tempEnvVars = append(tempEnvVars, buyOffsetEnv)
				}

				if config.SellOffset != 0 {
					sellOffsetEnv := fmt.Sprintf("%s_SELL_OFFSET=%g", exchangeUpper, config.SellOffset)
					tempEnvVars = append(tempEnvVars, sellOffsetEnv)
				}

				if config.Percent != 0 {
					percentEnv := fmt.Sprintf("%s_PERCENT=%g", exchangeUpper, config.Percent)
					tempEnvVars = append(tempEnvVars, percentEnv)
				}
			}
		}

		// Ajouter la commande de création de cycle
		args = append(args, "-n")

		// Préparer la commande
		cmd := exec.Command("go", append([]string{"run", "."}, args...)...)
		cmd.Dir = projectDir

		// Ajouter un timeout à la commande
		var cmdCtx context.Context
		var cmdCancel context.CancelFunc
		cmdCtx, cmdCancel = context.WithTimeout(ctx, 2*time.Minute)
		defer cmdCancel()
		cmd = exec.CommandContext(cmdCtx, cmd.Path, cmd.Args[1:]...)
		cmd.Dir = projectDir

		// Ajouter les variables d'environnement si nécessaire
		if len(tempEnvVars) > 0 {
			s.logger.Info("Paramètres personnalisés pour la tâche: %s", strings.Join(tempEnvVars, ", "))

			// Récupérer l'environnement actuel et ajouter les variables temporaires
			currentEnv := os.Environ()
			cmd.Env = append(currentEnv, tempEnvVars...)
		}

		// Exécuter la commande
		output, err := cmd.CombinedOutput()

		if err != nil {
			s.logger.Error("Erreur lors de l'exécution de la commande new-cycle: %v, output: %s", err, string(output))
			return err
		}

		s.logger.Info("Commande new-cycle exécutée avec succès: %s", string(output))
		return nil
	}
}

// CreateUpdateTask crée une fonction pour la tâche de mise à jour des cycles
func (s *Scheduler) CreateUpdateTask() func(ctx context.Context, config types.TaskConfig) error {
	return s.createUpdateTask()
}

func findProjectRoot() (string, error) {
	// Répertoire de travail actuel
	currentDir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// Parcourir les répertoires parents à la recherche du fichier go.mod
	dir := currentDir
	for {
		// Vérifier si go.mod existe dans ce répertoire
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			// Vérifier s'il y a des fichiers Go dans ce répertoire
			files, err := filepath.Glob(filepath.Join(dir, "*.go"))
			if err != nil || len(files) == 0 {
				// S'il n'y a pas de fichiers Go, essayer le sous-répertoire cmd/bot-spot
				cmdBotSpotPath := filepath.Join(dir, "cmd", "bot-spot")
				if _, err := os.Stat(filepath.Join(cmdBotSpotPath, "main.go")); err == nil {
					return cmdBotSpotPath, nil
				}
			}
			return dir, nil
		}

		// Monter d'un niveau dans l'arborescence
		parentDir := filepath.Dir(dir)

		// Si on est arrivé à la racine du système de fichiers sans trouver go.mod
		if parentDir == dir {
			// Dernier recours : vérifier le chemin spécifique
			cmdBotSpotPath := filepath.Join(currentDir, "cmd", "bot-spot")
			if _, err := os.Stat(filepath.Join(cmdBotSpotPath, "main.go")); err == nil {
				return cmdBotSpotPath, nil
			}
			return "", fmt.Errorf("fichier go.mod non trouvé")
		}

		dir = parentDir
	}
}

// CreateNewCycleTask crée une fonction pour la tâche de création de nouveaux cycles
func (s *Scheduler) CreateNewCycleTask() func(ctx context.Context, config types.TaskConfig) error {
	return s.createNewCycleTask()
}

// CreateDefaultTasks crée les tâches par défaut pour le bot
func (s *Scheduler) CreateDefaultTasks() {
	// Mise à jour des cycles toutes les 5 minutes
	s.AddTask(types.TaskConfig{
		Name:          "update-cycles",
		Type:          "update",
		IntervalValue: 5,
		IntervalUnit:  types.Minutes,
		Enabled:       true,
	}, s.createUpdateTask())

	// Création d'un nouveau cycle tous les jours à 9h00
	s.AddTask(types.TaskConfig{
		Name:          "create-cycle",
		Type:          "new",
		IntervalValue: 24,
		IntervalUnit:  types.Hours,
		SpecificTime:  "09:00",
		Enabled:       true,
	}, s.createNewCycleTask())
}

// ParseInterval convertit une chaîne d'intervalle (ex: "5m", "2h", "1d") en valeur et unité
func ParseInterval(intervalStr string) (int, types.TimeUnit, error) {
	if intervalStr == "" {
		return 0, "", fmt.Errorf("intervalle vide")
	}

	// Extraire la valeur numérique et l'unité
	var valueStr string
	var unitStr string

	for i, char := range intervalStr {
		if char < '0' || char > '9' {
			valueStr = intervalStr[:i]
			unitStr = intervalStr[i:]
			break
		}

		// Si on atteint la fin de la chaîne, tout est une valeur
		if i == len(intervalStr)-1 {
			valueStr = intervalStr
		}
	}

	if valueStr == "" {
		return 0, "", fmt.Errorf("aucune valeur numérique trouvée dans l'intervalle")
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return 0, "", fmt.Errorf("valeur d'intervalle invalide: %v", err)
	}

	// Convertir l'unité en TimeUnit
	switch strings.ToLower(strings.TrimSpace(unitStr)) {
	case "m", "min", "minute", "minutes":
		return value, types.Minutes, nil
	case "h", "hour", "hours", "heure", "heures":
		return value, types.Hours, nil
	case "d", "day", "days", "jour", "jours":
		return value, types.Days, nil
	default:
		return 0, "", fmt.Errorf("unité d'intervalle non reconnue: %s", unitStr)
	}
}

// FormatIntervalToString convertit une valeur et une unité en chaîne lisible
func FormatIntervalToString(value int, unit types.TimeUnit) string {
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

// SaveTasksToConfig sauvegarde les tâches dans la configuration
func (s *Scheduler) SaveTasksToConfig() error {
	// Chemin du fichier de configuration des tâches
	tasksConfigFile := "tasks.conf"

	// Préparer le contenu du fichier
	var lines []string
	lines = append(lines, "# Configuration des tâches planifiées")
	lines = append(lines, "# Format: TASK_[index]_[property]=[value]")
	lines = append(lines, fmt.Sprintf("TASKS_COUNT=%d", len(s.tasks)))

	// Écrire chaque tâche
	for i, task := range s.tasks {
		prefix := fmt.Sprintf("TASK_%d_", i+1)

		// Propriétés de base
		lines = append(lines, prefix+"NAME="+task.Config.Name)
		lines = append(lines, prefix+"TYPE="+task.Config.Type)
		lines = append(lines, prefix+"ENABLED="+strconv.FormatBool(task.Config.Enabled))
		lines = append(lines, prefix+"INTERVAL_VALUE="+strconv.Itoa(task.Config.IntervalValue))
		lines = append(lines, prefix+"INTERVAL_UNIT="+string(task.Config.IntervalUnit))

		// Ajouter l'heure spécifique si définie
		if task.Config.SpecificTime != "" {
			lines = append(lines, prefix+"SPECIFIC_TIME="+task.Config.SpecificTime)
		}

		// Ajouter l'exchange si défini
		if task.Config.Exchange != "" {
			lines = append(lines, prefix+"EXCHANGE="+task.Config.Exchange)
		}

		// Paramètres spécifiques aux tâches de type "new"
		if task.Config.Type == "new" {
			if task.Config.BuyOffset != 0 {
				lines = append(lines, prefix+"BUY_OFFSET="+strconv.FormatFloat(task.Config.BuyOffset, 'f', -1, 64))
			}
			if task.Config.SellOffset != 0 {
				lines = append(lines, prefix+"SELL_OFFSET="+strconv.FormatFloat(task.Config.SellOffset, 'f', -1, 64))
			}
			if task.Config.Percent != 0 {
				lines = append(lines, prefix+"PERCENT="+strconv.FormatFloat(task.Config.Percent, 'f', -1, 64))
			}
		}

		if !task.Config.NextScheduledAt.IsZero() {
			lines = append(lines, prefix+"NEXT_SCHEDULED_AT="+task.Config.NextScheduledAt.Format(time.RFC3339))
		}
	}

	// Écrire le contenu dans le fichier
	content := strings.Join(lines, "\n") + "\n"
	err := os.WriteFile(tasksConfigFile, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("erreur lors de la sauvegarde des tâches: %w", err)
	}

	return nil
}
