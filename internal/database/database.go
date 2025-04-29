package database

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"
)

const CollectionName = "cycles"

func GetDatabasePath() string {
	// Obtenir le répertoire de travail courant
	workDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	// Créer un chemin pour la base de données dans le projet
	databasePath := filepath.Join(workDir, "data", "db")

	// Créer le dossier s'il n'existe pas
	if _, err := os.Stat(databasePath); errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(databasePath, os.ModePerm)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Dossier de base de données créé: %s", databasePath)
	}

	return databasePath
}

type Status string

type Cycle struct {
	IdInt       int32     `json:"idInt"`
	Exchange    string    `json:"exchange"`
	Status      string    `json:"status"`
	Quantity    float64   `json:"quantity"`
	BuyPrice    float64   `json:"buyPrice"`
	BuyId       string    `json:"buyId"`
	SellPrice   float64   `json:"sellPrice"`
	SellId      string    `json:"sellId"`
	CreatedAt   time.Time `json:"createdAt"`   // Date d'achat (création)
	CompletedAt time.Time `json:"completedAt"` // Date de vente (complétion)

	// Nouveaux champs ajoutés pour le calcul précis des gains
	PurchaseAmountUSDC float64 `json:"purchaseAmountUSDC"`
	SaleAmountUSDC     float64 `json:"saleAmountUSDC"`
	ExactExchangeGain  float64 `json:"exactExchangeGain"`
	TotalFees          float64 `json:"totalFees"` // Total des frais (achat + vente)
}

// Nouvelle fonction pour calculer le gain exact
func (c *Cycle) CalculateExactGain() {
	// Calcul précis du gain exact basé sur les montants USDC
	c.ExactExchangeGain = c.SaleAmountUSDC - c.PurchaseAmountUSDC
}

// Fonction modifiée pour calculer les gains de tous les cycles
func CalculateCyclesGains(cycles []Cycle) {
	for i := range cycles {
		cycles[i].CalculateExactGain()
	}
}

// GetAge retourne l'âge du cycle en jours
func (c *Cycle) GetAge() float64 {
	// Si CreatedAt n'est pas défini, on retourne 0
	if c.CreatedAt.IsZero() {
		return 0
	}

	// Calcul de la différence en jours
	duration := time.Since(c.CreatedAt)
	return duration.Hours() / 24
}

// CalculateProfit calcule le profit en USD du cycle
func (c *Cycle) CalculateProfit() float64 {
	if c.Status != "completed" {
		return 0
	}

	buyTotal := c.BuyPrice * c.Quantity
	sellTotal := c.SellPrice * c.Quantity

	return sellTotal - buyTotal
}

// CalculateProfitPercentage calcule le pourcentage de profit du cycle
func (c *Cycle) CalculateProfitPercentage() float64 {
	if c.Status != "completed" || c.BuyPrice == 0 {
		return 0
	}

	profit := c.CalculateProfit()
	buyTotal := c.BuyPrice * c.Quantity

	return (profit / buyTotal) * 100
}

// FormatStatus retourne un statut formaté pour l'affichage
func (c *Cycle) FormatStatus() string {
	switch c.Status {
	case "buy":
		return "Achat en cours"
	case "sell":
		return "Vente en cours"
	case "completed":
		return "Complété"
	case "cancelled":
		return "Annulé"
	default:
		return c.Status
	}
}

// ToCycleDTO convertit un Cycle en CycleDTO pour l'affichage dans l'interface
func (c *Cycle) ToCycleDTO() map[string]interface{} {
	return map[string]interface{}{
		"idInt":     c.IdInt,
		"exchange":  c.Exchange,
		"status":    c.Status,
		"quantity":  c.Quantity,
		"buyPrice":  c.BuyPrice,
		"sellPrice": c.SellPrice,
		"change":    c.CalculateProfitPercentage(),
		"buyId":     c.BuyId,
		"sellId":    c.SellId,
		"createdAt": c.CreatedAt.Format(time.RFC3339),
		"age":       c.GetAge(),
	}
}
