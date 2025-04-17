// internal/config/config.go
package config

import (
	"errors"
	"fmt"
	"log"
	"main/internal/types"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// ConfigFilename est le nom du fichier de configuration principal
const ConfigFilename = "bot.conf"

type ExchangeConfig struct {
	Name                   string
	APIKey                 string
	SecretKey              string
	BuyOffset              float64
	SellOffset             float64
	Percent                float64
	BuyMaxDays             int
	BuyMaxPriceDeviation   float64
	Accumulation           bool    // Activation de l'accumulation
	SellAccuPriceDeviation float64 // Pourcentage de déviation pour l'accumulation
	AdaptiveOrder          bool    // Activation du calcul adaptatif d'ordres
	MinLockedRatio         float64 // Ratio minimal pour appliquer la formule adaptative
	Enabled                bool
}

// Config contient toutes les configurations de l'application
type Config struct {
	// Informations générales
	MainExchangeName string
	Exchanges        map[string]ExchangeConfig

	// Paramètres globaux par défaut
	DefaultPercent                float64
	DefaultBuyMaxDays             int
	DefaultBuyMaxPriceDeviation   float64
	DefaultAccumulation           bool    // Valeur par défaut pour l'accumulation
	DefaultSellAccuPriceDeviation float64 // Valeur par défaut pour la déviation d'accumulation
	DefaultAdaptiveOrder          bool
	DefaultMinLockedRatio         float64

	// Autres paramètres potentiels
	Environment string
	LogLevel    string
}

// LoadConfig charge la configuration depuis le fichier et l'environnement
func LoadConfig() (*Config, error) {
	// S'assurer que le fichier de configuration existe
	created, err := CreateConfigFileIfNotExists()
	if err != nil {
		return nil, fmt.Errorf("error creating config file: %w", err)
	}

	// Si le fichier vient d'être créé, informer l'utilisateur et sortir sans erreur
	// pour qu'il puisse compléter la configuration
	if created {
		log.Println("Un nouveau fichier de configuration bot.conf a été créé.")
		log.Println("Veuillez éditer ce fichier pour configurer vos clés API avant de continuer.")
		os.Exit(0)
	}

	// Charger le fichier de configuration
	err = godotenv.Load(ConfigFilename)
	if err != nil {
		return nil, fmt.Errorf("error loading config file: %w", err)
	}

	// Exchanges supportés
	supportedExchanges := []string{"BINANCE", "MEXC", "KUCOIN", "KRAKEN"}

	// Créer la configuration des exchanges
	exchangeConfigs := make(map[string]ExchangeConfig)

	// Récupérer les valeurs par défaut globales
	defaultPercent := getEnvFloat("DEFAULT_PERCENT", 5)
	defaultBuyMaxDays := getEnvInt("DEFAULT_BUY_MAX_DAYS", 0)
	defaultBuyMaxPriceDeviation := getEnvFloat("DEFAULT_BUY_MAX_PRICE_DEVIATION", 0)

	// Récupérer les valeurs par défaut pour l'accumulation
	defaultAccumulation := getEnvBool("DEFAULT_ACCUMULATION", false)
	defaultSellAccuPriceDeviation := getEnvFloat("DEFAULT_SELL_ACCU_PRICE_DEVIATION", 10.0)

	// Récupérer les valeurs par défaut pour les ordres adaptatifs
	defaultAdaptiveOrder := getEnvBool("DEFAULT_ADAPTIVE_ORDER", false)
	defaultMinLockedRatio := getEnvFloat("DEFAULT_MIN_LOCKED_RATIO", 0.1)

	for _, ex := range supportedExchanges {
		// Récupérer les paramètres spécifiques à l'exchange, avec repli sur les valeurs par défaut
		exchangeConfigs[ex] = ExchangeConfig{
			Name:       ex,
			APIKey:     getEnvString(fmt.Sprintf("%s_API_KEY", ex), ""),
			SecretKey:  getEnvString(fmt.Sprintf("%s_SECRET_KEY", ex), ""),
			BuyOffset:  getEnvFloat(fmt.Sprintf("%s_BUY_OFFSET", ex), -700),
			SellOffset: getEnvFloat(fmt.Sprintf("%s_SELL_OFFSET", ex), 700),

			// Utiliser les paramètres spécifiques de l'exchange ou les valeurs par défaut
			Percent:    getEnvFloat(fmt.Sprintf("%s_PERCENT", ex), defaultPercent),
			BuyMaxDays: getEnvInt(fmt.Sprintf("%s_BUY_MAX_DAYS", ex), defaultBuyMaxDays),
			BuyMaxPriceDeviation: getEnvFloat(
				fmt.Sprintf("%s_BUY_MAX_PRICE_DEVIATION", ex),
				defaultBuyMaxPriceDeviation,
			),

			// Paramètres d'accumulation
			Accumulation: getEnvBool(
				fmt.Sprintf("%s_ACCUMULATION", ex),
				defaultAccumulation,
			),
			SellAccuPriceDeviation: getEnvFloat(
				fmt.Sprintf("%s_SELL_ACCU_PRICE_DEVIATION", ex),
				defaultSellAccuPriceDeviation,
			),

			// Nouveaux paramètres pour le calcul adaptatif des ordres
			AdaptiveOrder: getEnvBool(
				fmt.Sprintf("%s_ADAPTIVE_ORDER", ex),
				defaultAdaptiveOrder,
			),
			MinLockedRatio: getEnvFloat(
				fmt.Sprintf("%s_MIN_LOCKED_RATIO", ex),
				defaultMinLockedRatio,
			),

			Enabled: getEnvString(fmt.Sprintf("%s_API_KEY", ex), "") != "",
		}
	}

	// Obtenir le nom de l'exchange principal
	mainExchangeName := getEnvString("EXCHANGE", "BINANCE")

	// Créer et valider la configuration
	config := &Config{
		MainExchangeName: strings.ToUpper(mainExchangeName),
		Exchanges:        exchangeConfigs,

		// Stocker les valeurs par défaut globales
		DefaultPercent:                defaultPercent,
		DefaultBuyMaxDays:             defaultBuyMaxDays,
		DefaultBuyMaxPriceDeviation:   defaultBuyMaxPriceDeviation,
		DefaultAccumulation:           defaultAccumulation,
		DefaultSellAccuPriceDeviation: defaultSellAccuPriceDeviation,
		DefaultAdaptiveOrder:          defaultAdaptiveOrder,
		DefaultMinLockedRatio:         defaultMinLockedRatio,

		Environment: getEnvString("ENVIRONMENT", "production"),
		LogLevel:    getEnvString("LOG_LEVEL", "info"),
	}

	// Validation de base
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate vérifie que la configuration est valide
func (c *Config) Validate() error {
	// Validation de l'exchange principal
	c.MainExchangeName = strings.ToUpper(c.MainExchangeName)

	// Vérifier que l'exchange principal est valide et a une configuration
	mainExchangeConfig, exists := c.Exchanges[c.MainExchangeName]
	if !exists {
		log.Printf("Warning: Exchange %s is not supported, using BINANCE\n", c.MainExchangeName)
		c.MainExchangeName = "BINANCE"
		mainExchangeConfig = c.Exchanges["BINANCE"]
	}

	// Validation des clés API de l'exchange principal
	if mainExchangeConfig.APIKey == "" || mainExchangeConfig.SecretKey == "" {
		return fmt.Errorf("%s_API_KEY and %s_SECRET_KEY are required", c.MainExchangeName, c.MainExchangeName)
	}

	// Validation des paramètres de trading pour chaque exchange
	for name, exchange := range c.Exchanges {
		// Vérifier les paramètres de pourcentage
		if exchange.Percent <= 0 || exchange.Percent > 100 {
			return fmt.Errorf("%s_PERCENT must be between 0 and 100", name)
		}

		// Validation des paramètres d'annulation automatique
		if exchange.BuyMaxDays < 0 {
			log.Printf("Warning: %s_BUY_MAX_DAYS cannot be negative, setting to 0 (disabled)\n", name)
			exchange.BuyMaxDays = 0
		}

		if exchange.BuyMaxPriceDeviation < 0 {
			log.Printf("Warning: %s_BUY_MAX_PRICE_DEVIATION cannot be negative, setting to 0 (disabled)\n", name)
			exchange.BuyMaxPriceDeviation = 0
		}

		// Validation des paramètres d'accumulation
		if exchange.SellAccuPriceDeviation < 0 {
			log.Printf("Warning: %s_SELL_ACCU_PRICE_DEVIATION cannot be negative, setting to 10 (default)\n", name)
			exchange.SellAccuPriceDeviation = 10.0
		}

		// Ajuster les offsets
		exchange.BuyOffset = -math.Abs(exchange.BuyOffset)
		exchange.SellOffset = math.Abs(exchange.SellOffset)

		// Mettre à jour la configuration
		c.Exchanges[name] = exchange
	}

	return nil
}

// GetExchangeConfig retourne la configuration d'un exchange spécifique
func (c *Config) GetExchangeConfig(exchangeName string) (ExchangeConfig, error) {
	exchangeName = strings.ToUpper(exchangeName)
	config, exists := c.Exchanges[exchangeName]
	if !exists {
		return ExchangeConfig{}, fmt.Errorf("exchange %s not configured", exchangeName)
	}
	return config, nil
}

// GetMainExchangeConfig retourne la configuration de l'exchange principal
func (c *Config) GetMainExchangeConfig() ExchangeConfig {
	return c.Exchanges[c.MainExchangeName]
}

// Exchange retourne le nom de l'exchange principal
func (c *Config) Exchange() string {
	return c.MainExchangeName
}

// APIKey retourne la clé API de l'exchange principal
func (c *Config) APIKey() string {
	return c.Exchanges[c.MainExchangeName].APIKey
}

// SecretKey retourne la clé secrète de l'exchange principal
func (c *Config) SecretKey() string {
	return c.Exchanges[c.MainExchangeName].SecretKey
}

// BuyOffset retourne l'offset d'achat de l'exchange principal
func (c *Config) BuyOffset() float64 {
	return c.Exchanges[c.MainExchangeName].BuyOffset
}

// SellOffset retourne l'offset de vente de l'exchange principal
func (c *Config) SellOffset() float64 {
	return c.Exchanges[c.MainExchangeName].SellOffset
}

// Percent retourne le pourcentage de trading de l'exchange principal
func (c *Config) Percent() float64 {
	return c.Exchanges[c.MainExchangeName].Percent
}

// BuyMaxDays retourne le nombre max de jours pour un ordre d'achat de l'exchange principal
func (c *Config) BuyMaxDays() int {
	return c.Exchanges[c.MainExchangeName].BuyMaxDays
}

// BuyMaxPriceDeviation retourne la déviation maximale de prix pour l'exchange principal
func (c *Config) BuyMaxPriceDeviation() float64 {
	return c.Exchanges[c.MainExchangeName].BuyMaxPriceDeviation
}

// Accumulation retourne si l'accumulation est activée pour l'exchange principal
func (c *Config) Accumulation() bool {
	return c.Exchanges[c.MainExchangeName].Accumulation
}

// SellAccuPriceDeviation retourne la déviation de prix pour l'accumulation de l'exchange principal
func (c *Config) SellAccuPriceDeviation() float64 {
	return c.Exchanges[c.MainExchangeName].SellAccuPriceDeviation
}

// Fonctions utilitaires (getEnvString, getEnvFloat, getEnvInt, getEnvBool)
func getEnvString(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvFloat(key string, defaultValue float64) float64 {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		log.Printf("Warning: Could not parse %s as float, using default: %f\n", key, defaultValue)
		return defaultValue
	}

	return value
}

func getEnvInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		log.Printf("Warning: Could not parse %s as int, using default: %d\n", key, defaultValue)
		return defaultValue
	}

	return value
}

