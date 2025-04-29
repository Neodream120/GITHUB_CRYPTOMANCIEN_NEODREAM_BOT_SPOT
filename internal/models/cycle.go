// internal/database/cycle.go
package database

import (
	"fmt"
	"log"
	"time"
)

// Status représente l'état d'un cycle de trading
type Status string

const (
	// StatusBuy représente un ordre d'achat actif
	StatusBuy Status = "buy"

	// StatusSell représente un ordre de vente actif
	StatusSell Status = "sell"

	// StatusCompleted représente un cycle complet (achat puis vente)
	StatusCompleted Status = "completed"

	// StatusCancelled représente un cycle annulé
	StatusCancelled Status = "cancelled"
)

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
	dto := map[string]interface{}{
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

	// Ajouter les informations de date dynamiquement
	dateInfo := c.ProcessCycleDates()
	for k, v := range dateInfo {
		dto[k] = v
	}

	return dto
}

func (c *Cycle) GetCycleDuration() time.Duration {
	if c.Status != "completed" || c.CompletedAt.IsZero() {
		// Si le cycle n'est pas complété ou que la date de complétion n'est pas définie,
		// retourner la durée depuis la création
		return time.Since(c.CreatedAt)
	}

	// Sinon, retourner la durée entre la création et la complétion
	return c.CompletedAt.Sub(c.CreatedAt)
}

func (c *Cycle) GetCycleDurationDays() float64 {
	return c.GetCycleDuration().Hours() / 24
}

func (c *Cycle) ProcessCycleDates() map[string]interface{} {
	log.Printf("DEBUG: Traitement des dates pour le cycle %d (Exchange: %s, Status: %s)",
		c.IdInt, c.Exchange, c.Status)

	dateInfo := map[string]interface{}{
		"sellDateFormatted": "-",
		"formattedDuration": "Durée inconnue",
	}

	switch c.Status {
	case "completed":
		// Utiliser CompletedAt si disponible, sinon utiliser une estimation
		completionTime := c.CompletedAt
		if completionTime.IsZero() {
			// Estimation basée sur l'exchange
			switch c.Exchange {
			case "KUCOIN":
				completionTime = c.CreatedAt.Add(6 * time.Hour)
			case "MEXC":
				completionTime = c.CreatedAt.Add(2 * time.Hour)
			case "BINANCE":
				completionTime = c.CreatedAt.Add(4 * time.Hour)
			default:
				completionTime = c.CreatedAt.Add(3 * time.Hour)
			}
		}

		log.Printf("DEBUG: CreatedAt: %s, CompletedAt (estimé): %s",
			c.CreatedAt.Format(time.RFC3339), completionTime.Format(time.RFC3339))

		// Formater la date de vente
		dateInfo["sellDateFormatted"] = completionTime.Format("02/01/2006 15:04")

		// Calculer la durée
		cycleDuration := completionTime.Sub(c.CreatedAt)
		durationDays := cycleDuration.Hours() / 24

		log.Printf("DEBUG: Durée du cycle calculée: %.4f jours", durationDays)

		dateInfo["formattedDuration"] = c.formatDetailedDuration(durationDays)

		// Informations de débogage
		dateInfo["rawBuyDate"] = c.CreatedAt
		dateInfo["rawSellDate"] = completionTime
		dateInfo["rawDurationDays"] = durationDays

	case "sell":
		// Pour les cycles en vente, indiquer la date de début de vente
		dateInfo["sellDateFormatted"] = fmt.Sprintf("Vente en cours depuis %s", c.CreatedAt.Format("02/01/2006"))

		ageInDays := c.GetAge()
		log.Printf("DEBUG: Âge du cycle en vente: %.4f jours", ageInDays)

		dateInfo["formattedDuration"] = c.formatDetailedDuration(ageInDays)
	}

	return dateInfo
}

// Méthode utilitaire pour formater la durée détaillée
func (c *Cycle) formatDetailedDuration(ageInDays float64) string {
	// Logs de débogage
	log.Printf("DEBUG: Formatage de la durée - Âge en jours: %.4f", ageInDays)

	// Convertir en heures pour des calculs précis
	hours := ageInDays * 24

	var formattedDuration string
	switch {
	case hours < 1:
		// Moins d'une heure
		minutes := int(hours * 60)
		formattedDuration = fmt.Sprintf("%d min", minutes)
		log.Printf("DEBUG: Moins d'une heure - %d minutes", minutes)

	case hours < 24:
		// Entre 1 heure et 24 heures
		h := int(hours)
		m := int((hours - float64(h)) * 60)
		formattedDuration = fmt.Sprintf("%dh %dm", h, m)
		log.Printf("DEBUG: Moins de 24 heures - %d heures %d minutes", h, m)

	case ageInDays < 7:
		// Entre 1 et 7 jours
		days := int(ageInDays)
		remainingHours := int(hours) % 24
		formattedDuration = fmt.Sprintf("%dj %dh", days, remainingHours)
		log.Printf("DEBUG: Entre 1 et 7 jours - %d jours %d heures", days, remainingHours)

	case ageInDays < 35:
		// Entre 7 et 35 jours (semaines)
		weeks := int(ageInDays / 7)
		remainingDays := int(ageInDays) % 7
		formattedDuration = fmt.Sprintf("%dsem %dj", weeks, remainingDays)
		log.Printf("DEBUG: Entre 7 et 35 jours - %d semaines %d jours", weeks, remainingDays)

	default:
		// Plus de 35 jours (mois)
		months := int(ageInDays / 30)
		remainingDays := int(ageInDays) % 30
		formattedDuration = fmt.Sprintf("%dmois %dj", months, remainingDays)
		log.Printf("DEBUG: Plus de 35 jours - %d mois %d jours", months, remainingDays)
	}

	return formattedDuration
}