func getEnvBool(key string, defaultValue bool) bool {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		log.Printf("Warning: Could not parse %s as bool, using default: %v\n", key, defaultValue)
		return defaultValue
	}

	return value
}

// AdaptiveOrder retourne si le calcul adaptatif des ordres est activé pour l'exchange principal
func (c *Config) AdaptiveOrder() bool {
	return c.Exchanges[c.MainExchangeName].AdaptiveOrder
}

// MinLockedRatio retourne le ratio minimal pour l'exchange principal
func (c *Config) MinLockedRatio() float64 {
	return c.Exchanges[c.MainExchangeName].MinLockedRatio
}

// CreateConfigFileIfNotExists crée le fichier de configuration s'il n'existe pas
func CreateConfigFileIfNotExists() (bool, error) {
	if _, err := os.Stat(ConfigFilename); errors.Is(err, os.ErrNotExist) {
		// Vérifier si le fichier d'exemple existe
		exampleFile := "bot.conf.example"
		if _, err := os.Stat(exampleFile); err != nil {
			// Si le fichier exemple n'existe pas, utiliser un template par défaut
			return createConfigFromTemplate()
		}

		// Lire le contenu du fichier exemple
		content, err := os.ReadFile(exampleFile)
		if err != nil {
			return false, fmt.Errorf("failed to read example config file: %w", err)
		}

		// Écrire le contenu dans le nouveau fichier de configuration
		err = os.WriteFile(ConfigFilename, content, 0644)
		if err != nil {
			return false, fmt.Errorf("failed to create config file: %w", err)
		}

		log.Printf("Fichier de configuration créé à partir du modèle: %s", ConfigFilename)
		return true, nil
	}

	return false, nil
}

// createConfigFromTemplate crée un fichier de configuration à partir d'un template intégré
// Cette fonction est utilisée si le fichier bot.conf.example n'existe pas
func createConfigFromTemplate() (bool, error) {
	defaultConfig := `# Configuration de l'exchange principal à utiliser
# Options: BINANCE, MEXC, KUCOIN, KRAKEN
# Actuellement, BINANCE, MEXC, KUCOIN, KRAKEN Entièrement supportés
# Exchange par défaut :
EXCHANGE=BINANCE

# =========== PARAMÈTRES SPÉCIFIQUES PAR EXCHANGE ===========
# FORMAT: EXCHANGE_NAME_[PARAM]

# ----- Binance -----
# Offset d'achat: décalage en $ par rapport au prix actuel du BTC (valeur négative)
BINANCE_BUY_OFFSET=-500

# Offset de vente: décalage en $ par rapport au prix d'achat (valeur positive)
BINANCE_SELL_OFFSET=500

# Pourcentage du capital disponible à utiliser pour chaque cycle (1-100)
BINANCE_PERCENT=4

# Conditions d'annulation automatique des ordres d'achat (reliées par un OU logique):
# - Si l'ordre n'est pas exécuté après X jours (0 = désactivé)
BINANCE_BUY_MAX_DAYS=0

# - Si le prix actuel dépasse de X% le prix d'achat (0 = désactivé)
# Exemple: Pour 10%, le bot annulera l'ordre si le prix monte de 10% par rapport au prix d'achat
BINANCE_BUY_MAX_PRICE_DEVIATION=0

# Paramètres d'accumulation:
# - Activer l'accumulation (true = activé, false = désactivé)
BINANCE_ACCUMULATION=false
# - Pourcentage de déviation pour l'accumulation (déviation minimale entre le prix de vente et le prix actuel)
# Exemple: Pour 10%, le bot annulera l'ordre de vente pour accumuler si le prix actuel baisse de 10% par rapport au prix de vente configuré
BINANCE_SELL_ACCU_PRICE_DEVIATION=10

# Paramètres pour le calcul adaptatif des ordres d'achat:
# - Activer le calcul adaptatif (true = activé, false = désactivé)
BINANCE_ADAPTIVE_ORDER=false
# - Ratio minimal de capital verrouillé/capital total pour activer la formule adaptative
# Exemple: Pour 0,1 : 10% / 0,2 : 20%, le bot n'appliquera la formule que si le capital verrouillé 
# représente au moins 10% du capital total. La formule permet de diminuer le capital utilisé dans le cas où le capital libre d'USDT > 50%  
# et inférieur à (100% - MIN_LOCKED_RATIO). Ainsi si le BTC monte vite, vous éviter d'acheter trop fort trop haut.
BINANCE_MIN_LOCKED_RATIO=0.1

# ----- Mexc -----
MEXC_BUY_OFFSET=-250
MEXC_SELL_OFFSET=250
MEXC_PERCENT=4
MEXC_BUY_MAX_DAYS=2
MEXC_BUY_MAX_PRICE_DEVIATION=40
MEXC_ACCUMULATION=true
MEXC_SELL_ACCU_PRICE_DEVIATION=40
MEXC_ADAPTIVE_ORDER=false
MEXC_MIN_LOCKED_RATIO=0.1

# ----- Kucoin -----
KUCOIN_BUY_OFFSET=-250
KUCOIN_SELL_OFFSET=250
KUCOIN_PERCENT=7
KUCOIN_BUY_MAX_DAYS=2
KUCOIN_BUY_MAX_PRICE_DEVIATION=40
KUCOIN_ACCUMULATION=true
KUCOIN_SELL_ACCU_PRICE_DEVIATION=40
KUCOIN_ADAPTIVE_ORDER=false
KUCOIN_MIN_LOCKED_RATIO=0.1

# ----- Kraken -----
KRAKEN_BUY_OFFSET=-300
KRAKEN_SELL_OFFSET=300
KRAKEN_PERCENT=5
KRAKEN_BUY_MAX_DAYS=2
KRAKEN_BUY_MAX_PRICE_DEVIATION=40
KRAKEN_ACCUMULATION=true
KRAKEN_SELL_ACCU_PRICE_DEVIATION=30
KRAKEN_ADAPTIVE_ORDER=false
KRAKEN_MIN_LOCKED_RATIO=0.1


# =========== VALEURS PAR DÉFAUT GLOBALES ===========
# Ces valeurs sont utilisées si les paramètres spécifiques à un exchange ne sont pas définis
DEFAULT_PERCENT=4
DEFAULT_BUY_MAX_DAYS=0
DEFAULT_BUY_MAX_PRICE_DEVIATION=0
DEFAULT_ACCUMULATION=false
DEFAULT_SELL_ACCU_PRICE_DEVIATION=10

# =========== CLÉS API PAR EXCHANGE ===========
# Ces clés sont OBLIGATOIRES pour l'exchange que vous utilisez
BINANCE_API_KEY=
BINANCE_SECRET_KEY=

MEXC_API_KEY=
MEXC_SECRET_KEY=

# Secret Key doit contenir la passphrase selon ce format : SECRET_KEY:PassPhrase
KUCOIN_API_KEY=
KUCOIN_SECRET_KEY=

KRAKEN_API_KEY=
KRAKEN_SECRET_KEY=

# =========== CONFIGURATION SUPPLÉMENTAIRE ===========
# Environment: production ou development
ENVIRONMENT=production

# Niveau de log: debug, info, warn, error
LOG_LEVEL=info`

	err := os.WriteFile(ConfigFilename, []byte(defaultConfig), 0644)
	if err != nil {
		return false, fmt.Errorf("failed to create config file: %w", err)
	}

	log.Printf("Fichier de configuration créé: %s", ConfigFilename)
	return true, nil
}

// GetScheduledTasks retourne la liste des tâches planifiées
func (c *Config) GetScheduledTasks() []types.TaskConfig {
	// Vérifier si le fichier de configuration des tâches existe
	tasksConfigFile := "tasks.conf"

	if _, err := os.Stat(tasksConfigFile); os.IsNotExist(err) {
		// Le fichier n'existe pas, retourner une liste vide
		return []types.TaskConfig{}
	}

	// Charger le fichier de configuration des tâches
	tasksConfigContent, err := os.ReadFile(tasksConfigFile)
	if err != nil {
		log.Printf("Erreur lors de la lecture du fichier de configuration des tâches: %v", err)
		// En cas d'erreur, retourner une liste vide
		return []types.TaskConfig{}
	}

	// Charger les variables d'environnement depuis le contenu du fichier
	// Utiliser Unmarshal au lieu de Parse pour accepter une string
	env, err := godotenv.Unmarshal(string(tasksConfigContent))
	if err != nil {
		log.Printf("Erreur lors du parsing du fichier de configuration des tâches: %v", err)
		return []types.TaskConfig{}
	}

	// Récupérer le nombre de tâches
	tasksCountStr, ok := env["TASKS_COUNT"]
	if !ok {
		log.Printf("Erreur: TASKS_COUNT non trouvé dans le fichier de configuration des tâches")
		return []types.TaskConfig{}
	}

	tasksCount, err := strconv.Atoi(tasksCountStr)
	if err != nil {
		log.Printf("Erreur lors de la conversion de TASKS_COUNT: %v", err)
		return []types.TaskConfig{}
	}

	// Préparer la liste des tâches
	tasks := make([]types.TaskConfig, 0, tasksCount)

	// Charger chaque tâche
	for i := 1; i <= tasksCount; i++ {
		taskConfig := types.TaskConfig{}

		// Récupérer les propriétés de base
		prefix := fmt.Sprintf("TASK_%d_", i)

		taskConfig.Name = env[prefix+"NAME"]
		taskConfig.Type = env[prefix+"TYPE"]

		// Charger la prochaine exécution prévue
		nextScheduledAtStr, ok := env[prefix+"NEXT_SCHEDULED_AT"]
		if ok {
			if nextScheduledAt, err := time.Parse(time.RFC3339, nextScheduledAtStr); err == nil {
				taskConfig.NextScheduledAt = nextScheduledAt
			}
		}

		// Récupérer si la tâche est activée
		enabledStr, ok := env[prefix+"ENABLED"]
		if ok {
			taskConfig.Enabled, _ = strconv.ParseBool(enabledStr)
		} else {
			taskConfig.Enabled = true // Activée par défaut
		}

		// Récupérer l'intervalle
		intervalValueStr, ok := env[prefix+"INTERVAL_VALUE"]
		if ok {
			intervalValue, err := strconv.Atoi(intervalValueStr)
			if err == nil {
				taskConfig.IntervalValue = intervalValue
			}
		}

		intervalUnitStr, ok := env[prefix+"INTERVAL_UNIT"]
		if ok {
			switch intervalUnitStr {
			case string(types.Minutes):
				taskConfig.IntervalUnit = types.Minutes
			case string(types.Hours):
				taskConfig.IntervalUnit = types.Hours
			case string(types.Days):
				taskConfig.IntervalUnit = types.Days
			}

			// Calculer l'intervalle en durée
			switch taskConfig.IntervalUnit {
			case types.Minutes:
				taskConfig.Interval = time.Duration(taskConfig.IntervalValue) * time.Minute
			case types.Hours:
				taskConfig.Interval = time.Duration(taskConfig.IntervalValue) * time.Hour
			case types.Days:
				taskConfig.Interval = time.Duration(taskConfig.IntervalValue) * 24 * time.Hour
			}
		}

		// Récupérer l'heure spécifique
		taskConfig.SpecificTime = env[prefix+"SPECIFIC_TIME"]

		// Récupérer l'exchange
		taskConfig.Exchange = env[prefix+"EXCHANGE"]

		// Récupérer les paramètres personnalisés pour les tâches de type "new"
		if taskConfig.Type == "new" {
			buyOffsetStr, ok := env[prefix+"BUY_OFFSET"]
			if ok {
				taskConfig.BuyOffset, _ = strconv.ParseFloat(buyOffsetStr, 64)
			}

			sellOffsetStr, ok := env[prefix+"SELL_OFFSET"]
			if ok {
				taskConfig.SellOffset, _ = strconv.ParseFloat(sellOffsetStr, 64)
			}

			percentStr, ok := env[prefix+"PERCENT"]
			if ok {
				taskConfig.Percent, _ = strconv.ParseFloat(percentStr, 64)
			}
		}

		tasks = append(tasks, taskConfig)
	}

	return tasks
}
